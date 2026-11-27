package channel

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HeapOfChaos/goondvr/detect"
	"github.com/HeapOfChaos/goondvr/entity"
)

func detectChannel(t *testing.T, d *entity.DetectConfig) *Channel {
	t.Helper()
	ch := newTestChannel(t, &fakeUI{pushes: map[string]int{}}, time.Second)
	ch.Config.Detect = d
	return ch
}

func hit(label string, score float64) *detect.Result {
	return &detect.Result{Scores: map[string]float64{label: score, "other": 0.1}}
}

func TestDetectDueRules(t *testing.T) {
	now := time.Now()
	off := detectChannel(t, nil)
	off.Available = true
	if _, due := off.DetectDue(now); due {
		t.Fatal("a channel without detection was due")
	}
	noModel := detectChannel(t, &entity.DetectConfig{Enabled: true})
	noModel.Available = true
	if _, due := noModel.DetectDue(now); due {
		t.Fatal("a channel without a model was due")
	}

	ch := detectChannel(t, &entity.DetectConfig{Enabled: true, Model: "m"})
	if _, due := ch.DetectDue(now); due {
		t.Fatal("a channel that is off the air was due")
	}
	ch.Available = true
	if cfg, due := ch.DetectDue(now); !due || cfg.Source != "thumb" || cfg.Interval != 10 {
		t.Fatalf("on the air with the preview as source: due=%v cfg=%+v", due, cfg)
	}
	if _, due := ch.DetectDue(now); due {
		t.Fatal("a channel that is being checked was due again")
	}
	ch.DetectDone(ch.DetectConfig(), hit("a", 0.9), nil)
	if _, due := ch.DetectDue(now); due {
		t.Fatal("due before the interval had passed")
	}
	if _, due := ch.DetectDue(now.Add(time.Minute)); !due {
		t.Fatal("not due after the interval")
	}

	// the recording as source: only while it is being recorded
	st := detectChannel(t, &entity.DetectConfig{Enabled: true, Model: "m", Source: "stream"})
	st.Available = true
	if _, due := st.DetectDue(now); due {
		t.Fatal("source stream, not recording, was due")
	}
	st.IsOnline = true
	if _, due := st.DetectDue(now); !due {
		t.Fatal("source stream, recording, was not due")
	}
	st.DetectRelease()
	if _, due := st.DetectDue(now); due {
		t.Fatal("a released channel is not due at once (it waits a second)")
	}
}

