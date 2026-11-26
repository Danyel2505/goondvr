package channel

import (
	"math/rand"
	"time"
)

// A renamed account: the site sends its old address to the new one. The manager (manager/rename.go) asks
// the site about the channels that are not on the air; these are the channel's side of it.

// NoteNotFound records that the site answered "not found" (404) for the channel's name, which is what a
// renamed (or deleted) account looks like: its check is brought forward.
func (ch *Channel) NoteNotFound() { ch.notFound.Store(true) }

// RenameDue reports whether the site should be asked now whether the account was renamed: every `every`
// for a channel that is not on the air (the first time at a random moment inside that period, so that
// many channels do not ask at once), and after `soon` for one the site said was not found. A channel that
// is on the air or being recorded plainly still has its name.
func (ch *Channel) RenameDue(now time.Time, every, soon time.Duration) bool {
	if ch.IsAvailable() {
		return false
	}
	last := ch.lastRename.Load()
	if last == 0 {
		if ch.notFound.Load() {
			return true
		}
		ch.lastRename.CompareAndSwap(0, now.UnixNano()-rand.Int63n(int64(every)))
		return false
	}
	age := now.Sub(time.Unix(0, last))
	if ch.notFound.Load() {
		return age >= soon
	}
	return age >= every
}

// RenameChecked records that the site was just asked.
func (ch *Channel) RenameChecked(now time.Time) {
	ch.lastRename.Store(now.UnixNano())
	ch.notFound.Store(false)
}
