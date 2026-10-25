package ui

import (
	"fmt"
	"strings"

	"github.com/Orizon-Software-Foundation/tachtop/internal/format"
	"github.com/Orizon-Software-Foundation/tachtop/internal/metrics"
	"github.com/charmbracelet/lipgloss"
)

// --- GPU panel -------------------------------------------------------------

func (m Model) gpuPanel(w, h int) string {
	title := "GPU"
	if len(m.snap.GPUs) > 1 {
		title = fmt.Sprintf("GPUs %d", len(m.snap.GPUs))
	}
	return m.panel(title, w, h, m.gpuLines(w, h))
}

func (m Model) gpuLines(w, h int) []string {
	innerW, innerH := w-2, h-2
	if innerH <= 0 || innerW <= 0 {
		return nil
	}

	gpus := m.snap.GPUs
	more := 0
	if len(gpus) > innerH {
		if innerH > 1 {
			gpus = gpus[:innerH-1]
			more = len(m.snap.GPUs) - len(gpus)
		} else {
			gpus = gpus[:1]
			more = len(m.snap.GPUs) - 1
		}
	}

	lines := make([]string, 0, innerH)
	for _, gpu := range gpus {
		lines = append(lines, m.gpuLine(gpu, innerW))
	}
	if more > 0 && len(lines) < innerH {
		lines = append(lines, m.dimText(fmt.Sprintf("… and %d more", more)))
	}
	return lines
}

// gpuLine keeps the name in a flexible column and drops the least important
// statistics when the terminal is narrow. One line per GPU keeps the panel
// compact even on hybrid laptops with two or more adapters.
func (m Model) gpuLine(gpu metrics.GPU, width int) string {
	prefix := m.dimText(fmt.Sprintf("#%d", gpu.Index))

	type piece struct {
		text string
		w    int
	}
	var stats []piece
	add := func(text string) {
		if text != "" {
			stats = append(stats, piece{text: text, w: lipgloss.Width(text)})
		}
	}

	if gpu.HasUtil {
		pct := clampPct(gpu.UtilPercent)
		add(m.style(m.pal.Severity(pct)).Render(fmt.Sprintf("%.0f%%", pct)))
	}
	if gpu.HasTemp {
		add(m.gpuTempText(gpu.Temp))
	}
	if clock := gpuClockText(gpu); clock != "" {
		add(m.accentText(clock))
	}
	if clock := gpuMemClockText(gpu); clock != "" {
		add(m.dimText(clock))
	}
	if gpu.MemTotal > 0 {
		add(m.dimText("VRAM " + format.BytesShort(gpu.MemUsed) + "/" + format.BytesShort(gpu.MemTotal)))
	}
	if gpu.Power > 0 {
		power := formatGPUPower(gpu.Power)
		if gpu.PowerLimit > 0 {
			power += "/" + formatGPUPower(gpu.PowerLimit)
		}
		add(m.dimText(power))
	}

	statsWidth := 0
	for _, p := range stats {
		statsWidth += p.w
	}
	if len(stats) > 1 {
		statsWidth += (len(stats) - 1) * 2
	}

	// The prefix is "#N", followed by a space and then the name column.
	// Keep enough of the name to identify the adapter before optional
	// statistics such as VRAM or power are sacrificed.
	const minNameBudget = 14
	nameBudget := width - 3 - 2 - statsWidth
	for nameBudget < minNameBudget && len(stats) > 0 {
		stats = stats[:len(stats)-1]
		statsWidth = 0
		for _, p := range stats {
			statsWidth += p.w
		}
		if len(stats) > 1 {
			statsWidth += (len(stats) - 1) * 2
		}
		nameBudget = width - 3 - 2 - statsWidth
	}
	if nameBudget < 1 {
		nameBudget = 1
	}
	nameBudget = minInt(nameBudget, 48)

	name := fit(clean(gpu.Name), nameBudget)
	line := prefix + " " + m.boldText(name)
	if len(stats) > 0 {
		texts := make([]string, len(stats))
		for i, p := range stats {
			texts[i] = p.text
		}
		line += "  " + strings.Join(texts, "  ")
	}
	return fit(line, width)
}

func (m Model) gpuTempText(temp float64) string {
	color := m.pal.Good
	switch {
	case temp >= 85:
		color = m.pal.Bad
	case temp >= 70:
		color = m.pal.Warn
	}
	return m.style(color).Render(format.Temperature(temp, m.cfg.TempUnit))
}

func gpuClockText(gpu metrics.GPU) string {
	if gpu.CoreMHz <= 0 {
		return ""
	}
	if gpu.MaxCoreMHz > gpu.CoreMHz {
		return fmt.Sprintf("%.0f/%.0fMHz", gpu.CoreMHz, gpu.MaxCoreMHz)
	}
	return fmt.Sprintf("%.0fMHz", gpu.CoreMHz)
}

func gpuMemClockText(gpu metrics.GPU) string {
	if gpu.MemMHz <= 0 {
		return ""
	}
	if gpu.MaxMemMHz > gpu.MemMHz {
		return fmt.Sprintf("MEM %.0f/%.0fMHz", gpu.MemMHz, gpu.MaxMemMHz)
	}
	return fmt.Sprintf("MEM %.0fMHz", gpu.MemMHz)
}

func formatGPUPower(watts float64) string {
	if watts >= 10 {
		return fmt.Sprintf("%.0fW", watts)
	}
	return fmt.Sprintf("%.1fW", watts)
}
