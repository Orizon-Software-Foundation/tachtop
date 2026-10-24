package theme

import "testing"

func TestGet(t *testing.T) {
	if _, ok := Get("tachtop"); !ok {
		t.Fatal("tachtop theme should exist")
	}
	if th, ok := Get("DRACULA"); !ok || th.Name != "dracula" {
		t.Fatalf("Get is not case-insensitive: %q %v", th.Name, ok)
	}
	if th, ok := Get("nope"); ok || th.Name != "tachtop" {
		t.Fatalf("unknown theme should fall back to default")
	}
}

func TestNamesAndAll(t *testing.T) {
	names := Names()
	if len(names) != 5 {
		t.Fatalf("Names() = %v", names)
	}
	want := []string{"tachtop", "dracula", "gruvbox", "nord", "light"}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("Names()[%d] = %q, want %q", i, names[i], n)
		}
	}

	all := All()
	all[0].Name = "changed"
	if Names()[0] != "tachtop" {
		t.Error("All() should return a copy")
	}
}

func TestSeverity(t *testing.T) {
	th, _ := Get("tachtop")
	if got := th.Severity(10); got != th.Good {
		t.Errorf("Severity(10) = %v, want Good", got)
	}
	if got := th.Severity(60); got != th.Warn {
		t.Errorf("Severity(60) = %v, want Warn", got)
	}
	if got := th.Severity(85); got != th.Bad {
		t.Errorf("Severity(85) = %v, want Bad", got)
	}
}

func TestBlend(t *testing.T) {
	tests := []struct {
		a, b string
		f    float64
		want string
	}{
		{"#000000", "#FFFFFF", 0, "#000000"},
		{"#000000", "#FFFFFF", 1, "#FFFFFF"},
		{"#000000", "#FFFFFF", 0.5, "#808080"},
		{"#FF0000", "#0000FF", 0.5, "#800080"},
	}
	for _, tt := range tests {
		if got := Blend(tt.a, tt.b, tt.f); got != tt.want {
			t.Errorf("Blend(%q, %q, %v) = %q, want %q", tt.a, tt.b, tt.f, got, tt.want)
		}
	}
	if got := Blend("red", "blue", 0.2); got != "red" {
		t.Errorf("Blend with names = %q, want fallback", got)
	}
	if got := Blend("red", "blue", 0.8); got != "blue" {
		t.Errorf("Blend with names = %q, want fallback", got)
	}
}

func TestRampColor(t *testing.T) {
	th, _ := Get("tachtop")
	if got := RampColor(nil, 0.5); got != "" {
		t.Errorf("RampColor(nil) = %q", got)
	}
	if got := RampColor(th.Ramp, -1); got != th.Ramp[0] {
		t.Errorf("RampColor low = %v", got)
	}
	if got := RampColor(th.Ramp, 2); got != th.Ramp[len(th.Ramp)-1] {
		t.Errorf("RampColor high = %v", got)
	}
}
