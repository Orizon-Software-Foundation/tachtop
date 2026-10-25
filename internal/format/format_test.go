package format

import (
	"strings"
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{10 * 1024, "10 KiB"},
		{100 * 1024 * 1024, "100 MiB"},
		{15 * 1024 * 1024 * 1024, "15.0 GiB"},
		{17_179_869_184, "16.0 GiB"},
	}
	for _, tt := range tests {
		if got := Bytes(tt.in); got != tt.want {
			t.Errorf("Bytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBytesShort(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{0, "0B"},
		{999, "999B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{10 * 1024, "10K"},
		{289 * 1024 * 1024, "289M"},
		{8 * 1024 * 1024 * 1024, "8.0G"},
	}
	for _, tt := range tests {
		if got := BytesShort(tt.in); got != tt.want {
			t.Errorf("BytesShort(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRate(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0 B/s"},
		{-10, "0 B/s"},
		{2048, "2.0 KiB/s"},
		{200 * 1024, "200 KiB/s"},
	}
	for _, tt := range tests {
		if got := Rate(tt.in); got != tt.want {
			t.Errorf("Rate(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPower(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0.0W"},
		{5.34, "5.3W"},
		{9.96, "10.0W"},
		{45.2, "45W"},
		{150, "150W"},
		{-1, "0.0W"},
	}
	for _, tt := range tests {
		if got := Power(tt.in); got != tt.want {
			t.Errorf("Power(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDurationShort(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{8 * time.Second, "8s"},
		{90 * time.Second, "1m 30s"},
		{12 * time.Minute, "12m"},
		{2*time.Hour + 5*time.Minute, "2h 5m"},
		{26 * time.Hour, "1d 2h"},
		{48 * time.Hour, "2d"},
	}
	for _, tt := range tests {
		if got := DurationShort(tt.in); got != tt.want {
			t.Errorf("DurationShort(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := Duration(3660); got != "1h 1m" {
		t.Errorf("Duration = %q", got)
	}
}

func TestTemperature(t *testing.T) {
	if got := Temperature(60, "c"); got != "60°C" {
		t.Errorf("Temperature c = %q", got)
	}
	if got := Temperature(60, "F"); got != "140°F" {
		t.Errorf("Temperature f = %q", got)
	}
	if got := Temperature(0, "c"); got != "0°C" {
		t.Errorf("Temperature zero = %q", got)
	}
	if got := Temperature(-5, "c"); got != "-5°C" {
		t.Errorf("Temperature negative = %q", got)
	}
	if got := Temperature(200, "c"); got != "–" {
		t.Errorf("Temperature out of range = %q", got)
	}
}

func TestWrap(t *testing.T) {
	got := Wrap("one two three four", 9)
	want := []string{"one two", "three", "four"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Wrap = %#v, want %#v", got, want)
	}

	got = Wrap("abcdefghij", 4)
	want = []string{"abcd", "efgh", "ij"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Wrap long word = %#v, want %#v", got, want)
	}

	// Wide runes occupy two cells each.
	got = Wrap("你好世界", 4)
	want = []string{"你好", "世界"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Wrap wide runes = %#v, want %#v", got, want)
	}
}
