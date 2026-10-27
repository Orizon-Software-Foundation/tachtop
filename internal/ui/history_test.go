package ui

import (
	"math"
	"testing"
)

func TestSeriesAddClampsBadValues(t *testing.T) {
	s := newSeries(10)
	s.add(math.NaN())
	s.add(math.Inf(1))
	s.add(-5)
	got := s.data()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, v := range got {
		if v != 0 {
			t.Errorf("data[%d] = %v, want 0", i, v)
		}
	}
}

func TestSeriesRespectsLimit(t *testing.T) {
	s := newSeries(3)
	for i := 1; i <= 5; i++ {
		s.add(float64(i))
	}
	got := s.data()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []float64{3, 4, 5}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("data[%d] = %v, want %v", i, got[i], v)
		}
	}
}

func TestSeriesNewSeriesZeroLimit(t *testing.T) {
	s := newSeries(0)
	if s.limit != 1 {
		t.Fatalf("limit = %d, want 1", s.limit)
	}
	s.add(42)
	if got := s.data(); len(got) != 1 || got[0] != 42 {
		t.Fatalf("data = %v", got)
	}
}
