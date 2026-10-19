package manager

import (
	"testing"
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
	d := probeSpacing(40, false)
	if d < availMinSpacing || d > availMaxSpacing {
		t.Fatalf("40 channels: %v out of bounds", d)
	}
	if round := d * 40; round < availCycle || round > availCycle*3/2 {
		t.Fatalf("40 channels: a full round takes %v, want about %v", round, availCycle)
	}
	if d := probeSpacing(0, false); d <= 0 {
		t.Fatalf("no channels: spacing %v", d)
	}
}
