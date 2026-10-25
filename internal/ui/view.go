package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Orizon-Software-Foundation/tachtop/internal/config"
	"github.com/Orizon-Software-Foundation/tachtop/internal/format"
	"github.com/Orizon-Software-Foundation/tachtop/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// View renders the whole screen.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "Starting tachtop…"
	}
	if m.width < 20 || m.height < 6 {
		return truncate("tachtop: terminal too small", m.width)
	}
	if m.layout().tooSmall {
		return m.clampScreen(m.tooSmallView())
	}

	header := m.headerLine()
	footer := m.footerLine()
	var body string
	switch {
	case m.helpOpen:
		body = m.helpPanel()
	case m.detailsOpen:
		body = m.detailsPanel()
	default:
		body = m.bodyView()
	}
	return m.clampScreen(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
}

func (m Model) bodyView() string {
	l := m.layout()
	if l.showDisk {
		if m.width >= 108 {
			row1 := lipgloss.JoinHorizontal(lipgloss.Top,
				m.cpuPanel(l.cpu.w, l.cpu.h),
				m.memPanel(l.mem.w, l.mem.h),
				m.netPanel(l.net.w, l.net.h),
			)
			row2 := lipgloss.JoinHorizontal(lipgloss.Top,
				m.diskPanel(l.disk.w, l.disk.h),
				m.processPanel(l.proc.w, l.proc.h),
			)
			if l.showGPU {
				return lipgloss.JoinVertical(lipgloss.Left, row1, m.gpuPanel(l.gpu.w, l.gpu.h), row2)
			}
			return lipgloss.JoinVertical(lipgloss.Left, row1, row2)
		}

		right := lipgloss.JoinVertical(lipgloss.Left,
			m.memPanel(l.mem.w, l.mem.h),
			m.netPanel(l.net.w, l.net.h),
		)
		row1 := lipgloss.JoinHorizontal(lipgloss.Top,
			m.cpuPanel(l.cpu.w, l.cpu.h),
			right,
		)
		row2 := lipgloss.JoinHorizontal(lipgloss.Top,
			m.diskPanel(l.disk.w, l.disk.h),
			m.processPanel(l.proc.w, l.proc.h),
		)
		if l.showGPU {
			return lipgloss.JoinVertical(lipgloss.Left, row1, m.gpuPanel(l.gpu.w, l.gpu.h), row2)
		}
		return lipgloss.JoinVertical(lipgloss.Left, row1, row2)
	}

	row1 := m.cpuPanel(l.cpu.w, l.cpu.h)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top,
		m.memPanel(l.mem.w, l.mem.h),
		m.netPanel(l.net.w, l.net.h),
	)
	row3 := m.processPanel(l.proc.w, l.proc.h)
	if l.showGPU {
		row3 = lipgloss.JoinVertical(lipgloss.Left, m.gpuPanel(l.gpu.w, l.gpu.h), row3)
	}
	return lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3)
}

func (m Model) headerLine() string {
	host := m.snap.Hostname
	if host == "" {
		host = "starting…"
	}
	left := m.accentBold().Render("▚ tachtop") + "  " + m.boldText(truncate(clean(host), 40))

	var extra []string
	if platform := clean(m.snap.Platform); platform != "" && m.width >= 84 && !strings.EqualFold(platform, host) {
		extra = append(extra, platform)
	}
	if m.width >= 118 && m.snap.Kernel != "" {
		extra = append(extra, clean(m.snap.Kernel))
	}
	if m.width >= 92 && m.snap.Uptime > 0 {
		extra = append(extra, "up "+format.Duration(m.snap.Uptime))
	}
	if m.width >= 106 && m.hasSnap {
		extra = append(extra, fmt.Sprintf("load %.2f %.2f %.2f",
			m.snap.Load[0], m.snap.Load[1], m.snap.Load[2]))
	}
	if len(extra) > 0 {
		left += "  " + m.dimText("· "+strings.Join(extra, " · "))
	}

	clock := time.Now()
	if m.hasSnap && !m.snap.At.IsZero() {
		clock = m.snap.At
	}
	right := m.dimText(clock.Format("Mon 15:04:05"))
	if m.paused {
		right = m.warnText("PAUSED") + "  " + right
	}
	return lineSides(left, right, m.width)
}

