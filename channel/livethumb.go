package channel

import (
	"context"
	"hash/crc32"
	"time"

	"github.com/HeapOfChaos/goondvr/internal"
	"github.com/HeapOfChaos/goondvr/server"
	"github.com/HeapOfChaos/goondvr/site"
)

// Live previews of sites whose preview address is a snapshot (Stripchat) go
// out of date: the address is worked out once when the recording starts and
// then keeps pointing at the same picture for hours. keepLiveThumbFresh asks
// the site for a newer one now and then.
//
// It costs one light API call per channel per minute and only while somebody
// has the web UI open, so a recorder running headless does no extra work.

var (
	// liveThumbRefreshInterval is how often the newest preview address is asked for.
	// (Variables only so that tests can shorten them.)
	liveThumbRefreshInterval = 60 * time.Second
	// liveThumbMaxBackoff is the longest wait after repeated failures.
	liveThumbMaxBackoff = 5 * time.Minute
	// liveThumbJitterMax spreads the channels over this much time so that they
	// do not all ask in the same second.
	liveThumbJitterMax = 10 * time.Second
)

// liveThumbURL returns the address of the channel's live preview.
func (ch *Channel) liveThumbURL() string {
	ch.stateMu.RLock()
	defer ch.stateMu.RUnlock()
	return ch.LiveThumbURL
}

// setLiveThumbURL stores a new preview address and reports whether it differs
// from the previous one.
func (ch *Channel) setLiveThumbURL(u string) bool {
	ch.stateMu.Lock()
	defer ch.stateMu.Unlock()
	if ch.LiveThumbURL == u {
		return false
	}
	ch.LiveThumbURL = u
	return true
}

// GetLiveThumbURL is the address of the channel's live preview (safe to call
// from any goroutine).
func (ch *Channel) GetLiveThumbURL() string { return ch.liveThumbURL() }

// keepLiveThumbFresh renews the live preview address until ctx ends.
func (ch *Channel) keepLiveThumbFresh(ctx context.Context, src site.LiveThumbSource) {
	req := internal.NewReq()
	var jitter time.Duration
	if liveThumbJitterMax > 0 {
		jitter = time.Duration(crc32.ChecksumIEEE([]byte(ch.Config.Username))) % liveThumbJitterMax
	}

	failures := 0
	for {
		wait := liveThumbRefreshInterval + jitter
		for i := 0; i < failures && wait < liveThumbMaxBackoff; i++ {
			wait *= 2
		}
		if wait > liveThumbMaxBackoff {
			wait = liveThumbMaxBackoff
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		if !server.Manager.HasViewers() {
			continue // nobody is looking at the previews
		}
		u, err := src.FetchLiveThumbURL(ctx, req, ch.Config.Username)
		if err != nil || u == "" {
			if ctx.Err() != nil {
				return
			}
			failures++ // keep the picture we have and try again later
			continue
		}
		failures = 0
		ch.setLiveThumbURL(u)
	}
}
