package channel

import (
	"testing"
	"time"
)

func TestRenameDueRules(t *testing.T) {
	const every, soon = time.Hour, 10 * time.Minute
	now := time.Now()
	ch := &Channel{} // the zero value is enough: only atomics and the availability flag are read

	// first look: scheduled at a random moment inside the period, never asked at once
	if ch.RenameDue(now, every, soon) {
		t.Fatal("asked at the first look")
	}
	if ch.RenameDue(now, every, soon) {
		t.Fatal("asked right after scheduling")
	}
	if !ch.RenameDue(now.Add(every), every, soon) {
		t.Fatal("not asked after a whole period")
	}
	ch.RenameChecked(now)
	if ch.RenameDue(now.Add(every-time.Second), every, soon) || !ch.RenameDue(now.Add(every), every, soon) {
		t.Fatal("the period is counted from the last question")
	}

	// the site said "not found": asked at once the first time, then after `soon`
	nf := &Channel{}
	nf.NoteNotFound()
	if !nf.RenameDue(now, every, soon) {
		t.Fatal("a channel that was not found waited for its first question")
	}
	nf.RenameChecked(now)
	nf.NoteNotFound()
	if nf.RenameDue(now.Add(soon-time.Second), every, soon) || !nf.RenameDue(now.Add(soon), every, soon) {
		t.Fatal("a channel that was not found is asked again after `soon`")
	}

	// on the air (or being recorded): it plainly has its name
	ch.Available = true
	if ch.RenameDue(now.Add(24*time.Hour), every, soon) {
		t.Fatal("asked a channel that is on the air")
	}
}