func (m Model) footerLine() string {
	var left, right string

	switch {
	case m.kill != nil:
		name := truncate(clean(m.kill.Name), 24)
		left = m.badText(fmt.Sprintf("Terminate %d (%s)?", m.kill.PID, name))
		right = m.dimText("y yes · Y force · esc cancel")

	case m.filtering:
		left = m.accentBold().Render("/ ") + m.filterInput.View()
		right = m.dimText("enter apply · esc cancel")

	case m.helpOpen:
		left = m.dimText("esc or q closes help")
		right = m.dimText("tachtop " + m.version)

	case m.detailsOpen:
		left = m.dimText("esc or q closes details · ↑↓ scroll")
		right = m.dimText(fmt.Sprintf("PID %d", m.detailsPID))

	case m.toast != "":
		prefix := "✓ "
		style := m.goodText
		if m.toastErr {
			prefix = "! "
			style = m.badText
		}
		left = style(prefix + clean(m.toast))
		if m.paused {
			right = m.warnText("PAUSED")
		}

	default:
		left = m.hints()
		if m.paused {
			right = m.warnText("PAUSED")
		}
		if m.snap.Err != "" {
			errText := m.warnText("! " + clean(m.snap.Err))
			if lipgloss.Width(errText) > m.width/2 {
				errText = truncate(errText, maxInt(8, m.width/2))
			}
			right = errText
		}
	}

	return lineSides(left, right, m.width)
}

func (m Model) hints() string {
	pairs := [][2]string{
		{"q", "quit"},
		{"?", "help"},
		{"p", "pause"},
		{"/", "filter"},
		{"s", "sort"},
		{"k", "kill"},
		{"enter", "details"},
	}
	// Narrow terminals get the shorter set; details is still in the help.
	if m.width < 70 {
		pairs = pairs[:len(pairs)-1]
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, m.accentBold().Render(p[0])+" "+m.dimText(p[1]))
	}
	return strings.Join(parts, "  ")
}

// --- CPU panel -------------------------------------------------------------

func (m Model) cpuPanel(w, h int) string {
	if !m.hasSnap {
		return m.panel("CPU", w, h, []string{m.dimText("Collecting live metrics…")})
	}
	return m.panel("CPU", w, h, m.cpuLines(w, h))
}

func (m Model) cpuLines(w, h int) []string {
	innerW, innerH := w-2, h-2
	if innerH <= 0 || innerW <= 0 {
		return nil
	}

	stats := []string{}
	cores := m.snap.CPUCores
	if cores == 0 {
		cores = len(m.snap.CorePercents)
	}
	if cores > 0 {
		stats = append(stats, fmt.Sprintf("%d cores", cores))
	}
	if m.snap.CPUMHz > 0 {
		stats = append(stats, formatFreq(m.snap.CPUMHz))
	}
	if m.snap.CPUTemp > 0 {
		stats = append(stats, format.Temperature(m.snap.CPUTemp, m.cfg.TempUnit))
	}
	left := m.accentBold().Render(strings.Join(stats, " · "))
	lines := []string{fit(left, innerW)}

	// Total CPU is the headline number, so give it its own row below the
	// hardware summary. The model name shares that row when there is room.
	cpuPct := clampPct(m.snap.CPUPercent)
	gaugeW := clampInt(innerW/3, 8, 24)
	totalLine := m.gaugeBracketed(cpuPct, gaugeW) + " " + m.percentText(cpuPct)
	if innerW > 34 && m.snap.CPUModel != "" {
		modelW := innerW - lipgloss.Width(totalLine) - 1
		if modelW >= 8 {
			totalLine = twoSides(totalLine, m.dimText(truncate(clean(m.snap.CPUModel), modelW)), innerW)
		}
	}
	lines = append(lines, totalLine)

	remaining := innerH - 2
	if remaining < 1 {
		return lines
	}

	values := m.snap.CorePercents
	idxW := 1
	if len(values) > 1 {
		idxW = len(strconv.Itoa(len(values) - 1))
	}
	cols := 1
	if innerW >= 30 {
		cols = maxInt(1, innerW/15)
	}
	cols = minInt(cols, maxInt(1, len(values)))
	rowsNeeded := 0
	if len(values) > 0 && cols > 0 {
		rowsNeeded = (len(values) + cols - 1) / cols
	}

	graphH := remaining
	coreRows := 0
	noteRows := 0
	if rowsNeeded > 0 && remaining >= 6 {
		graphH = maxInt(2, remaining/2)
		coreRows = minInt(rowsNeeded, minInt(4, remaining-graphH-1))
		if coreRows < 1 {
			coreRows = 0
			graphH = remaining
		} else {
			if rowsNeeded > coreRows {
				noteRows = 1
			}
			graphH = remaining - coreRows - noteRows
			if graphH < 2 {
				coreRows = maxInt(0, remaining-2)
				noteRows = 0
				graphH = remaining - coreRows
			}
		}
	}
	if graphH > 0 {
		cpuMax := AutoMax(m.histCPU.data(), 10)
		if cpuMax > 100 {
			cpuMax = 100
		}
		lines = append(lines, RenderGraph(m.histCPU.data(), innerW, graphH, cpuMax, m.pal.Ramp)...)
	}

	if coreRows > 0 && cols > 0 {
		cellW := (innerW - (cols-1)*2) / cols
		gaugeW := cellW - idxW - 6
		if gaugeW < 3 {
			gaugeW = 3
		}
		shown := 0
		for r := 0; r < coreRows; r++ {
			cells := make([]string, 0, cols)
			for c := 0; c < cols; c++ {
				idx := r*cols + c
				if idx >= len(values) {
					break
				}
				pct := values[idx]
				cell := m.dimText(padLeft(strconv.Itoa(idx), idxW)) + " " +
					Gauge(pct, gaugeW, m.pal) + " " +
					m.style(m.pal.Severity(pct)).Render(fmt.Sprintf("%3.0f%%", pct))
				cells = append(cells, cell)
				shown++
			}
			if len(cells) > 0 {
				lines = append(lines, fit(strings.Join(cells, "  "), innerW))
			}
		}
		if noteRows > 0 && shown < len(values) {
			lines = append(lines, m.dimText(fmt.Sprintf("… and %d more cores", len(values)-shown)))
		}
	}
	return lines
}

