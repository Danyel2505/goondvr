package view

import (
	"bytes"
	"strings"
	"testing"

	"github.com/HeapOfChaos/goondvr/entity"
)

func render(t *testing.T, info entity.ChannelInfo, mode string) string {
	t.Helper()
	info.ViewMode = mode
	var b bytes.Buffer
	if err := InfoTpl.ExecuteTemplate(&b, "channel_details", &info); err != nil {
		t.Fatalf("%s: %v", mode, err)
	}
	return b.String()
}

func TestEveryViewShowsTheLastConnection(t *testing.T) {
	off := entity.ChannelInfo{Username: "ana", Site: "chaturbate", ChannelID: "chaturbate__ana", LastSeen: "2026-11-27 15:04"}
	never := entity.ChannelInfo{Username: "ana", Site: "chaturbate", ChannelID: "chaturbate__ana"}
	live := entity.ChannelInfo{Username: "ana", Site: "chaturbate", ChannelID: "chaturbate__ana", IsAvailable: true, LastSeen: "2026-11-27 15:04"}
	for _, mode := range []string{"expanded", "grid", "collapsed"} {
		if out := render(t, off, mode); !strings.Contains(out, "2026-11-27 15:04") {
			t.Errorf("%s: an offline channel must show its last connection", mode)
		}
		if out := render(t, live, mode); strings.Contains(out, "2026-11-27 15:04") {
			t.Errorf("%s: a channel that is on air shows \"now\", not an old date", mode)
		}
		if out := render(t, never, mode); strings.Contains(out, "Last seen: <") {
			t.Errorf("%s: never seen must say so, not print an empty date", mode)
		}
	}
	if !strings.Contains(render(t, live, "expanded"), "Online now") || !strings.Contains(render(t, never, "expanded"), "Never seen yet") {
		t.Error("expanded view: wrong text for a channel that is on air / never seen")
	}
	if !strings.Contains(render(t, live, "grid"), "Last seen: now") || !strings.Contains(render(t, never, "grid"), "Last seen: never") {
		t.Error("grid view: wrong text for a channel that is on air / never seen")
	}
}
