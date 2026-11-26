package chaturbate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HeapOfChaos/goondvr/entity"
	"github.com/HeapOfChaos/goondvr/internal"
	"github.com/HeapOfChaos/goondvr/server"
)

// The old address of a renamed account sends the page to the new name, and the new name is a real room.
func TestResolveRenameFollowsTheRedirectOfTheOldAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/old/":
			http.Redirect(w, r, "/new/", http.StatusMovedPermanently)
		case r.URL.Path == "/lobby/": // e.g. the age check or the home page: not a rename
			http.Redirect(w, r, "/", http.StatusFound)
		case r.URL.Path == "/tags/": // a real page of the site that is not a room
			http.Redirect(w, r, "/female-cams/", http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/api/chatvideocontext/new/"):
			w.Write([]byte(`{"room_status":"offline","hls_source":""}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := server.Config
	server.Config = &entity.Config{Domain: srv.URL + "/"}
	defer func() { server.Config = old }()

	cb := New()
	for user, want := range map[string]string{"old": "new", "lobby": "", "tags": "", "plain": ""} {
		got, err := cb.ResolveRename(context.Background(), internal.NewReq(), user)
		if err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", user, got, err, want)
		}
	}
}