func formatFreq(mhz float64) string {
	if mhz >= 1000 {
		return fmt.Sprintf("%.1f GHz", mhz/1000)
	}
	return fmt.Sprintf("%.0f MHz", mhz)
}

// --- Memory panel ----------------------------------------------------------

func (m Model) memPanel(w, h int) string {
	if !m.hasSnap {
		return m.panel("MEMORY", w, h, []string{m.dimText("Collecting live metrics…")})
	}
	return m.panel("MEMORY", w, h, m.memLines(w, h))
}

func (m Model) memLines(w, h int) []string {
	innerW, innerH := w-2, h-2
	if innerH <= 0 || innerW <= 0 {
		return nil
	}
	total := m.snap.MemTotal
	if total == 0 {
		return []string{m.dimText("Memory information unavailable")}
	}

	pct := float64(m.snap.MemUsed) / float64(total) * 100
	gaugeW := innerW - 9
	if gaugeW < 4 {
		gaugeW = 4
	}
	lines := []string{
		m.gaugeBracketed(clampPct(pct), gaugeW) + " " + m.percentText(pct),
	}

	usedLine := m.dimText("Used ") + format.Bytes(m.snap.MemUsed) + m.dimText(" / ") + format.Bytes(total)
	cacheLine := m.dimText("Cache ") + format.Bytes(m.snap.MemCached)
	if lipgloss.Width(usedLine)+1+lipgloss.Width(cacheLine) <= innerW {
		lines = append(lines, twoSides(usedLine, cacheLine, innerW))
	} else {
		lines = append(lines, usedLine)
	}

	availableLine := m.dimText("Available ") + format.Bytes(m.snap.MemAvailable) +
		m.dimText("  ·  Free ") + format.Bytes(m.snap.MemFree)
	if m.snap.SwapTotal > 0 {
		swapPct := float64(m.snap.SwapUsed) / float64(m.snap.SwapTotal) * 100
		swapW := minInt(10, maxInt(4, innerW/4))
		swapLine := m.dimText("Swap ") + m.gaugeBracketed(clampPct(swapPct), swapW) +
			" " + format.Bytes(m.snap.SwapUsed) + m.dimText(" / ") + format.Bytes(m.snap.SwapTotal)
		lines = append(lines, swapLine)
	}
	lines = append(lines, availableLine)

	// Keep at least two rows for the history graph when there is room.
	if innerH >= 5 {
		maxText := innerH - 2
		if len(lines) > maxText {
			lines = lines[:maxText]
		}
	}
	graphH := innerH - len(lines)
	if graphH > 0 {
		lines = append(lines, RenderGraph(m.histMem.data(), innerW, graphH, 100, m.pal.Ramp)...)
	}
	return lines
}

