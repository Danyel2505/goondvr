package manager

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/HeapOfChaos/goondvr/channel"
	"github.com/HeapOfChaos/goondvr/entity"
	"github.com/HeapOfChaos/goondvr/internal"
	"github.com/HeapOfChaos/goondvr/site"
)

// Renamed accounts. Streamers sometimes change the name of their account; the site then sends the old address
// to the new one, and the channel in this program stops working under its old name. A few channels at a time
// (a request each, so it costs next to nothing), the ones that are not on the air are asked whether the site
// sends them somewhere else (site.Renamer). If it does, to an account that exists, the channel takes the new
// name and keeps the old ones in its history, which the card shows.

var (
	renameTick    = 5 * time.Minute // how often the channels are looked at
	renamePerTick = 3               // channels asked per look: a slow trickle, 36 an hour
	renameEvery   = 6 * time.Hour   // a channel that is off the air is asked this often
	renameSoon    = time.Hour       // a channel the site said was not found, after this
	renameNow     = time.Now        // (tests replace it)

	// renamerFor is the site that can tell about a rename (tests replace it).
	renamerFor = func(siteName string) (site.Renamer, bool) {
		r, ok := channel.ResolveSite(siteName).(site.Renamer)
		return r, ok
	}
)

// renamedEvent tells the web pages to redraw: the card of a renamed channel has a new id.
const renamedEvent = "renamed"

func (m *Manager) renameLoop(ctx context.Context) {
	req := internal.NewReq()
	t := time.NewTicker(renameTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		m.renameStep(ctx, req)
	}
}

// renameStep asks about the channels that are due, at most renamePerTick of them.
func (m *Manager) renameStep(ctx context.Context, req *internal.Req) {
	now := renameNow()
	var due []*channel.Channel
	m.Channels.Range(func(_, v any) bool {
		ch := v.(*channel.Channel)
		if ch.RenameDue(now, renameEvery, renameSoon) && len(due) < renamePerTick {
			due = append(due, ch)
		}
		return true // goes on: the first look at a channel also schedules its first question
	})
	for _, ch := range due {
		if ctx.Err() != nil {
			return
		}
		m.checkRename(ctx, req, ch)
	}
}

func (m *Manager) checkRename(ctx context.Context, req *internal.Req, ch *channel.Channel) {
	ch.RenameChecked(renameNow())
	r, ok := renamerFor(ch.Config.Site)
	if !ok {
		return
	}
	name, err := r.ResolveRename(ctx, req, ch.Config.Username)
	if err != nil || name == "" {
		return
	}
	if err := m.RenameChannel(entity.ChannelID(ch.Config.Site, ch.Config.Username), name); err != nil {
		log.Printf("WARN  [%s] the site now calls this account %s, but the channel was not renamed: %v", ch.Config.Username, name, err)
	}
}

// RenameChannel gives a channel a new name, keeping its settings and adding the change to its history. The
// channel is replaced by a new one (its name is read without a lock everywhere, so it never changes in
// place), and if it was running it keeps running under the new name. The old recordings stay where they are.
func (m *Manager) RenameChannel(oldID, newName string) error {
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()

	v, ok := m.Channels.Load(oldID)
	if !ok {
		return fmt.Errorf("channel %s does not exist", oldID)
	}
	old := v.(*channel.Channel)
	cfg := old.ConfigSnapshot()
	oldName := cfg.Username
	cfg.Username = newName
	cfg.Sanitize()
	if cfg.Username == "" || cfg.Username != newName {
		return fmt.Errorf("%q is not a valid name", newName)
	}
	newID := entity.ChannelID(cfg.Site, cfg.Username)
	if _, exists := m.Channels.Load(newID); exists {
		return fmt.Errorf("a channel called %s already exists", cfg.Username)
	}

	cfg.NameHistory = append(append([]entity.NameChange(nil), cfg.NameHistory...), entity.NameChange{From: oldName, To: cfg.Username, At: renameNow().Unix()})
	if cfg.Detect != nil { // not shared with the old channel, which is still winding down
		d := *cfg.Detect
		d.Labels = append([]string(nil), d.Labels...)
		cfg.Detect = &d
	}

	old.Stop()
	m.Channels.Delete(oldID)
	nc := channel.New(&cfg)
	m.Channels.Store(newID, nc)
	if !cfg.IsPaused {
		go nc.Resume(0)
	}
	m.kickPresence()
	log.Printf(" INFO [%s] the account was renamed on the site: %s -> %s; the channel now uses the new name (recordings already made keep the old one)", oldName, oldName, cfg.Username)

	err := m.SaveConfig()
	m.hub.publish(hubMessage{event: renamedEvent, data: []byte(newID)})
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
