package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HeapOfChaos/goondvr/server"
	"github.com/gin-gonic/gin"
)

type fakeManager struct {
	server.IManager
	paused, resumed []string
}

func (f *fakeManager) PauseChannel(id string) error  { f.paused = append(f.paused, id); return nil }
func (f *fakeManager) ResumeChannel(id string) error { f.resumed = append(f.resumed, id); return nil }

func newTestRouter(t *testing.T) (*gin.Engine, *fakeManager) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	old := server.Manager
	fm := &fakeManager{}
	server.Manager = fm
	t.Cleanup(func() { server.Manager = old })

	r := gin.New()
	r.POST("/pause_channel/:channelID", PauseChannel)
	r.POST("/resume_channel/:channelID", ResumeChannel)
	return r, fm
}

func do(r *gin.Engine, path string, htmx bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The htmx buttons must not trigger a download of the whole dashboard.
func TestPauseResumeFromHtmxGetNoContent(t *testing.T) {
	r, fm := newTestRouter(t)

	for _, path := range []string{"/pause_channel/chaturbate__a", "/resume_channel/chaturbate__a"} {
		w := do(r, path, true)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: status %d, want 204", path, w.Code)
		}
		if loc := w.Header().Get("Location"); loc != "" {
			t.Fatalf("%s: unexpected redirect to %q", path, loc)
		}
		if w.Body.Len() != 0 {
			t.Fatalf("%s: body should be empty, got %d bytes", path, w.Body.Len())
		}
	}
	if len(fm.paused) != 1 || len(fm.resumed) != 1 {
		t.Fatalf("handlers did not reach the manager: paused=%v resumed=%v", fm.paused, fm.resumed)
	}
}

// ...while ordinary form posts keep redirecting to the dashboard.
func TestPauseResumeFromPlainPostStillRedirect(t *testing.T) {
	r, _ := newTestRouter(t)

	for _, path := range []string{"/pause_channel/chaturbate__a", "/resume_channel/chaturbate__a"} {
		w := do(r, path, false)
		if w.Code != http.StatusFound || w.Header().Get("Location") != "/" {
			t.Fatalf("%s: status %d Location %q, want 302 to /", path, w.Code, w.Header().Get("Location"))
		}
	}
}
