package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// panel draws a bordered box with its title embedded in the top border.
// content is padded or clipped to the exact inner size.
func (m Model) panel(title string, w, h int, content []string) string {
	if w < 8 || h < 3 {
		return ""
	}
	borderStyle := m.borderStyle()
	titleStyle := m.accentBold()
	title = clean(title)
	innerW := w - 2
	innerH := h - 2

	title = truncate(title, w-6)
	fill := w - 5 - lipgloss.Width(title)
	if fill < 0 {
		fill = 0
	}
	top := borderStyle.Render("╭─ ") + titleStyle.Render(title+" ") +
		borderStyle.Render(strings.Repeat("─", fill)+"╮")
	bottom := borderStyle.Render("╰" + strings.Repeat("─", w-2) + "╯")

	// Flatten any embedded newlines so a panel can never break the layout.
	lines := make([]string, 0, len(content))
	for _, line := range content {
		parts := strings.Split(line, "\n")
		lines = append(lines, parts...)
	}

	rows := make([]string, 0, innerH)
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		line = strings.ReplaceAll(line, "\t", "  ")
		line = fit(line, innerW)
		rows = append(rows, borderStyle.Render("│")+line+borderStyle.Render("│"))
	}

	return strings.Join(append([]string{top}, append(rows, bottom)...), "\n")
}

// fit pads or ANSI-aware truncates s to exactly width terminal cells.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) > width {
		if width > 1 {
			s = ansi.Truncate(s, width, "…")
		} else {
			s = ansi.Truncate(s, width, "")
		}
	}
	if pad := width - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(s, width, "…")
}

func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return truncate(s, width)
}

func padLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if pad := width - lipgloss.Width(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return truncate(s, width)
}

// twoSides puts left and right on one line, padding the gap. If space runs
// out, the right side is shortened first and then the left side is clipped.
func twoSides(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if pad := width - lw - rw; pad >= 1 {
		return left + strings.Repeat(" ", pad) + right
	}
	if lw >= width {
		return fit(left, width)
	}
	avail := width - lw - 1
	if avail >= 2 {
		return left + " " + fit(right, avail)
	}
	return fit(left, width)
}

// clean removes control characters from kernel-provided strings. Without
// this, a process name containing an escape sequence could alter the
// terminal state.
func clean(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
			// drop newlines, carriage returns, escapes and friends
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m Model) style(color lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(color)
}

func (m Model) borderStyle() lipgloss.Style {
	return m.style(m.pal.Border)
}

func (m Model) accentText(s string) string {
	return m.style(m.pal.Accent).Render(s)
}

func (m Model) boldText(s string) string {
	return lipgloss.NewStyle().Bold(true).Render(s)
}

func (m Model) accentBold() lipgloss.Style {
	return m.style(m.pal.Accent).Bold(true)
}

func (m Model) dimText(s string) string {
	return m.style(m.pal.Dim).Render(s)
}

func (m Model) warnText(s string) string {
	return m.style(m.pal.Warn).Render(s)
}

func (m Model) badText(s string) string {
	return m.style(m.pal.Bad).Render(s)
}

func (m Model) goodText(s string) string {
	return m.style(m.pal.Good).Render(s)
}

func (m Model) borderText(s string) string {
	return m.borderStyle().Render(s)
}

func (m Model) gaugeBracketed(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	return m.borderText("[") + Gauge(pct, width, m.pal) + m.borderText("]")
}

func (m Model) percentText(pct float64) string {
	return m.style(m.pal.Severity(pct)).Render(fmt.Sprintf("%.1f%%", pct))
}

// lineSides places left and right on one line. The right side is kept intact
// where possible (clock, status) and the left side is shortened instead.
func lineSides(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	rw := lipgloss.Width(right)
	avail := width - rw - 1
	if avail < 0 {
		return fit(left, width)
	}
	if lipgloss.Width(left) > avail {
		left = truncate(left, avail)
	}
	gap := width - lipgloss.Width(left) - rw
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}
