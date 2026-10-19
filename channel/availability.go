package channel

import (
	"context"
	"fmt"
	"time"

	"github.com/HeapOfChaos/goondvr/internal"
	"github.com/HeapOfChaos/goondvr/site"
)

// A channel that is paused is not looked at by Monitor, so nothing tells the UI
// that it is on air. The manager asks for it, one paused channel at a time and
// only while a browser has the web UI open (see manager/availability.go), and
// the answer is kept in Channel.Available. Channels that are being monitored
// do not need this: Monitor sets Available itself (recording => true, the site
// says offline/private/hidden => false).

// setAvailable stores whether the channel is on air and reports whether that
// changed. It does not publish anything.
func (ch *Channel) setAvailable(v bool) bool {
	ch.stateMu.Lock()
	defer ch.stateMu.Unlock()
	if ch.Available == v {
		return false
	}
	ch.Available = v
	if !v && !ch.IsOnline {
		ch.NumViewers = 0
	}
	return true
}

// IsAvailable reports whether the channel is broadcasting right now.
func (ch *Channel) IsAvailable() bool {
	ch.stateMu.RLock()
	defer ch.stateMu.RUnlock()
	return ch.Available || ch.IsOnline
}

// ProbeCandidate reports whether the channel should be probed (it is paused and
// not recording) and when it was probed last (unix nanoseconds, 0 = never).
func (ch *Channel) ProbeCandidate() (eligible bool, last int64) {
	if !ch.Paused() || ch.Online() {
		return false, 0
	}
	return true, ch.lastProbe.Load()
}

// ProbeAvailability asks the site once whether the channel is broadcasting and
// updates the card if the answer changed. A failed probe leaves the previous
// answer alone and is returned so that the caller can back off.
func (ch *Channel) ProbeAvailability(ctx context.Context, req *internal.Req) error {
	ch.lastProbe.Store(time.Now().UnixNano())
	src, ok := resolveSite(ch.Config.Site).(site.AvailabilitySource)
	if !ok {
		return fmt.Errorf("site %q cannot report availability", ch.Config.Site)
	}
	av, err := src.FetchAvailability(ctx, req, ch.Config.Username)
	if err != nil {
		return err
	}
	if ch.Online() || !ch.Paused() {
		return nil // it was resumed meanwhile: Monitor owns the state from now on
	}

	ch.stateMu.Lock()
	changed := ch.Available != av.Live
	ch.Available = av.Live
	if av.Live {
		if av.NumViewers > 0 {
			ch.NumViewers = av.NumViewers
		}
		if av.LiveThumbURL != "" {
			ch.LiveThumbURL = av.LiveThumbURL // read per request by the preview proxy: no redraw needed
		}
	} else {
		ch.NumViewers = 0
	}
	ch.stateMu.Unlock()

	if changed {
		ch.UpdateThumb() // the card switches between the profile photo and the live preview
		ch.UpdateNow()
	}
	return nil
}
