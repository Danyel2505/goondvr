package channel

import (
	"sync"
	"testing"
	"time"

	"github.com/HeapOfChaos/goondvr/entity"
	"github.com/HeapOfChaos/goondvr/server"
)

// fakeUI stands in for the manager: it records what the Publisher pushes and
// can be told to refuse (queue full) or to hang (browser not reading).
type fakeUI struct {
	server.IManager

	mu      sync.Mutex
	pushes  map[string]int
	refuse  int           // refuse this many Publish calls (return false)
	hang    chan struct{} // if set, Publish blocks until closed
	lastLog []string
}

func (f *fakeUI) Publish(evt string, info *entity.ChannelInfo) bool {
	if f.hang != nil {
		<-f.hang
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse > 0 {
		f.refuse--
		return false
	}
	f.pushes[evt]++
	if evt == entity.EventLog {
		f.lastLog = info.Logs
	}
	return true
}

func (f *fakeUI) count(evt string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pushes[evt]
}

func newTestChannel(t *testing.T, ui *fakeUI, interval time.Duration) *Channel {
	t.Helper()
	oldInterval, oldMgr, oldCfg := uiMinInterval, server.Manager, server.Config
	uiMinInterval = interval
	server.Manager = ui
	server.Config = &entity.Config{}
	ch := New(&entity.ChannelConfig{Username: "tester", Site: "chaturbate"})
	t.Cleanup(func() {
		ch.stopPublisher()
		<-ch.publisherExited // the goroutine reads the globals restored below
		uiMinInterval, server.Manager, server.Config = oldInterval, oldMgr, oldCfg
	})
	return ch
}

func TestPublisherCoalescesBursts(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}}
	ch := newTestChannel(t, ui, 100*time.Millisecond)

	// 2000 updates and 2000 log lines in ~400 ms, like a busy recording.
	deadline := time.Now().Add(400 * time.Millisecond)
	n := 0
	for time.Now().Before(deadline) {
		ch.Update()
		ch.sendLog("line")
		n++
		time.Sleep(200 * time.Microsecond)
	}
	time.Sleep(250 * time.Millisecond) // let the trailing push happen

	if got := ui.count(entity.EventUpdate); got < 2 || got > 8 {
		t.Fatalf("%d signals produced %d update pushes, want a handful (2-8)", n, got)
	}
	if got := ui.count(entity.EventLog); got < 2 || got > 8 {
		t.Fatalf("%d log lines produced %d log pushes, want a handful (2-8)", n, got)
	}
	// The last push must carry the latest state, capped at maxLogLines.
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if len(ui.lastLog) != maxLogLines {
		t.Fatalf("last pushed log has %d lines, want %d", len(ui.lastLog), maxLogLines)
	}
}

func TestPublisherSendsFirstChangeImmediately(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}}
	ch := newTestChannel(t, ui, time.Hour) // cool-down far longer than the test

	ch.Update()
	deadline := time.Now().Add(time.Second)
	for ui.count(entity.EventUpdate) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ui.count(entity.EventUpdate) != 1 {
		t.Fatal("first change after a quiet period was not pushed immediately")
	}
}

func TestProducersNeverBlockOnAStuckBrowser(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}, hang: make(chan struct{})}
	ch := newTestChannel(t, ui, time.Millisecond)
	defer close(ui.hang)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50000; i++ { // what the recording loop does per segment
			ch.sendLog("duration")
			ch.Update()
			ch.UpdateThumb()
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Update/UpdateThumb/sendLog blocked while the web UI was not consuming events")
	}
}

func TestPublishIsRetriedWhenQueueIsFull(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}, refuse: 3}
	ch := newTestChannel(t, ui, 20*time.Millisecond)

	ch.Update()
	deadline := time.Now().Add(2 * time.Second)
	for ui.count(entity.EventUpdate) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ui.count(entity.EventUpdate) != 1 {
		t.Fatal("update refused by a full queue was never retried")
	}
}
