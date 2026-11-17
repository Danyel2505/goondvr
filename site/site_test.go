package site

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HeapOfChaos/goondvr/entity"
	"github.com/HeapOfChaos/goondvr/internal"
	"github.com/HeapOfChaos/goondvr/server"
)

func TestRenameFromRedirect(t *testing.T) {
	old := server.Config
	server.Config = &entity.Config{}
	defer func() { server.Config = old }()
	redirects := map[string]string{
		"/old/":    "/new_name/",                  // the real case
		"/abs/":    "ABS/new-name/",               // absolute address on the same host (filled in below)
		"/case/":   "/Case/",                      // only the case changes: not a rename
		"/self/":   "/self",                       // the same name
		"/login/":  "/auth/login/?next=/login/",   // sent to a login page
		"/home/":   "/",                           // sent to the home page
		"/age/":    "/a",                          // too short to be an account
		"/other/":  "http://elsewhere.invalid/x/", // another host
		"/ghost/":  "/ghost2/",                    // leads to something that is not an account
		"/status/": "",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		to, ok := redirects[r.URL.Path]
		switch {
		case !ok || to == "":
			w.Write([]byte("hello"))
		case to == "ABS/new-name/":
			http.Redirect(w, r, "http://"+r.Host+"/new-name/", http.StatusMovedPermanently)
		default:
			http.Redirect(w, r, to, http.StatusFound)
		}
	}))
	defer srv.Close()

	exists := func(_ context.Context, name string) error {
		if name == "ghost2" {
			return errors.New("not found")
		}
		return nil
	}
	for _, tc := range []struct{ path, user, want string }{
		{"/old/", "old", "new_name"},
		{"/abs/", "abs", "new-name"},
		{"/case/", "case", ""},
		{"/self/", "self", ""},
		{"/login/", "login", ""},
		{"/home/", "home", ""},
		{"/age/", "age", ""},
		{"/other/", "other", ""},
		{"/ghost/", "ghost", ""},
		{"/status/", "status", ""},
		{"/missing/", "missing", ""},
	} {
		got, err := RenameFromRedirect(context.Background(), internal.NewReq(), srv.URL+tc.path, tc.user, exists)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.path, got, err, tc.want)
		}
	}

	// a site that does not answer is "could not tell", never a rename
	srv.Close()
	if got, err := RenameFromRedirect(context.Background(), internal.NewReq(), srv.URL+"/old/", "old", exists); err == nil || got != "" {
		t.Errorf("closed server: got %q, %v", got, err)
	}
}
