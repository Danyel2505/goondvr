package channel

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/HeapOfChaos/goondvr/internal"
	"github.com/HeapOfChaos/goondvr/site"
)

func TestLastSeenFollowsTheBroadcast(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}}
	ch := newTestChannel(t, ui, time.Millisecond)

	if got := ch.ExportInfo().LastSeen; got != "" {
		t.Fatalf("a channel never seen must show nothing, got %q", got)
	}
	ch.UpdateOnlineStatus(false) // a failed poll of a channel that was never online
	if ch.ExportInfo().LastSeen != "" || ui.saved != 0 {
		t.Fatalf("polling an offline channel must not stamp or save anything (saved %d)", ui.saved)
	}

	ch.UpdateOnlineStatus(true)
	ch.stateMu.RLock()
	started := ch.LastSeen
	ch.stateMu.RUnlock()
	if started == 0 || ch.Config.LastSeen != started {
		t.Fatalf("going online must stamp the date and mirror it in the saved config: %d / %d", started, ch.Config.LastSeen)
	}

	ch.stateMu.Lock()
	ch.LastSeen -= 3600 // pretend the broadcast has been running for an hour
	ch.stateMu.Unlock()
	ch.UpdateOnlineStatus(false)
	ch.stateMu.RLock()
	ended := ch.LastSeen
	ch.stateMu.RUnlock()
	if ended < started || ch.Config.LastSeen != ended {
		t.Fatalf("the end of the broadcast must move the date forward: %d -> %d", started, ended)
	}
	if ui.saved != 1 {
		t.Fatalf("the end of a broadcast must save the list once, saved %d", ui.saved)
	}
	if want := time.Unix(ended, 0).Format("2006-01-02 15:04"); ch.ExportInfo().LastSeen != want {
		t.Fatalf("date shown %q, want %q", ch.ExportInfo().LastSeen, want)
	}
}

func TestLastSeenOfAPausedChannelComesFromThePresenceAndTheProbe(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}}
	ch := newTestChannel(t, ui, time.Millisecond)
	ch.Config.IsPaused = true

	if air, _ := ch.ApplyPresence(&site.Presence{Live: true}); !air || ch.ExportInfo().LastSeen == "" {
		t.Fatal("seen on the list of rooms on air: the date must be stamped")
	}
	ch.stateMu.Lock()
	ch.LastSeen = time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local).Unix()
	ch.stateMu.Unlock()
	if air, _ := ch.ApplyPresence(&site.Presence{Live: true}); air {
		t.Fatal("still on air is not a change")
	}
	if ch.ExportInfo().LastSeen != "2020-01-02 03:04" {
		t.Fatalf("an unchanged mark must not move the date, got %q", ch.ExportInfo().LastSeen)
	}
	if air, _ := ch.ApplyPresence(nil); !air || ch.ExportInfo().LastSeen == "2020-01-02 03:04" {
		t.Fatal("going off the air must stamp the moment it was last seen")
	}

	var body atomic.Value
	var calls atomic.Int32
	body.Store(roomLive)
	fakeRoomAPI(t, &body, &calls)
	ch.stateMu.Lock()
	ch.LastSeen = time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local).Unix()
	ch.stateMu.Unlock()
	if err := ch.ProbeAvailability(t.Context(), internal.NewReq()); err != nil {
		t.Fatal(err)
	}
	if ch.ExportInfo().LastSeen == "2020-01-02 03:04" {
		t.Fatal("the probe saw it go on air: the date must move")
	}
}

func TestLastSeenFallsBackToTheLastStreamAndUses24Hours(t *testing.T) {
	ui := &fakeUI{pushes: map[string]int{}}
	ch := newTestChannel(t, ui, time.Millisecond)
	ch.stateMu.Lock()
	ch.StreamedAt = time.Date(2026, 11, 27, 15, 4, 0, 0, time.Local).Unix()
	ch.stateMu.Unlock()
	info := ch.ExportInfo()
	if info.LastSeen != "2026-11-27 15:04" || info.StreamedAt != "2026-11-27 15:04" {
		t.Fatalf("an old config has only the last stream: it stands in, in 24 h format (no AM/PM); got %q / %q", info.LastSeen, info.StreamedAt)
	}
}
