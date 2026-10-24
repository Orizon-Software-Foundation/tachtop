package ui

import (
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/Orizon-Software-Foundation/tachtop/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func TestResample(t *testing.T) {
	got := resample(nil, 4)
	if len(got) != 4 {
		t.Fatalf("len = %d", len(got))
	}
	for _, v := range got {
		if v != 0 {
			t.Fatalf("empty resample = %v", got)
		}
	}

	got = resample([]float64{7}, 3)
	for _, v := range got {
		if v != 7 {
			t.Fatalf("single value resample = %v", got)
		}
	}

	got = resample([]float64{0, 10}, 5)
	want := []float64{0, 2.5, 5, 7.5, 10}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("resample = %v, want %v", got, want)
		}
	}

	in := []float64{1, 2, 3}
	got = resample(in, 3)
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("identity resample = %v", got)
		}
	}
}

func TestAutoMax(t *testing.T) {
	if got := AutoMax(nil, 10); got != 11.5 {
		t.Errorf("AutoMax(empty) = %v", got)
	}
	if got := AutoMax([]float64{5}, 10); got != 11.5 {
		t.Errorf("AutoMax below floor = %v", got)
	}
	if got := AutoMax([]float64{50}, 10); math.Abs(got-57.5) > 1e-9 {
		t.Errorf("AutoMax above floor = %v", got)
	}
	if got := AutoMax([]float64{math.NaN(), 20, math.Inf(1)}, 10); got != 23 {
		t.Errorf("AutoMax should ignore NaN/Inf, got %v", got)
	}
}

func TestRenderGraph(t *testing.T) {
	th, _ := theme.Get("tachtop")
	lines := RenderGraph([]float64{10, 30, 50}, 20, 4, 100, th.Ramp)
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != 20 {
			t.Errorf("line %d width = %d, want 20", i, w)
		}
	}
	if plain(lines[3]) == strings.Repeat(" ", 20) {
		t.Error("lowest graph row should not be blank")
	}

	if got := RenderGraph(nil, 0, 4, 10, th.Ramp); got != nil {
		t.Errorf("zero width should return nil, got %v", got)
	}
	if got := RenderGraph(nil, 4, 0, 10, th.Ramp); got != nil {
		t.Errorf("zero height should return nil, got %v", got)
	}

	empty := RenderGraph(nil, 10, 2, 10, th.Ramp)
	for i, line := range empty {
		if plain(line) != strings.Repeat(" ", 10) {
			t.Errorf("empty graph row %d = %q", i, plain(line))
		}
	}

	single := RenderGraph([]float64{50}, 10, 2, 100, th.Ramp)
	if plain(single[1])[0] != ' ' {
		t.Errorf("single sample should not span the graph: %q", plain(single[1]))
	}
	if plain(single[1])[9] == ' ' {
		t.Errorf("single sample should appear at the right edge: %q", plain(single[1]))
	}
}

func TestGauge(t *testing.T) {
	th, _ := theme.Get("tachtop")

	empty := plain(Gauge(0, 10, th))
	if empty != strings.Repeat("░", 10) {
		t.Errorf("Gauge(0) = %q", empty)
	}
	full := plain(Gauge(100, 10, th))
	if full != strings.Repeat("█", 10) {
		t.Errorf("Gauge(100) = %q", full)
	}
	half := plain(Gauge(50, 10, th))
	if lipgloss.Width(half) != 10 || !strings.ContainsRune(half, '█') || !strings.ContainsRune(half, '░') {
		t.Errorf("Gauge(50) = %q", half)
	}
	clamped := plain(Gauge(250, 8, th))
	if clamped != strings.Repeat("█", 8) {
		t.Errorf("Gauge clamps high values: %q", clamped)
	}
	if got := Gauge(50, 0, th); got != "" {
		t.Errorf("Gauge zero width = %q", got)
	}
}
