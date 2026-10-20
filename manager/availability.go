package manager

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/HeapOfChaos/goondvr/channel"
	"github.com/HeapOfChaos/goondvr/internal"
)

// Availability watcher: tells the web UI which paused channels are on air, so
// that they can move up the list without being recorded.
//
// It is built to cost almost nothing:
//   - nothing runs while no browser has the web UI open;
//   - one channel is asked at a time, one light API request, no browser fallback;
//   - the pace adapts to the number of paused channels (a full round about every
//     availCycle, never faster than availMinSpacing between two requests), and the
//     first round after start-up is a bit quicker so that the list settles soon;
//   - a Cloudflare block makes it back off (doubling, up to availMaxBlock) and
//     is not reported as a channel problem.
//
// (Variables only so that tests can shorten them.)
var (
	availCycle        = 60 * time.Second
	availMinSpacing   = 1200 * time.Millisecond
	availMaxSpacing   = 6 * time.Second
	availFirstSpacing = 700 * time.Millisecond // between channels never probed yet
	availIdlePoll     = 5 * time.Second        // how often it checks for a browser / work when idle
	availMaxBlock     = 10 * time.Minute
)

// availabilityLoop runs for the life of the process.
func (m *Manager) availabilityLoop() {
	req := internal.NewReq()
	block := time.Duration(0)
	for {
		if !m.HasViewers() {
			time.Sleep(availIdlePoll)
			continue
		}
		if m.cloudflareBlockedRecently() {
			// The channels being recorded are already being blocked: do not add to it.
			time.Sleep(availIdlePoll)
			continue
		}
		ch, last, n := m.nextProbeCandidate()
		if ch == nil {
			time.Sleep(availIdlePoll)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err := ch.ProbeAvailability(ctx, req)
		cancel()

		wait := probeSpacing(n, last == 0)
		switch {
		case err == nil:
			block = 0
		case errors.Is(err, internal.ErrCloudflareBlocked):
			if block == 0 {
				block = time.Minute
			} else if block *= 2; block > availMaxBlock {
				block = availMaxBlock
			}
			if block > wait {
				wait = block
			}
		}
		time.Sleep(wait + time.Duration(rand.Int63n(int64(wait/5)+1)))
	}
}

// probeSpacing is the pause after a probe when n channels are waiting for one.
func probeSpacing(n int, firstRound bool) time.Duration {
	if firstRound {
		return availFirstSpacing
	}
	if n < 1 {
		n = 1
	}
	d := availCycle / time.Duration(n)
	if d < availMinSpacing {
		d = availMinSpacing
	}
	if d > availMaxSpacing {
		d = availMaxSpacing
	}
	return d
}

// nextProbeCandidate returns the paused channel that was probed the longest ago
// (never probed first), when that was, and how many channels are paused.
func (m *Manager) nextProbeCandidate() (best *channel.Channel, bestLast int64, n int) {
	m.Channels.Range(func(_, v any) bool {
		ch := v.(*channel.Channel)
		eligible, last := ch.ProbeCandidate()
		if !eligible {
			return true
		}
		n++
		if best == nil || last < bestLast {
			best, bestLast = ch, last
		}
		return true
	})
	return best, bestLast, n
}

// availCFQuiet is how long after the last Cloudflare block reported by a
// channel the watcher stays silent.
const availCFQuiet = 5 * time.Minute

// cloudflareBlockedRecently reports whether any channel was blocked by
// Cloudflare in the last availCFQuiet.
func (m *Manager) cloudflareBlockedRecently() bool {
	m.cfBlocksMu.Lock()
	defer m.cfBlocksMu.Unlock()
	for _, t := range m.cfBlocks {
		if time.Since(t) < availCFQuiet {
			return true
		}
	}
	return false
}
