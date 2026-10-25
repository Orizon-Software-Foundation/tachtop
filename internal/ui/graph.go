package ui

import (
	"math"
	"strings"

	"github.com/Orizon-Software-Foundation/tachtop/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// Braille cells are 2 columns by 4 rows. This lookup converts a position
// inside a cell into the corresponding Unicode braille dot bit.
var dotBits = [2][4]byte{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

var partialBlocks = []rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉'}

// RenderGraph draws a filled area graph using braille characters, one
// coloured string per terminal row. Height and width are measured in cells.
func RenderGraph(values []float64, width, height int, max float64, ramp []lipgloss.Color) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	if max <= 0 || math.IsNaN(max) || math.IsInf(max, 0) {
		max = 1
	}

	pixelWidth := width * 2
	pixelHeight := height * 4

	var points []float64
	switch len(values) {
	case 0:
		// No history yet: draw an empty grid rather than a fake baseline.
		points = make([]float64, pixelWidth)
		for i := range points {
			points[i] = math.NaN()
		}
	case 1:
		// A single sample is one column at the right edge, not a flat line
		// across the whole graph.
		points = make([]float64, pixelWidth)
		for i := range points {
			points[i] = math.NaN()
		}
		points[pixelWidth-1] = values[0]
	default:
		points = resample(values, pixelWidth)
	}

	grid := make([][]byte, height)
	for i := range grid {
		grid[i] = make([]byte, width)
	}

	for x := 0; x < pixelWidth; x++ {
		v := points[x]
		if math.IsNaN(v) {
			continue
		}
		t := v / max
		if t < 0 || math.IsNaN(t) {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		level := int(math.Round(t * float64(pixelHeight-1)))
		for y := 0; y <= level; y++ {
			cellY := (pixelHeight - 1 - y) / 4
			dotY := (pixelHeight - 1 - y) % 4
			cellX := x / 2
			dotX := x % 2
			grid[cellY][cellX] |= dotBits[dotX][dotY]
		}
	}

	lines := make([]string, 0, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		b.Grow(width)
		for _, bits := range grid[row] {
			if bits == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(rune(0x2800 + int(bits)))
			}
		}
		t := 0.0
		if height > 1 {
			t = 1 - float64(row)/float64(height-1)
		}
		style := lipgloss.NewStyle().Foreground(theme.RampColor(ramp, t))
		lines = append(lines, style.Render(b.String()))
	}
	return lines
}

// resample expands or contracts values to exactly n points using linear
// interpolation, keeping the newest values at the right edge.
func resample(values []float64, n int) []float64 {
	out := make([]float64, n)
	if n <= 0 || len(values) == 0 {
		return out
	}
	if n == 1 {
		out[0] = values[len(values)-1]
		return out
	}
	if len(values) == 1 {
		for i := range out {
			out[i] = values[0]
		}
		return out
	}
	last := len(values) - 1
	for i := 0; i < n; i++ {
		pos := float64(i) * float64(last) / float64(n-1)
		lo := int(math.Floor(pos))
		hi := lo + 1
		if hi > last {
			hi = last
		}
		frac := pos - float64(lo)
		out[i] = values[lo]*(1-frac) + values[hi]*frac
	}
	return out
}

// AutoMax returns a graph ceiling with a little headroom above the largest
// value, never below floor.
func AutoMax(values []float64, floor float64) float64 {
	max := floor
	for _, v := range values {
		if v > max && !math.IsNaN(v) && !math.IsInf(v, 0) {
			max = v
		}
	}
	if max <= 0 {
		max = 1
	}
	return max * 1.15
}

// Gauge renders a smooth horizontal bar. The percentage determines both the
// fill and its colour (green, amber, red).
func Gauge(pct float64, width int, pal theme.Theme) string {
	if width <= 0 {
		return ""
	}
	if pct < 0 || math.IsNaN(pct) {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	eighths := int(math.Round(pct / 100 * float64(width) * 8))
	full := eighths / 8
	rem := eighths % 8
	if full > width {
		full = width
		rem = 0
	}
	if full == width {
		rem = 0
	}

	filled := strings.Repeat("█", full)
	if rem > 0 {
		filled += string(partialBlocks[rem-1])
	}
	empty := width - full
	if rem > 0 {
		empty--
	}
	if empty < 0 {
		empty = 0
	}

	fillStyle := lipgloss.NewStyle().Foreground(pal.Severity(pct))
	trackStyle := lipgloss.NewStyle().Foreground(pal.Border)
	return fillStyle.Render(filled) + trackStyle.Render(strings.Repeat("░", empty))
}
