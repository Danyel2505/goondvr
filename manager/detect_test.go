package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HeapOfChaos/goondvr/channel"
	"github.com/HeapOfChaos/goondvr/detect"
	"github.com/HeapOfChaos/goondvr/entity"
	"github.com/HeapOfChaos/goondvr/server"
)

func TestValidateDetect(t *testing.T) {
	ok := entity.DetectConfig{Enabled: true, Model: "mi-modelo_1", Labels: []string{"show"}, MinScore: 0.5, Interval: 10, Idle: 60, Pre: 5, Post: 10}
	if err := validateDetect(&ok); err != nil {
		t.Fatalf("a good setting was refused: %v", err)
	}
	if err := validateDetect(&entity.DetectConfig{}); err != nil {
		t.Fatalf("detection that is off needs nothing: %v", err)
	}
	cases := map[string]func(d *entity.DetectConfig){
		"no model":                func(d *entity.DetectConfig) { d.Model = "" },
		"model with a path":       func(d *entity.DetectConfig) { d.Model = "../x" },
		"unknown source":          func(d *entity.DetectConfig) { d.Source = "screen" },
		"gate on the recording":   func(d *entity.DetectConfig) { d.Gate, d.Source = true, "stream" },
		"empty label":             func(d *entity.DetectConfig) { d.Labels = []string{""} },
		"long label":              func(d *entity.DetectConfig) { d.Labels = []string{strings.Repeat("a", 65)} },
		"too many labels":         func(d *entity.DetectConfig) { d.Labels = make([]string, 31) },
		"score over 1":            func(d *entity.DetectConfig) { d.MinScore = 1.5 },
		"negative score":          func(d *entity.DetectConfig) { d.MinScore = -0.1 },
		"interval too short":      func(d *entity.DetectConfig) { d.Interval = 1 },
		"interval too long":       func(d *entity.DetectConfig) { d.Interval = 4000 },
		"idle too short":          func(d *entity.DetectConfig) { d.Idle = 4 },
		"clip padding too long":   func(d *entity.DetectConfig) { d.Pre = 301 },
		"negative clip padding":   func(d *entity.DetectConfig) { d.Post = -1 },
		"gate on, model missing":  func(d *entity.DetectConfig) { d.Gate, d.Model = true, "" },
		"model with a space name": func(d *entity.DetectConfig) { d.Model = "a b" },
	}
	for name, mut := range cases {
		d := ok
		d.Labels = append([]string(nil), ok.Labels...)
		mut(&d)
		if validateDetect(&d) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValidateDetectURL(t *testing.T) {
	for _, tc := range []struct {
		url   string
		https bool
		good  bool
	}{
		{"", true, true},
		{"http://127.0.0.1:8099", false, true},
		{"https://abc.trycloudflare.com", true, true},
		{"http://abc.trycloudflare.com", true, false}, // the token would travel in the clear
		{"ftp://x", false, false},
		{"127.0.0.1:8099", false, false},
		{"http://", false, false},
	} {
		if err := ValidateDetectURL(tc.url, tc.https); (err == nil) != tc.good {
			t.Errorf("ValidateDetectURL(%q, %v) = %v", tc.url, tc.https, err)
		}
	}
}

func TestDetectTargetPicksTheBackend(t *testing.T) {
	c := &entity.Config{DetectToken: "t", DetectColabURL: "https://c.example", DetectLocalURL: " "}
	if got := detectTarget(c); got.URL != DefaultDetectLocalURL || got.Token != "t" {
		t.Fatalf("local default: %+v", got)
	}
	c.DetectLocalURL = "http://127.0.0.1:9"
	if got := detectTarget(c); got.URL != "http://127.0.0.1:9" {
		t.Fatalf("local: %+v", got)
	}
	c.DetectBackend = "colab"
	if got := detectTarget(c); got.URL != "https://c.example" {
		t.Fatalf("colab: %+v", got)
	}
}

func TestDetectModelsDoesNotSendTheTokenToAColabAddressWithoutHTTPS(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer ts.Close()
	old := server.Config
	server.Config = &entity.Config{}
	defer func() { server.Config = old }()

	resp := (&Manager{}).DetectModels(context.Background(), &server.DetectSettings{Backend: "colab", ColabURL: ts.URL, Token: "secret"})
	if resp.OK || !strings.Contains(resp.Error, "https") {
		t.Fatalf("a plain http Colab address was accepted: %+v", resp)
	}
	if hits != 0 {
		t.Fatalf("the detector was asked %d time(s) over plain http", hits)
	}
}

func matchOf(label string) *detect.Result {
	return &detect.Result{Scores: map[string]float64{label: 0.9}}
}

func TestGateStartsOnAMatchAndPausesAfterTheIdleTime(t *testing.T) {
	m, _, ch := autoRig(t,
		&entity.ChannelConfig{Username: "gated", IsPaused: true, Detect: &entity.DetectConfig{Enabled: true, Model: "m", Gate: true, Idle: 1}},
		&entity.ChannelConfig{Username: "off", IsPaused: true, Detect: &entity.DetectConfig{Enabled: false, Model: "m", Gate: true}},
	)
	g := ch["gated"]
	for _, c := range ch {
		c.ApplyPresence(live())
	}
	m.autoStep()
	time.Sleep(30 * time.Millisecond)
	if !g.Paused() {
		t.Fatal("on the air but nothing matched: the gate must hold the recording back")
	}
	if !ch["off"].Paused() {
		t.Fatal("a gate whose detection is off must do nothing")
	}

	g.DetectDone(g.DetectConfig(), matchOf("show"), nil) // a picture matched
	m.autoStep()
	waitFor(t, "the gate to open", func() bool { return !g.Paused() })
	if !g.AutoStarted() {
		t.Fatal("the start must be remembered as automatic")
	}
	if snap := g.ConfigSnapshot(); !snap.IsPaused {
		t.Fatalf("what is saved is what the user chose (paused, waiting for a match): %+v", snap)
	}

	time.Sleep(1100 * time.Millisecond) // the idle time (1 s) passes without another match
	g.ApplyPresence(live())             // (the fake site says offline once the monitor looks; the presence list says on the air)
	m.autoStep()
	waitFor(t, "the gate to close", func() bool { return g.Paused() })
	if g.AutoStarted() {
		t.Fatal("it is waiting for a match again, not 'started'")
	}

	g.DetectDone(g.DetectConfig(), matchOf("show"), nil) // and the next match starts it again
	g.ApplyPresence(live())
	m.autoStep()
	waitFor(t, "the second start", func() bool { return !g.Paused() })
}

func TestDetectLoopFeedsTheGateFromThePreviewAndTheDetector(t *testing.T) {
	jpg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xd9")
	thumbs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(jpg) }))
	defer thumbs.Close()
	var asked atomic.Int32
	var gotAuth, gotModel atomic.Value
	det := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		gotAuth.Store(r.Header.Get("Authorization"))
		gotModel.Store(r.URL.Query().Get("model"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"scores":{"show":0.93,"other":0.05},"model":"m","kind":"embed","provider":"CPU","ms":3}`))
	}))
	defer det.Close()

	oldBase, oldTick := channel.ThumbBase, detectTick
	channel.ThumbBase, detectTick = thumbs.URL+"/", 20*time.Millisecond
	defer func() { channel.ThumbBase, detectTick = oldBase, oldTick }()

	m, _, ch := autoRig(t, &entity.ChannelConfig{Username: "gated", IsPaused: true,
		Detect: &entity.DetectConfig{Enabled: true, Model: "m", Gate: true, Interval: 1, Labels: []string{"show"}}})
	server.Config.DetectBackend, server.Config.DetectLocalURL, server.Config.DetectToken = "local", det.URL, "secreto"
	g := ch["gated"]
	g.ApplyPresence(live())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.detectLoop(ctx)

	waitFor(t, "the recording to start after the first match", func() bool { return !g.Paused() })
	if gotAuth.Load() != "Bearer secreto" || gotModel.Load() != "m" {
		t.Fatalf("the detector got auth %v, model %v", gotAuth.Load(), gotModel.Load())
	}
	if info := g.ExportInfo().Detect; !info.On || !info.Matching || info.Label != "show" {
		t.Fatalf("the card shows %+v", info)
	}
}

func TestDetectLoopShowsAnUnreachableDetectorOnTheCardAndKeepsTheChannelPaused(t *testing.T) {
	jpg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xd9")
	thumbs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(jpg) }))
	defer thumbs.Close()
	det := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
	}))
	defer det.Close()

	oldBase, oldTick := channel.ThumbBase, detectTick
	channel.ThumbBase, detectTick = thumbs.URL+"/", 20*time.Millisecond
	defer func() { channel.ThumbBase, detectTick = oldBase, oldTick }()

	m, _, ch := autoRig(t, &entity.ChannelConfig{Username: "gated", IsPaused: true,
		Detect: &entity.DetectConfig{Enabled: true, Model: "nope", Gate: true, Interval: 2}})
	server.Config.DetectBackend, server.Config.DetectLocalURL = "local", det.URL
	g := ch["gated"]
	g.ApplyPresence(live())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.detectLoop(ctx)

	waitFor(t, "the error on the card", func() bool { return strings.Contains(g.ExportInfo().Detect.Error, "model not found") })
	if !g.Paused() {
		t.Fatal("a detector that fails must not start the recording")
	}
}
