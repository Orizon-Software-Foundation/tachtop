package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestClean(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"a\tb", "a b"},
		{"a\nb", "ab"},
		{"\x1b[31mred\x1b[0m", "[31mred[0m"},
		{"ok\x00", "ok"},
		{"evil\u202Etext", "eviltext"},
	}
	for _, tt := range tests {
		if got := clean(tt.in); got != tt.want {
			t.Errorf("clean(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFitAndTruncate(t *testing.T) {
	if got := fit("ab", 4); got != "ab  " {
		t.Errorf("fit pad = %q", got)
	}
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("abc", 4); got != "abc" {
		t.Errorf("truncate no-op = %q", got)
	}
	if got := fit("abcdef", 4); got != "abc…" {
		t.Errorf("fit truncate = %q", got)
	}
	if got := fit("hello", 0); got != "" {
		t.Errorf("fit zero = %q", got)
	}

	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("abcdef")
	if got := lipgloss.Width(truncate(styled, 4)); got != 4 {
		t.Errorf("ANSI truncate width = %d", got)
	}
}

func TestTwoSides(t *testing.T) {
	if got := twoSides("left", "right", 20); got != "left"+"           "+"right" {
		t.Errorf("twoSides = %q", got)
	}
	if got := twoSides("long-left", "right", 12); lipgloss.Width(got) != 12 {
		t.Errorf("twoSides small width = %q (%d)", got, lipgloss.Width(got))
	}
	if got := twoSides("left", "right", 4); got != "left" {
		t.Errorf("twoSides no room = %q", got)
	}
}

func TestLineSides(t *testing.T) {
	if got := lineSides("left", "right", 15); got != "left"+"      "+"right" {
		t.Errorf("lineSides = %q", got)
	}
	if got := lineSides(strings.Repeat("x", 20), "right", 15); lipgloss.Width(got) != 15 {
		t.Errorf("lineSides clipping = %q", got)
	}
}
