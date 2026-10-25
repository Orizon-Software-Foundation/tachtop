package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Orizon-Software-Foundation/tachtop/internal/format"
	"github.com/Orizon-Software-Foundation/tachtop/internal/metrics"
	"github.com/charmbracelet/lipgloss"
)

const (
	pidWidth   = 7
	userWidth  = 9
	cpuWidth   = 6
	memWidth   = 6
	memBWWidth = 8
	stateWidth = 5
)

func (m Model) currentFilter() string {
	if m.filtering {
		return strings.TrimSpace(m.filterInput.Value())
	}
	return m.filter
}

// visibleProcs returns the process list after filtering and sorting.
func (m Model) visibleProcs() []metrics.Proc {
	query := strings.ToLower(m.currentFilter())
	out := make([]metrics.Proc, 0, len(m.snap.Procs))
	for _, p := range m.snap.Procs {
		if query != "" &&
			!strings.Contains(strings.ToLower(p.Name), query) &&
			!strings.Contains(strings.ToLower(p.User), query) &&
			!strings.Contains(strconv.Itoa(int(p.PID)), query) {
			continue
		}
		out = append(out, p)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		var cmp int
		switch m.sortMode {
		case sortMem:
			cmp = compareFloat(a.MemPercent, b.MemPercent)
		case sortPID:
			cmp = int(a.PID) - int(b.PID)
		case sortName:
			cmp = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		default:
			cmp = compareFloat(a.CPU, b.CPU)
		}
		if cmp == 0 {
			return a.PID < b.PID
		}
		if m.sortDesc {
			return cmp > 0
		}
		return cmp < 0
	})
	return out
}

func compareFloat(a, b float64) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}

func (m Model) selectedIndex(list []metrics.Proc) int {
	if len(list) == 0 {
		return 0
	}
	if m.selectedPID != 0 {
		for i, p := range list {
			if p.PID == m.selectedPID {
				return i
			}
		}
	}
	return clampInt(m.procOffset, 0, len(list)-1)
}

func (m *Model) selectedProc() *metrics.Proc {
	list := m.visibleProcs()
	if len(list) == 0 {
		return nil
	}
	idx := clampInt(m.selectedIndex(list), 0, len(list)-1)
	p := list[idx]
	return &p
}

func (m *Model) reconcileSelection() {
	list := m.visibleProcs()
	if len(list) == 0 {
		m.selectedPID = 0
		m.procOffset = 0
		return
	}
	if m.selectedPID == 0 {
		// Nothing has been picked yet: keep following the top of the list
		// instead of pinning whichever process happened to be first.
		return
	}
	for _, p := range list {
		if p.PID == m.selectedPID {
			return
		}
	}
	// The selected process disappeared (or was filtered out).
	m.selectedPID = 0
	m.procOffset = 0
}

func (m *Model) ensureVisible() {
	list := m.visibleProcs()
	if len(list) == 0 {
		m.procOffset = 0
		return
	}
	offset, _ := m.procViewport(list, m.procRows())
	m.procOffset = offset
}

// procViewport returns the first visible row and the selected row for list,
// honouring the current scroll offset. rows is the number of process rows
// that fit in the table.
func (m Model) procViewport(list []metrics.Proc, rows int) (offset, selected int) {
	if rows < 1 {
		rows = 1
	}
	if len(list) == 0 {
		return 0, 0
	}
	maxOffset := len(list) - rows
	if maxOffset < 0 {
		maxOffset = 0
	}
	selected = m.selectedIndex(list)
	offset = clampInt(m.procOffset, 0, maxOffset)
	if selected < offset {
		offset = selected
	}
	if selected >= offset+rows {
		offset = selected - rows + 1
	}
	return offset, selected
}

// procPanelOrigin returns the top-left cell of the process panel. It mirrors
// the composition in bodyView so mouse events can be mapped back to rows.
func (m Model) procPanelOrigin(l screenLayout) (x, y int) {
	if l.showDisk {
		return l.disk.w, 1 + l.cpu.h + l.gpu.h
	}
	return 0, 1 + l.cpu.h + l.mem.h + l.gpu.h
}

