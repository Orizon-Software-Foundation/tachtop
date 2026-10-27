package ui

import "testing"

func TestClampInt(t *testing.T) {
	tests := []struct {
		v, lo, hi, want int
	}{
		{5, 0, 10, 5},
		{-3, 0, 10, 0},
		{15, 0, 10, 10},
		{5, 10, 0, 10}, // inverted bounds clamp to lo
	}
	for _, tt := range tests {
		if got := clampInt(tt.v, tt.lo, tt.hi); got != tt.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestMinMaxInt(t *testing.T) {
	tests := []struct {
		a, b, min, max int
	}{
		{3, 7, 3, 7},
		{7, 3, 3, 7},
		{5, 5, 5, 5},
	}
	for _, tt := range tests {
		if got := minInt(tt.a, tt.b); got != tt.min {
			t.Errorf("minInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.min)
		}
		if got := maxInt(tt.a, tt.b); got != tt.max {
			t.Errorf("maxInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.max)
		}
	}
}

func TestGpuPanelHeight(t *testing.T) {
	tests := []struct {
		count, maxRows, want int
	}{
		{0, 5, 0},
		{3, 0, 0},
		{3, 5, 5}, // 2 + min(3, 5)
		{8, 3, 5}, // 2 + min(8, 3)
		{1, 1, 3},
	}
	for _, tt := range tests {
		if got := gpuPanelHeight(tt.count, tt.maxRows); got != tt.want {
			t.Errorf("gpuPanelHeight(%d, %d) = %d, want %d", tt.count, tt.maxRows, got, tt.want)
		}
	}
}
