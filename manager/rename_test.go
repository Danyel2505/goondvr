package manager

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HeapOfChaos/goondvr/channel"
	"github.com/HeapOfChaos/goondvr/entity"
	"github.com/HeapOfChaos/goondvr/internal"
	"github.com/HeapOfChaos/goondvr/site"
)

type fakeRenamer struct {
	mu    sync.Mutex
	to    map[string]string // username -> new name
	asked []string
}

func (f *fakeRenamer) ResolveRename(_ context.Context, _ *internal.Req, username string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, username)
	return f.to[username], nil
}

func (f *fakeRenamer) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.asked) }

func renameRig(t *testing.T, to map[string]string, cfgs ...*entity.ChannelConfig) (*Manager, map[string]*channel.Channel, *fakeRenamer) {
	t.Helper()
	m, _, ch := autoRig(t, cfgs...)
	fr := &fakeRenamer{to: to}
	oldFor, oldNow := renamerFor, renameNow
	renamerFor = func(string) (site.Renamer, bool) { return fr, true }
	renameNow = func() time.Time { return time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() {
		renamerFor, renameNow = oldFor, oldNow
		// a renamed channel is no longer in m.Channels, so the rig's own cleanup never waits for it: without this its
		// last goroutines can still read server.Config when the next test rewrites it
		for _, c := range ch {
			c.Stop()
			c.WaitPublisherExit()
			c.WaitBackground()
		}
	})
	return m, ch, fr
}

func snapshotOf(t *testing.T, m *Manager, name string) (entity.ChannelConfig, *channel.Channel) {
	t.Helper()
	v, ok := m.Channels.Load(entity.ChannelID("chaturbate", name))
	if !ok {
		t.Fatalf("there is no channel %q", name)
	}
	return v.(*channel.Channel).ConfigSnapshot(), v.(*channel.Channel)
}

func TestRenameKeepsTheSettingsAndBuildsTheHistory(t *testing.T) {
	m, ch, _ := renameRig(t, nil, &entity.ChannelConfig{Username: "old", IsPaused: true, Resolution: 720, Framerate: 30, AutoRecord: true,
		Detect: &entity.DetectConfig{Enabled: true, Model: "m", Labels: []string{"a"}}})
	oldCh := ch["old"]
	sub, _ := m.hub.subscribe(nil)

	if err := m.RenameChannel("chaturbate__old", "new1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Channels.Load("chaturbate__old"); ok {
		t.Fatal("the old channel is still there")
	}
	oldCh.WaitPublisherExit()
	cfg, nc := snapshotOf(t, m, "new1")
	if cfg.Resolution != 720 || cfg.Framerate != 30 || !cfg.AutoRecord || !cfg.IsPaused || cfg.Detect == nil || cfg.Detect.Model != "m" {
		t.Fatalf("the settings were not kept: %+v", cfg)
	}
	if want := []entity.NameChange{{From: "old", To: "new1", At: renameNow().Unix()}}; len(cfg.NameHistory) != 1 || cfg.NameHistory[0] != want[0] {
		t.Fatalf("history = %+v", cfg.NameHistory)
	}
	if cfg.Detect == oldCh.ConfigSnapshot().Detect {
		t.Fatal("the detection settings are shared with the old channel")
	}
	deadline := time.After(time.Second)
	for told := false; !told; {
		select {
		case f := <-sub.ch: // other events (the old channel's last log line) may come first
			told = strings.Contains(string(f), "event: renamed") && strings.Contains(string(f), "chaturbate__new1")
		case <-deadline:
			t.Fatal("the web pages were not told")
		}
	}

	// a second rename extends the history, oldest first, and everything is what channels.json will hold
	if err := m.RenameChannel("chaturbate__new1", "new2"); err != nil {
		t.Fatal(err)
	}
	nc.WaitPublisherExit()
	cfg, nc2 := snapshotOf(t, m, "new2")
	if len(cfg.NameHistory) != 2 || cfg.NameHistory[0].From != "old" || cfg.NameHistory[1].From != "new1" || cfg.NameHistory[1].To != "new2" {
		t.Fatalf("history = %+v", cfg.NameHistory)
	}
	b, _ := json.Marshal(cfg)
	if !strings.Contains(string(b), `"name_history":[{"from":"old","to":"new1","at":`) {
		t.Fatalf("json = %s", b)
	}
	info := nc2.ExportInfo()
	if len(info.NameHistory) != 2 || info.NameHistory[0].Date() != "2026-11-17" || info.Username != "new2" {
		t.Fatalf("card = %+v", info.NameHistory)
	}
}