// procPanelContains reports whether a screen cell is inside the process panel.
func (m Model) procPanelContains(x, y int) bool {
	l := m.layout()
	if l.tooSmall {
		return false
	}
	px, py := m.procPanelOrigin(l)
	return x >= px && x < px+l.proc.w && y >= py && y < py+l.proc.h
}

// procAt returns the process under a screen cell in the process table. Cells
// on the border, the column header or below the last row match nothing.
func (m Model) procAt(x, y int) (metrics.Proc, bool) {
	l := m.layout()
	if l.tooSmall || !m.procPanelContains(x, y) {
		return metrics.Proc{}, false
	}
	_, py := m.procPanelOrigin(l)
	row := y - py - 2 // top border and the column header
	if row < 0 {
		return metrics.Proc{}, false
	}
	list := m.visibleProcs()
	rows := m.procRows()
	offset, _ := m.procViewport(list, rows)
	if row >= rows {
		return metrics.Proc{}, false
	}
	idx := offset + row
	if idx < 0 || idx >= len(list) {
		return metrics.Proc{}, false
	}
	return list[idx], true
}

// selectProcessAt selects the row under a mouse click. It returns false when
// the click was not on a process row.
func (m *Model) selectProcessAt(x, y int) bool {
	p, ok := m.procAt(x, y)
	if !ok {
		return false
	}
	m.selectedPID = p.PID
	m.ensureVisible()
	return true
}

func (m *Model) moveSelection(delta int) {
	list := m.visibleProcs()
	if len(list) == 0 {
		return
	}
	current := m.selectedIndex(list)
	idx := clampInt(current+delta, 0, len(list)-1)
	if idx == current {
		return
	}
	m.selectedPID = list[idx].PID
	m.ensureVisible()
}

func (m *Model) selectEdge(last bool) {
	list := m.visibleProcs()
	if len(list) == 0 {
		return
	}
	idx := 0
	if last {
		idx = len(list) - 1
	}
	m.selectedPID = list[idx].PID
	m.ensureVisible()
}

func (m *Model) cycleSort() {
	switch m.sortMode {
	case sortCPU:
		m.sortMode = sortMem
		m.sortDesc = true
	case sortMem:
		m.sortMode = sortPID
		m.sortDesc = false
	case sortPID:
		m.sortMode = sortName
		m.sortDesc = false
	default:
		m.sortMode = sortCPU
		m.sortDesc = true
	}
	m.reconcileSelection()
	m.ensureVisible()
}

func (m Model) sortArrow() string {
	if m.sortDesc {
		return "↓"
	}
	return "↑"
}

func (m Model) processPanel(w, h int) string {
	list := m.visibleProcs()
	title := fmt.Sprintf("PROCESSES %s  ·  %s %s", m.processCountLabel(list), m.sortMode, m.sortArrow())
	if f := m.currentFilter(); f != "" {
		title += fmt.Sprintf("  ·  filter %q", truncate(clean(f), 24))
	}
	if m.paused {
		title += "  ·  PAUSED"
	}
	return m.panel(title, w, h, m.procTableLines(list, w, h))
}

func (m Model) processCountLabel(list []metrics.Proc) string {
	if m.currentFilter() != "" {
		return fmt.Sprintf("%d/%d", len(list), m.snap.ProcCount)
	}
	return strconv.Itoa(m.snap.ProcCount)
}

type procColumns struct {
	user  bool
	memB  bool
	state bool
	nameW int
	width int
}

