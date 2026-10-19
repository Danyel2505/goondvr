package manager

import (
	"testing"
	"time"
)

func TestProbeSpacingStaysLightAndAdapts(t *testing.T) {
	if d := probeSpacing(3, true); d != availFirstSpacing {
		t.Fatalf("first round: want %v, got %v", availFirstSpacing, d)
	}
	if d := probeSpacing(1000, false); d != availMinSpacing {
		t.Fatalf("1000 paused channels: never faster than %v, got %v", availMinSpacing, d)
	}
	if d := probeSpacing(2, false); d != availMaxSpacing {
		t.Fatalf("2 paused channels: never slower than %v, got %v", availMaxSpacing, d)
	}
	d := probeSpacing(60, false)
	if d < availMinSpacing || d > availMaxSpacing {
		t.Fatalf("60 channels: %v out of bounds", d)
	}
	if round := d * 60; round < 100*time.Second || round > 140*time.Second {
		t.Fatalf("60 channels: a full round takes %v, want about %v", round, availCycle)
	}
	if d := probeSpacing(0, false); d <= 0 {
		t.Fatalf("no channels: spacing %v", d)
	}
}