func clampPct(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// --- Network panel ---------------------------------------------------------

func (m Model) netPanel(w, h int) string {
	if !m.hasSnap {
		return m.panel("NETWORK", w, h, []string{m.dimText("Collecting live metrics…")})
	}
	return m.panel("NETWORK", w, h, m.netLines(w, h))
}

func (m Model) netLines(w, h int) []string {
	innerW, innerH := w-2, h-2
	if innerH <= 0 || innerW <= 0 {
		return nil
	}

	lines := []string{
		m.goodText("↓ "+format.Rate(m.snap.NetRx)) + "   " + m.accentText("↑ "+format.Rate(m.snap.NetTx)),
	}

	ifaceBudget := 0
	if innerH >= 6 {
		ifaceBudget = 2
	}
	shown := 0
	for _, n := range m.snap.Networks {
		if shown >= ifaceBudget {
			break
		}
		if n.Rx+n.Tx <= 0 {
			continue
		}
		name := m.dimText(padRight(truncate(clean(n.Name), 10), 10))
		line := name + " " + m.goodText("↓ "+format.Rate(n.Rx)) + "  " + m.accentText("↑ "+format.Rate(n.Tx))
		lines = append(lines, line)
		shown++
	}

	graphH := innerH - len(lines)
	if graphH > 0 {
		max := AutoMax(m.histNet.data(), 1024)
		lines = append(lines, RenderGraph(m.histNet.data(), innerW, graphH, max, m.pal.Ramp)...)
	}
	return lines
}

// --- Disk panel ------------------------------------------------------------

func (m Model) diskPanel(w, h int) string {
	if !m.hasSnap {
		return m.panel("DISK", w, h, []string{m.dimText("Collecting live metrics…")})
	}
	return m.panel("DISK", w, h, m.diskLines(w, h))
}

func (m Model) diskLines(w, h int) []string {
	innerW, innerH := w-2, h-2
	if innerH <= 0 || innerW <= 0 {
		return nil
	}

	lines := []string{
		m.goodText("R "+format.Rate(m.snap.DiskRead)) + "   " + m.accentText("W "+format.Rate(m.snap.DiskWrite)),
	}
	if len(m.snap.Disks) == 0 {
		lines = append(lines, m.dimText("No mounted filesystems"))
		return lines
	}

	mountRows := minInt(len(m.snap.Disks), minInt(6, innerH-1))
	graphH := 0
	if spare := innerH - 1 - mountRows; spare >= 3 {
		graphH = spare
	}

	gaugeW := 6
	if innerW >= 46 {
		gaugeW = 10
	}
	for i := 0; i < mountRows; i++ {
		d := m.snap.Disks[i]
		left := m.gaugeBracketed(clampPct(d.UsedPercent), gaugeW) + " " +
			m.style(m.pal.Severity(d.UsedPercent)).Render(fmt.Sprintf("%3.0f%%", d.UsedPercent)) + " " +
			clean(d.Mount)
		line := left
		if innerW >= 46 {
			right := m.dimText(format.Bytes(d.Used) + " / " + format.Bytes(d.Total))
			line = twoSides(left, right, innerW)
		}
		lines = append(lines, line)
	}
	if graphH > 0 {
		max := AutoMax(m.histDisk.data(), 1024)
		lines = append(lines, RenderGraph(m.histDisk.data(), innerW, graphH, max, m.pal.Ramp)...)
	}
	return lines
}

// --- Help and details ------------------------------------------------------

func (m Model) helpPanel() string {
	bodyH := m.height - 2
	version := m.version
	if version == "" {
		version = "dev"
	}
	cfgPath := m.cfgPath
	if cfgPath == "" {
		cfgPath = config.Path()
	}
	if cfgPath == "" {
		cfgPath = "(not found)"
	}
	lines := []string{
		m.accentBold().Render("tachtop " + version + " — live system monitor"),
		m.dimText("KEYS"),
		m.helpLine("q / ctrl+c", "quit tachtop"),
		m.helpLine("↑ ↓ / wheel", "select a process (or click)"),
		m.helpLine("PgUp / PgDn", "move one page"),
		m.helpLine("Home / End", "jump to first / last"),
		m.helpLine("/", "filter by name, user or PID"),
		m.helpLine("c / esc", "clear the filter"),
		m.helpLine("s / r", "cycle sort / reverse order"),
		m.helpLine("enter", "show process details"),
		m.helpLine("k", "terminate process (y, Y = force)"),
		m.helpLine("p", "pause / resume updates"),
		m.helpLine("?", "open / close this help"),
		"",
		m.dimText("ABOUT"),
		"  Data: /proc, /sys and vendor tools. No daemon or root.",
		"  Config (optional): " + m.dimText(cfgPath),
		"  Themes: " + strings.Join(theme.Names(), ", "),
	}
	return m.panel("HELP", m.width, bodyH, lines)
}

func (m Model) helpLine(keys, desc string) string {
	return "  " + m.accentBold().Render(padRight(keys, 13)) + m.dimText(desc)
}

func (m Model) detailsPanel() string {
	bodyH := m.height - 2
	title := fmt.Sprintf("PROCESS %d", m.detailsPID)
	if m.details != nil && m.details.Name != "" {
		title = fmt.Sprintf("PROCESS %d · %s", m.detailsPID, truncate(clean(m.details.Name), 40))
	}
	lines := m.detailsLines(m.width, bodyH)
	innerH := bodyH - 2
	if innerH < 1 {
		innerH = 1
	}
	maxOffset := len(lines) - innerH
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := clampInt(m.detailsOffset, 0, maxOffset)
	if offset > 0 {
		lines = lines[offset:]
	}
	return m.panel(title, m.width, bodyH, lines)
}

// detailsPage is the number of content rows visible in the details panel.
func (m Model) detailsPage() int {
	rows := m.height - 4 // header, footer and panel borders
	if rows < 1 {
		return 1
	}
	return rows
}

// scrollDetails moves the details viewport, clamping it to the content.
func (m *Model) scrollDetails(delta int) {
	lines := m.detailsLines(m.width, m.height-2)
	maxOffset := len(lines) - m.detailsPage()
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.detailsOffset = clampInt(m.detailsOffset+delta, 0, maxOffset)
}

func (m Model) detailsLines(w, h int) []string {
	innerW, innerH := w-2, h-2
	if innerH <= 0 || innerW <= 0 {
		return nil
	}
	if m.details == nil {
		return []string{m.dimText("Collecting process details…")}
	}
	d := m.details

	var liveCPU, liveMem, liveRSS float64
	liveFound := false
	for _, p := range m.snap.Procs {
		if p.PID == d.PID {
			liveCPU, liveMem = p.CPU, p.MemPercent
			liveRSS = float64(p.RSS)
			liveFound = true
			break
		}
	}

	rss := d.RSS
	if rss == 0 && liveFound {
		rss = uint64(liveRSS)
	}
	cpuText := "–"
	if liveFound {
		cpuText = fmt.Sprintf("%.1f%%", liveCPU)
	}
	memText := format.Bytes(rss)
	if liveFound {
		memText += fmt.Sprintf(" (%.1f%%)", liveMem)
	}
	started := "–"
	if d.Created > 0 {
		started = time.Unix(d.Created, 0).Format("2006-01-02 15:04")
	}

	lines := []string{
		m.detailPair("PID", strconv.Itoa(int(d.PID)), "User", d.User, innerW),
		m.detailPair("PPID", strconv.Itoa(int(d.PPID)), "Threads", strconv.Itoa(int(d.Threads)), innerW),
		m.detailPair("State", d.Status, "Started", started, innerW),
		m.detailPair("CPU", cpuText, "Memory", memText, innerW),
		"",
		m.detailField("Name", d.Name, innerW),
		m.detailField("Executable", d.Exe, innerW),
		m.detailField("Working dir", d.Cwd, innerW),
		m.detailField("Memory", format.Bytes(rss)+" RSS · "+format.Bytes(d.VMS)+" virtual · "+format.Bytes(d.Swap)+" swap", innerW),
	}

	if d.Cmdline != "" {
		lines = append(lines, "", m.dimText("Command line"))
		for _, line := range format.Wrap(clean(d.Cmdline), innerW-2) {
			lines = append(lines, "  "+line)
		}
	}
	for _, warning := range d.Warnings {
		lines = append(lines, m.badText("! "+clean(warning)))
	}
	return lines
}

func (m Model) detailPair(label1, value1, label2, value2 string, width int) string {
	colW := (width - 2) / 2
	if colW < 12 {
		return m.detailField(label1, value1, width)
	}
	left := fit(m.detailField(label1, value1, colW), colW)
	rightW := width - colW - 2
	right := fit(m.detailField(label2, value2, rightW), rightW)
	return left + "  " + right
}

func (m Model) detailField(label, value string, width int) string {
	if width <= 0 {
		return ""
	}
	labelW := 11
	value = clean(value)
	if value == "" {
		value = "–"
	}
	labelText := m.dimText(padRight(label, labelW))
	if width <= labelW+1 {
		return labelText
	}
	return labelText + " " + truncate(value, width-labelW-1)
}

// --- Small terminals and screen fitting ------------------------------------

func (m Model) tooSmallView() string {
	msg := m.accentBold().Render("tachtop") + "\n\n" +
		fmt.Sprintf("This terminal is %d×%d.\n", m.width, m.height) +
		"tachtop needs at least 64×22.\n\n" +
		m.dimText("Resize the window, or press q to quit.")
	if m.width < 20 || m.height < 6 {
		return "tachtop: terminal too small"
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg)
}

func (m Model) clampScreen(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i, line := range lines {
		lines[i] = truncate(line, m.width)
	}
	return strings.Join(lines, "\n")
}
