// Package theme contains the built-in colour palettes and helpers for
// blending them in gauges and graphs.
package theme

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a small set of semantic colours. Text is intentionally not part of
// the palette: body text uses the terminal's own foreground colour so that
// tachtop stays readable on both dark and light terminal backgrounds.
type Theme struct {
	Name        string
	Description string

	Accent    lipgloss.Color
	Secondary lipgloss.Color
	Good      lipgloss.Color
	Warn      lipgloss.Color
	Bad       lipgloss.Color
	Dim       lipgloss.Color
	Border    lipgloss.Color

	// Ramp is used for graph gradients, from low (bottom) to high (top).
	Ramp []lipgloss.Color
}

// Severity returns a colour appropriate for a 0-100 percentage.
func (t Theme) Severity(p float64) lipgloss.Color {
	switch {
	case p >= 85:
		return t.Bad
	case p >= 60:
		return t.Warn
	default:
		return t.Good
	}
}

var themes = []Theme{
	{
		Name:        "tachtop",
		Description: "teal and violet",
		Accent:      "#2CB1CF",
		Secondary:   "#A78BFA",
		Good:        "#4ADE80",
		Warn:        "#FBBF24",
		Bad:         "#F87171",
		Dim:         "#8B949E",
		Border:      "#48627A",
		Ramp:        []lipgloss.Color{"#4ADE80", "#FBBF24", "#F87171"},
	},
	{
		Name:        "dracula",
		Description: "the classic dark palette",
		Accent:      "#8BE9FD",
		Secondary:   "#BD93F9",
		Good:        "#50FA7B",
		Warn:        "#F1FA8C",
		Bad:         "#FF5555",
		Dim:         "#6272A4",
		Border:      "#44475A",
		Ramp:        []lipgloss.Color{"#50FA7B", "#F1FA8C", "#FF5555"},
	},
	{
		Name:        "gruvbox",
		Description: "warm, earthy colours",
		Accent:      "#83A598",
		Secondary:   "#D3869B",
		Good:        "#B8BB26",
		Warn:        "#FABD2F",
		Bad:         "#FB4934",
		Dim:         "#928374",
		Border:      "#665C54",
		Ramp:        []lipgloss.Color{"#B8BB26", "#FABD2F", "#FB4934"},
	},
	{
		Name:        "nord",
		Description: "cool arctic blues",
		Accent:      "#88C0D0",
		Secondary:   "#B48EAD",
		Good:        "#A3BE8C",
		Warn:        "#EBCB8B",
		Bad:         "#BF616A",
		Dim:         "#7B88A1",
		Border:      "#4C566A",
		Ramp:        []lipgloss.Color{"#A3BE8C", "#EBCB8B", "#BF616A"},
	},
	{
		Name:        "light",
		Description: "for light terminal backgrounds",
		Accent:      "#0969DA",
		Secondary:   "#8250DF",
		Good:        "#1A7F37",
		Warn:        "#9A6700",
		Bad:         "#CF222E",
		Dim:         "#57606A",
		Border:      "#8C959F",
		Ramp:        []lipgloss.Color{"#1A7F37", "#9A6700", "#CF222E"},
	},
}

// Names returns the built-in theme names in display order.
func Names() []string {
	out := make([]string, len(themes))
	for i, t := range themes {
		out[i] = t.Name
	}
	return out
}

// All returns a copy of the built-in themes.
func All() []Theme {
	out := make([]Theme, len(themes))
	copy(out, themes)
	return out
}

// Get looks up a theme by name (case-insensitive).
func Get(name string) (Theme, bool) {
	for _, t := range themes {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return themes[0], false
}

// RampColor returns the colour at position t (0..1) along a gradient.
func RampColor(ramp []lipgloss.Color, t float64) lipgloss.Color {
	if len(ramp) == 0 {
		return lipgloss.Color("")
	}
	if len(ramp) == 1 {
		return ramp[0]
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	scaled := t * float64(len(ramp)-1)
	idx := int(scaled)
	if idx >= len(ramp)-1 {
		return ramp[len(ramp)-1]
	}
	return lipgloss.Color(Blend(string(ramp[idx]), string(ramp[idx+1]), scaled-float64(idx)))
}

// Blend mixes two #RRGGBB colours. Inputs that cannot be parsed are returned
// unchanged, which keeps custom ANSI colour names working.
func Blend(a, b string, t float64) string {
	ar, ag, ab, okA := parseHex(a)
	br, bg, bb, okB := parseHex(b)
	if !okA || !okB {
		if t < 0.5 {
			return a
		}
		return b
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	r := int(float64(ar) + float64(br-ar)*t + 0.5)
	g := int(float64(ag) + float64(bg-ag)*t + 0.5)
	bl := int(float64(ab) + float64(bb-ab)*t + 0.5)
	return fmt.Sprintf("#%02X%02X%02X", r, g, bl)
}

func parseHex(c string) (int, int, int, bool) {
	c = strings.TrimSpace(c)
	if len(c) != 7 || c[0] != '#' {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(c[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16 & 0xFF), int(v >> 8 & 0xFF), int(v & 0xFF), true
}
