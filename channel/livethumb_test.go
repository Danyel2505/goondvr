package channel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HeapOfChaos/goondvr/internal"
)

// fakeThumbSource hands out a new preview address on every call.
type fakeThumbSource struct {
	calls atomic.Int32
	fail  atomic.Bool
}

func (f *fakeThumbSource) FetchLiveThumbURL(ctx context.Context, _ *internal.Req, user string) (string, error) {
	n := f.calls.Add(1)
	if f.fail.Load() {
		return "", errors.New("site unavailable")
	}
	return fmt.Sprintf("https://cdn.example/thumbs/%d/%s", n, user), nil
}

// startRefresher runs keepLiveThumbFresh with short timings and returns a
// function that stops it and waits for it to exit.
func startRefresher(t *testing.T, ui *fakeUI, src *fakeThumbSource) (*Channel, func()) {
	t.Helper()
	oldI, oldB, oldJ := liveThumbRefreshInterval, liveThumbMaxBackoff, liveThumbJitterMax
	liveThumbRefreshInterval, liveThumbMaxBackoff, liveThumbJitterMax = 20*time.Millisecond, 160*time.Millisecond, 0
	ch := newTestChannel(t, ui, time.Hour)
	ch.setLiveThumbURL("https://cdn.example/thumbs/0/tester")

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); ch.keepLiveThumbFresh(ctx, src) }()
	stop := func() { cancel(); wg.Wait() }
	// Registered after newTestChannel's cleanup, so it runs first: the goroutine
	// reads globals that are restored there.
	t.Cleanup(func() {
		stop()
		liveThumbRefreshInterval, liveThumbMaxBackoff, liveThumbJitterMax = oldI, oldB, oldJ
	})
	return ch, stop
}

func TestLiveThumbIsRenewedWhileSomeoneWatches(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}, viewers: true}
	src := &fakeThumbSource{}
	ch, _ := startRefresher(t, ui, src)

	first := ch.liveThumbURL()
	if !waitFor(2*time.Second, func() bool { return src.calls.Load() >= 2 }) {
		t.Fatal("the preview address was not renewed twice in 2 s")
	}
	if got := ch.liveThumbURL(); got == first {
		t.Fatalf("preview address still %q after %d renewals", got, src.calls.Load())
	}
}

func TestLiveThumbIsNotAskedForWithoutViewers(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}, viewers: false}
	src := &fakeThumbSource{}
	startRefresher(t, ui, src)

	time.Sleep(300 * time.Millisecond) // ~15 intervals
	if n := src.calls.Load(); n != 0 {
		t.Fatalf("site was asked %d times although no browser has the UI open", n)
	}
}

func TestLiveThumbKeepsThePictureWhenTheSiteFails(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}, viewers: true}
	src := &fakeThumbSource{}
	src.fail.Store(true)
	ch, _ := startRefresher(t, ui, src)
	before := ch.liveThumbURL()

	time.Sleep(500 * time.Millisecond)
	if got := ch.liveThumbURL(); got != before {
		t.Fatalf("a failing site replaced the preview address: %q", got)
	}
	// Backing off: 20, 40, 80, 160, 160... ms means ~5 calls in 500 ms, not 25.
	if n := src.calls.Load(); n < 2 || n > 8 {
		t.Fatalf("expected a handful of backed-off retries, got %d", n)
	}

	src.fail.Store(false)
	if !waitFor(2*time.Second, func() bool { return ch.liveThumbURL() != before }) {
		t.Fatal("the preview address did not recover once the site answered again")
	}
}

func TestLiveThumbRefresherStopsWithItsContext(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}, viewers: true}
	src := &fakeThumbSource{}
	_, stop := startRefresher(t, ui, src)

	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("refresher did not stop when its context was cancelled")
	}
	n := src.calls.Load()
	time.Sleep(100 * time.Millisecond)
	if src.calls.Load() != n {
		t.Fatal("refresher kept calling the site after it was stopped")
	}
}