func TestRenameRefusesANameThatIsTakenOrInvalid(t *testing.T) {
	m, _, _ := renameRig(t, nil, &entity.ChannelConfig{Username: "a", IsPaused: true}, &entity.ChannelConfig{Username: "b", IsPaused: true})
	if err := m.RenameChannel("chaturbate__a", "b"); err == nil {
		t.Fatal("renamed onto a channel that exists")
	}
	if err := m.RenameChannel("chaturbate__a", "bad name!"); err == nil {
		t.Fatal("accepted an invalid name")
	}
	if err := m.RenameChannel("chaturbate__nobody", "c"); err == nil {
		t.Fatal("renamed a channel that does not exist")
	}
	if cfg, _ := snapshotOf(t, m, "a"); len(cfg.NameHistory) != 0 {
		t.Fatalf("a refused rename left a history: %+v", cfg.NameHistory)
	}
}

func TestRenameStepAsksOnlyTheChannelsThatAreDueAndAFewAtATime(t *testing.T) {
	cfgs := []*entity.ChannelConfig{{Username: "onair", IsPaused: true}}
	to := map[string]string{"onair": "onair2"}
	for _, n := range []string{"c1", "c2", "c3", "c4", "c5"} {
		cfgs = append(cfgs, &entity.ChannelConfig{Username: n, IsPaused: true})
		to[n] = n + "x"
	}
	m, ch, fr := renameRig(t, to, cfgs...)
	oldEvery, oldPer := renameEvery, renamePerTick
	renameEvery, renamePerTick = 30*time.Millisecond, 3
	t.Cleanup(func() { renameEvery, renamePerTick = oldEvery, oldPer })
	ch["onair"].ApplyPresence(live())
	req := internal.NewReq()
	step := func() { m.renameStep(context.Background(), req) }

	step() // the first look only schedules the questions
	if fr.count() != 0 {
		t.Fatalf("asked at the first look: %v", fr.asked)
	}
	time.Sleep(60 * time.Millisecond) // they are due now (renameNow is fixed, so the clock of the test is the real one below)
	renameNow = time.Now
	step()
	if fr.count() != 3 {
		t.Fatalf("asked %d channels in one step, want 3: %v", fr.count(), fr.asked)
	}
	step()
	if fr.count() != 5 {
		t.Fatalf("after the second step %d were asked, want the 5 that are off the air: %v", fr.count(), fr.asked)
	}
	for _, n := range fr.asked {
		if n == "onair" {
			t.Fatal("a channel that is on the air was asked")
		}
	}
	for _, n := range []string{"c1x", "c2x", "c3x", "c4x", "c5x"} {
		cfg, _ := snapshotOf(t, m, n)
		if len(cfg.NameHistory) != 1 || cfg.NameHistory[0].To != n {
			t.Fatalf("%s: %+v", n, cfg.NameHistory)
		}
	}
	if _, ok := m.Channels.Load("chaturbate__onair"); !ok {
		t.Fatal("the channel on the air was renamed")
	}
}

func TestRenameStepAsksAChannelTheSiteCouldNotFindWithoutWaiting(t *testing.T) {
	m, ch, fr := renameRig(t, map[string]string{"gone": "back"}, &entity.ChannelConfig{Username: "gone", IsPaused: true}, &entity.ChannelConfig{Username: "quiet", IsPaused: true})
	ch["gone"].NoteNotFound()
	m.renameStep(context.Background(), internal.NewReq())
	if fr.count() != 1 || fr.asked[0] != "gone" {
		t.Fatalf("asked %v, want only the channel that was not found", fr.asked)
	}
	if _, ok := m.Channels.Load("chaturbate__back"); !ok {
		t.Fatal("the channel was not renamed")
	}
}

func TestTheCardShowsTheFormerNames(t *testing.T) {
	info := &entity.ChannelInfo{ChannelID: "chaturbate__c", Username: "c", Site: "chaturbate", SiteDomain: "https://chaturbate.com/",
		GlobalConfig: &entity.Config{}, NameHistory: []entity.NameChange{{From: "a", To: "b", At: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC).Unix()}, {From: "b", To: "c", At: time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC).Unix()}}}
	msg, err := renderEvent(entity.EventUpdate, info)
	if err != nil {
		t.Fatal(err)
	}
	html := string(msg.data)
	for _, want := range []string{
		"Former names: a &rarr; b (2026-01-02) &middot; b &rarr; c (2026-03-04)", // expanded view
		`title="Former names: a -> b (2026-01-02), b -> c (2026-03-04)"`,         // grid and collapsed views
		`data-q="c a b `, // the search box finds the card by a former name
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the card lacks %q", want)
		}
	}
	info.NameHistory = nil
	msg, _ = renderEvent(entity.EventUpdate, info)
	if strings.Contains(string(msg.data), "Former names") {
		t.Error("a channel that was never renamed shows a history")
	}
}