func procColumnLayout(innerW int) procColumns {
	c := procColumns{
		user:  innerW >= 58,
		memB:  innerW >= 66,
		state: innerW >= 80,
		width: innerW,
	}
	for {
		prefix := 2 + pidWidth + 1 // selection marker, PID and the space before NAME
		if c.user {
			prefix += 1 + userWidth
		}
		suffix := 1 + cpuWidth + 1 + memWidth
		if c.memB {
			suffix += 1 + memBWWidth
		}
		if c.state {
			suffix += 1 + stateWidth
		}
		c.nameW = innerW - prefix - suffix
		if c.nameW >= 10 {
			break
		}
		switch {
		case c.state:
			c.state = false
		case c.memB:
			c.memB = false
		case c.user:
			c.user = false
		default:
			if c.nameW < 4 {
				c.nameW = 4
			}
			return c
		}
	}
	return c
}

func (m Model) procTableLines(list []metrics.Proc, w, h int) []string {
	innerW := w - 2
	innerH := h - 2
	if innerH <= 0 || innerW <= 0 {
		return nil
	}
	cols := procColumnLayout(innerW)

	header := "  " + padLeft("PID", pidWidth)
	if cols.user {
		header += " " + padRight("USER", userWidth)
	}
	header += " " + padRight("NAME", cols.nameW)
	header += " " + padLeft("CPU%", cpuWidth) + " " + padLeft("MEM%", memWidth)
	if cols.memB {
		header += " " + padLeft("MEM", memBWWidth)
	}
	if cols.state {
		header += " " + padLeft("STATE", stateWidth)
	}
	header = fit(header, innerW)

	lines := []string{m.dimText(header)}
	rows := innerH - 1
	if rows <= 0 {
		return lines
	}

	if !m.hasSnap {
		lines = append(lines, m.dimText("Collecting processes…"))
		return lines
	}
	if len(list) == 0 {
		lines = append(lines, m.dimText("No processes match this filter."))
		return lines
	}

	offset, sel := m.procViewport(list, rows)

	for i := 0; i < rows; i++ {
		idx := offset + i
		if idx >= len(list) {
			break
		}
		selected := idx == sel
		lines = append(lines, m.procRow(list[idx], cols, selected))
	}
	return lines
}

func (m Model) procRow(p metrics.Proc, cols procColumns, selected bool) string {
	cpu := fmtCPU(p.CPU)
	name := truncate(clean(p.Name), cols.nameW)
	user := truncate(clean(p.User), userWidth)
	state := truncate(clean(p.State), stateWidth)

	// Selected rows are rendered without inner colours and inverted as a
	// whole. Nested lipgloss styles would reset each other's reverse video.
	if selected {
		row := "▌ " + padLeft(strconv.Itoa(int(p.PID)), pidWidth)
		if cols.user {
			row += " " + padRight(user, userWidth)
		}
		row += " " + padRight(name, cols.nameW)
		row += " " + padLeft(cpu, cpuWidth) + " " + padLeft(fmt.Sprintf("%.1f", p.MemPercent), memWidth)
		if cols.memB {
			row += " " + padLeft(format.Bytes(p.RSS), memBWWidth)
		}
		if cols.state {
			row += " " + padLeft(state, stateWidth)
		}
		row = fit(row, cols.width)
		return lipgloss.NewStyle().Reverse(true).Bold(true).Render(row)
	}

	row := "  " + m.dimText(padLeft(strconv.Itoa(int(p.PID)), pidWidth))
	if cols.user {
		row += " " + m.dimText(padRight(user, userWidth))
	}
	row += " " + padRight(name, cols.nameW)
	row += " " + m.style(m.pal.Severity(p.CPU)).Render(padLeft(cpu, cpuWidth))
	row += " " + m.style(m.pal.Severity(p.MemPercent)).Render(padLeft(fmt.Sprintf("%.1f", p.MemPercent), memWidth))

	if cols.memB {
		row += " " + m.dimText(padLeft(format.Bytes(p.RSS), memBWWidth))
	}
	if cols.state {
		stateCell := padLeft(state, stateWidth)
		switch state {
		case "Z", "T", "t":
			row += " " + m.warnText(stateCell)
		default:
			row += " " + m.dimText(stateCell)
		}
	}
	return fit(row, cols.width)
}

func fmtCPU(v float64) string {
	if v > 9999.9 {
		return "9999+"
	}
	return fmt.Sprintf("%.1f", v)
}