func TestDetectDoneKeepsTheMatchesAsSpansOfTheFile(t *testing.T) {
	ch := detectChannel(t, &entity.DetectConfig{Enabled: true, Model: "m", Labels: []string{"show"}, MinScore: 0.5})
	cfg := ch.DetectConfig()
	f, err := os.Create(filepath.Join(t.TempDir(), "rec.ts"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	step := func(dur float64, res *detect.Result, err error) bool {
		ch.fileMu.Lock()
		ch.Duration = dur
		ch.File = f // a file is open
		ch.fileMu.Unlock()
		ch.det.mu.Lock()
		ch.det.busy = true
		ch.det.mu.Unlock()
		return ch.DetectDone(cfg, res, err)
	}
	if !step(10, hit("show", 0.8), nil) {
		t.Fatal("the first match must change the card")
	}
	if step(20, hit("show", 0.9), nil) {
		t.Fatal("another match must not change the card")
	}
	step(30, hit("show", 0.7), nil)
	if !step(40, hit("show", 0.2), nil) { // below the minimum: not a match
		t.Fatal("the end of the match must change the card")
	}
	step(50, hit("other", 0.99), nil) // not one of the labels
	step(60, hit("show", 0.6), nil)   // a second stretch

	ch.det.mu.Lock()
	spans := append([]detect.Span(nil), ch.det.spans...)
	ch.det.mu.Unlock()
	if len(spans) != 2 || spans[0] != (detect.Span{Start: 10, End: 30, Label: "show", Score: 0.9}) || spans[1].Start != 60 {
		t.Fatalf("spans = %+v", spans)
	}
	info := ch.detectInfo(cfg)
	if !info.On || !info.Matching || info.Label != "show" {
		t.Fatalf("card: %+v", info)
	}
}

func TestDetectSpanStartsWhereThePictureWasTakenNotWhenTheAnswerArrives(t *testing.T) {
	jpg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xd9")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(jpg) }))
	defer srv.Close()
	old := ThumbBase
	ThumbBase = srv.URL + "/"
	defer func() { ThumbBase = old }()

	ch := detectChannel(t, &entity.DetectConfig{Enabled: true, Model: "m", Labels: []string{"show"}, MinScore: 0.5})
	cfg := ch.DetectConfig()
	f, err := os.Create(filepath.Join(t.TempDir(), "rec.ts"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	setDuration := func(d float64) {
		ch.fileMu.Lock()
		ch.Duration, ch.File = d, f
		ch.fileMu.Unlock()
	}
	check := func(takenAt, answeredAt float64, match bool) {
		t.Helper()
		setDuration(takenAt)
		ch.det.mu.Lock()
		ch.det.busy = true
		ch.det.mu.Unlock()
		if _, err := ch.DetectFrame(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		setDuration(answeredAt) // the recording went on while the detector thought
		res := hit("show", 0.9)
		if !match {
			res = hit("other", 0.9)
		}
		ch.DetectDone(cfg, res, nil)
	}
	spansNow := func() []detect.Span {
		ch.det.mu.Lock()
		defer ch.det.mu.Unlock()
		return append([]detect.Span(nil), ch.det.spans...)
	}
	check(10, 14, true)
	check(12, 16, true)
	check(14, 18, false)
	check(30, 33, true)
	if sp := spansNow(); len(sp) != 2 || sp[0].Start != 10 || sp[0].End != 12 || sp[1].Start != 30 || sp[1].End != 30 {
		t.Fatalf("spans = %+v", sp)
	}

	// the recording moved to a new file while a picture was being checked: a position of the old file means nothing
	setDuration(50)
	ch.det.mu.Lock()
	ch.det.busy = true
	ch.det.mu.Unlock()
	if _, err := ch.DetectFrame(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	ch.fileMu.Lock()
	ch.detectFileOpened() // the match goes on into the new file, which starts at 0
	ch.Duration = 3
	ch.fileMu.Unlock()
	ch.DetectDone(cfg, hit("show", 0.9), nil)
	if sp := spansNow(); len(sp) != 1 || sp[0].Start != 0 || sp[0].End != 3 {
		t.Fatalf("spans of the new file = %+v", sp)
	}
}

func TestDetectDoneBacksOffOnErrorsAndRecovers(t *testing.T) {
	ch := detectChannel(t, &entity.DetectConfig{Enabled: true, Model: "m", Interval: 10})
	cfg := ch.DetectConfig()
	for i, want := range []time.Duration{20 * time.Second, 40 * time.Second, 80 * time.Second} {
		ch.det.mu.Lock()
		ch.det.busy = true
		ch.det.mu.Unlock()
		changed := ch.DetectDone(cfg, nil, errors.New("down"))
		if changed != (i == 0) {
			t.Fatalf("failure %d: changed=%v (the card changes only when the message does)", i, changed)
		}
		ch.det.mu.Lock()
		wait := time.Until(ch.det.nextAt)
		ch.det.mu.Unlock()
		if wait < want-2*time.Second || wait > want+time.Second {
			t.Fatalf("failure %d: waits %v, want about %v", i, wait, want)
		}
	}
	if info := ch.detectInfo(cfg); info.Error != "down" {
		t.Fatalf("card: %+v", info)
	}
	if !ch.DetectDone(cfg, hit("a", 0.9), nil) || ch.detectInfo(cfg).Error != "" {
		t.Fatal("a good answer must clear the error")
	}
	if backoff(10, 100) != detectMaxBackoff {
		t.Fatal("the wait must be capped")
	}
}

func TestDetectKeepsOnlyTheNewestSegmentsAndOnlyWhenAsked(t *testing.T) {
	ch := detectChannel(t, nil)
	n := func() int { s, _ := ch.detectSegments(); return len(s) }
	ch.detectKeep([]byte("ignored"), 0)
	if n() != 0 {
		t.Fatal("kept a segment nobody asked for")
	}
	ch.detectReset(true)
	for i := 0; i < 20; i++ {
		ch.detectKeep([]byte{byte(i)}, float64(i*2))
	}
	segs, keyAt := ch.detectSegments()
	if len(segs) != detectMaxSegs || segs[len(segs)-1][0] != 19 {
		t.Fatalf("kept %d segments, the last is %v", len(segs), segs[len(segs)-1])
	}
	if keyAt != 38 { // the newest segment starts at 38 s: that is where the key frame that gets looked at is
		t.Fatalf("the newest segment starts at %v, want 38", keyAt)
	}
	big := make([]byte, detectMaxSegBytes)
	ch.detectKeep(big, 40)
	if n() != 1 {
		t.Fatalf("a segment over the byte cap must push the others out, %d kept", n())
	}
	ch.detectReset(false)
	if s, at := ch.detectSegments(); len(s) != 0 || at != 0 {
		t.Fatal("turning the stream source off must free the segments")
	}
}

func TestDetectFrameFromThePreviewAddress(t *testing.T) {
	jpg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xd9")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tester.jpg":
			w.Write(jpg)
		case "/html.jpg":
			w.Write([]byte("<html>no</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := ThumbBase
	ThumbBase = srv.URL + "/"
	defer func() { ThumbBase = old }()

	ch := detectChannel(t, &entity.DetectConfig{Enabled: true, Model: "m"})
	got, err := ch.DetectFrame(context.Background(), ch.DetectConfig())
	if err != nil || !bytes.Equal(got, jpg) {
		t.Fatalf("got %d bytes, err %v", len(got), err)
	}
	ch.Config.Username = "html"
	if _, err := ch.DetectFrame(context.Background(), ch.DetectConfig()); err == nil {
		t.Fatal("a page that is not a picture was accepted")
	}
	ch.Config.Username = "missing"
	if _, err := ch.DetectFrame(context.Background(), ch.DetectConfig()); err == nil {
		t.Fatal("a 404 was accepted")
	}
	// the recording as the source, with nothing recorded yet
	st := entity.DetectConfig{Enabled: true, Model: "m", Source: "stream"}
	if _, err := ch.DetectFrame(context.Background(), st.WithDefaults()); !errors.Is(err, ErrNoFrameYet) {
		t.Fatalf("err = %v, want ErrNoFrameYet", err)
	}
}
