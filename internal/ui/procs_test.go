package ui

import (
	"fmt"
	"testing"

	"github.com/Orizon-Software-Foundation/tachtop/internal/metrics"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testProcs() []metrics.Proc {
	return []metrics.Proc{
		{PID: 10, Name: "alpha", User: "ann", CPU: 1.0, MemPercent: 5.0},
		{PID: 20, Name: "beta", User: "bob", CPU: 20.0, MemPercent: 1.0},
		{PID: 30, Name: "gamma", User: "ann", CPU: 10.0, MemPercent: 9.0},
	}
}

func TestVisibleProcsSorts(t *testing.T) {
	m := Model{snap: metrics.Snapshot{Procs: testProcs()}, sortMode: sortCPU, sortDesc: true}
	got := m.visibleProcs()
	if len(got) != 3 || got[0].PID != 20 || got[1].PID != 30 || got[2].PID != 10 {
		t.Fatalf("cpu sort = %v", pids(got))
	}

	m.sortMode = sortMem
	got = m.visibleProcs()
	if got[0].PID != 30 || got[1].PID != 10 || got[2].PID != 20 {
		t.Fatalf("mem sort = %v", pids(got))
	}

	m.sortMode = sortPID
	m.sortDesc = false
	got = m.visibleProcs()
	if got[0].PID != 10 || got[1].PID != 20 || got[2].PID != 30 {
		t.Fatalf("pid sort = %v", pids(got))
	}

	m.sortMode = sortName
	got = m.visibleProcs()
	if got[0].Name != "alpha" || got[1].Name != "beta" || got[2].Name != "gamma" {
		t.Fatalf("name sort = %v", names(got))
	}

	m.sortDesc = true
	got = m.visibleProcs()
	if got[0].Name != "gamma" {
		t.Fatalf("reverse name sort = %v", names(got))
	}
}

func TestVisibleProcsFilter(t *testing.T) {
	m := Model{snap: metrics.Snapshot{Procs: testProcs()}, sortMode: sortPID}
	tests := []struct {
		filter string
		want   []int32
	}{
		{"", []int32{10, 20, 30}},
		{"ANN", []int32{10, 30}},
		{"bet", []int32{20}},
		{"2", []int32{20}},
		{"nothing", nil},
	}
	for _, tt := range tests {
		m.filter = tt.filter
		if got := pids(m.visibleProcs()); fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("filter %q = %v, want %v", tt.filter, got, tt.want)
		}
	}
}

func TestSelectionFollowsTopUntilPicked(t *testing.T) {
	m := Model{snap: metrics.Snapshot{Procs: testProcs()}, sortMode: sortCPU, sortDesc: true}
	list := m.visibleProcs()
	if got := m.selectedIndex(list); got != 0 {
		t.Fatalf("initial index = %d", got)
	}

	m.reconcileSelection()
	if m.selectedPID != 0 {
		t.Fatalf("selection should stay unpinned, got %d", m.selectedPID)
	}

	m.moveSelection(1)
	if m.selectedPID != 30 {
		t.Fatalf("selectedPID = %d, want 30", m.selectedPID)
	}

	// Removing the selected process should fall back to the top row.
	m.snap.Procs = m.snap.Procs[:2]
	m.reconcileSelection()
	if m.selectedPID != 0 {
		t.Fatalf("selection should reset, got %d", m.selectedPID)
	}
	if got := m.selectedIndex(m.visibleProcs()); got != 0 {
		t.Fatalf("reset index = %d", got)
	}
}

func TestMoveSelectionAtEdges(t *testing.T) {
	m := Model{snap: metrics.Snapshot{Procs: testProcs()}, sortMode: sortCPU, sortDesc: true}
	m.moveSelection(-1)
	if m.selectedPID != 0 {
		t.Fatalf("moving up at the top should not pin, got %d", m.selectedPID)
	}
	m.moveSelection(100)
	if m.selectedPID != 10 {
		t.Fatalf("moving down should select the last row, got %d", m.selectedPID)
	}
}

func TestSelectProcessAt(t *testing.T) {
	m := Model{
		width:    120,
		height:   40,
		hasSnap:  true,
		snap:     metrics.Snapshot{Procs: testProcs(), ProcCount: len(testProcs())},
		sortMode: sortPID,
		sortDesc: false,
	}
	l := m.layout()
	if !l.showDisk {
		t.Fatal("expected a layout with the disk panel")
	}
	px, py := m.procPanelOrigin(l)
	list := m.visibleProcs()

	for i, want := range []int32{10, 20, 30} {
		if !m.selectProcessAt(px+2, py+2+i) {
			t.Fatalf("row %d did not select", i)
		}
		if m.selectedPID != want {
			t.Fatalf("row %d selected PID %d, want %d", i, m.selectedPID, want)
		}
	}

	// Empty space below the last row, the header, the border and the CPU
	// panel must not change the selection.
	for _, pt := range [][2]int{
		{px + 2, py + 2 + len(list)},
		{px + 1, py + 1},
		{px, py},
		{2, 2},
	} {
		if m.selectProcessAt(pt[0], pt[1]) {
			t.Errorf("click at (%d,%d) unexpectedly selected a process", pt[0], pt[1])
		}
		if m.selectedPID != 30 {
			t.Errorf("click at (%d,%d) changed selection to %d", pt[0], pt[1], m.selectedPID)
		}
	}
}

func TestSelectProcessAtNarrowLayout(t *testing.T) {
	m := Model{
		width:    70,
		height:   30,
		hasSnap:  true,
		snap:     metrics.Snapshot{Procs: testProcs(), ProcCount: len(testProcs())},
		sortMode: sortPID,
	}
	l := m.layout()
	if l.showDisk {
		t.Fatal("expected a narrow layout without the disk panel")
	}
	px, py := m.procPanelOrigin(l)
	if !m.selectProcessAt(px+2, py+3) {
		t.Fatal("click on the second row did not select")
	}
	if m.selectedPID != 20 {
		t.Fatalf("selected PID %d, want 20", m.selectedPID)
	}
}

func TestSelectProcessAtScrolledList(t *testing.T) {
	procs := make([]metrics.Proc, 100)
	for i := range procs {
		procs[i] = metrics.Proc{PID: int32(i + 1), Name: fmt.Sprintf("p%d", i+1)}
	}
	m := Model{
		width:      120,
		height:     40,
		hasSnap:    true,
		snap:       metrics.Snapshot{Procs: procs, ProcCount: len(procs)},
		sortMode:   sortPID,
		procOffset: 50,
	}
	l := m.layout()
	px, py := m.procPanelOrigin(l)
	m.ensureVisible()

	list := m.visibleProcs()
	rows := m.procRows()
	offset, _ := m.procViewport(list, rows)
	if offset != 50 {
		t.Fatalf("offset = %d, want 50", offset)
	}
	if !m.selectProcessAt(px+2, py+2) {
		t.Fatal("first visible row did not select")
	}
	if want := list[offset].PID; m.selectedPID != want {
		t.Fatalf("selected PID %d, want %d", m.selectedPID, want)
	}
	if !m.selectProcessAt(px+2, py+2+rows-1) {
		t.Fatal("last visible row did not select")
	}
	if want := list[offset+rows-1].PID; m.selectedPID != want {
		t.Fatalf("selected PID %d, want %d", m.selectedPID, want)
	}
}

func TestMouseWheelOnlyScrollsProcessPanel(t *testing.T) {
	m := Model{
		width:    120,
		height:   40,
		hasSnap:  true,
		snap:     metrics.Snapshot{Procs: testProcs(), ProcCount: len(testProcs())},
		sortMode: sortPID,
	}
	l := m.layout()
	px, py := m.procPanelOrigin(l)
	list := m.visibleProcs()

	// A wheel event over the CPU panel is ignored.
	updated, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
		X:      2,
		Y:      2,
	})
	if got := updated.(Model); got.selectedPID != 0 {
		t.Fatalf("wheel outside the process panel selected PID %d", got.selectedPID)
	}

	// A wheel event over the process table moves the selection.
	updated, _ = m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
		X:      px + 2,
		Y:      py + 3,
	})
	if got := updated.(Model); got.selectedPID != list[len(list)-1].PID {
		t.Fatalf("wheel selected PID %d, want %d", got.selectedPID, list[len(list)-1].PID)
	}
}

func TestMouseClickDispatchesSelection(t *testing.T) {
	m := Model{
		width:    120,
		height:   40,
		hasSnap:  true,
		snap:     metrics.Snapshot{Procs: testProcs(), ProcCount: len(testProcs())},
		sortMode: sortPID,
	}
	l := m.layout()
	px, py := m.procPanelOrigin(l)
	list := m.visibleProcs()

	updated, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      px + 2,
		Y:      py + 3,
	})
	if got := updated.(Model); got.selectedPID != list[1].PID {
		t.Fatalf("click selected PID %d, want %d", got.selectedPID, list[1].PID)
	}
}

func TestProcColumnLayout(t *testing.T) {
	for inner := 28; inner <= 160; inner++ {
		cols := procColumnLayout(inner)
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
		if got := lipgloss.Width(header); got > inner {
			t.Fatalf("inner %d: header width %d exceeds space", inner, got)
		}
		if cols.nameW < 4 {
			t.Fatalf("inner %d: name width %d", inner, cols.nameW)
		}
	}
}

func TestLayoutFitsScreen(t *testing.T) {
	sizes := [][2]int{{64, 22}, {80, 24}, {100, 30}, {107, 28}, {108, 28}, {130, 40}, {200, 60}}
	for _, size := range sizes {
		m := Model{width: size[0], height: size[1]}
		l := m.layout()
		if l.tooSmall {
			t.Fatalf("%dx%d unexpectedly too small", size[0], size[1])
		}
		bodyH := m.height - 2
		if l.showDisk {
			if m.width >= 108 {
				if l.cpu.h != l.mem.h || l.cpu.h != l.net.h {
					t.Errorf("%dx%d: top heights differ: %d %d %d", size[0], size[1], l.cpu.h, l.mem.h, l.net.h)
				}
			}
			if l.disk.h != l.proc.h {
				t.Errorf("%dx%d: bottom heights differ: %d %d", size[0], size[1], l.disk.h, l.proc.h)
			}
			if l.cpu.h+l.disk.h != bodyH {
				t.Errorf("%dx%d: heights sum to %d, want %d", size[0], size[1], l.cpu.h+l.disk.h, bodyH)
			}
			if l.disk.w+l.proc.w != m.width {
				t.Errorf("%dx%d: bottom widths sum to %d", size[0], size[1], l.disk.w+l.proc.w)
			}
			if m.width >= 108 {
				if l.cpu.w+l.mem.w+l.net.w != m.width {
					t.Errorf("%dx%d: top widths sum to %d", size[0], size[1], l.cpu.w+l.mem.w+l.net.w)
				}
			} else if l.cpu.w+l.mem.w != m.width {
				t.Errorf("%dx%d: top widths sum to %d", size[0], size[1], l.cpu.w+l.mem.w)
			}
			if m.width < 108 && l.mem.h+l.net.h != l.cpu.h {
				t.Errorf("%dx%d: right column heights sum to %d, want %d", size[0], size[1], l.mem.h+l.net.h, l.cpu.h)
			}
		} else {
			if l.cpu.w != m.width {
				t.Errorf("%dx%d: cpu width = %d", size[0], size[1], l.cpu.w)
			}
			if l.mem.w+l.net.w != m.width {
				t.Errorf("%dx%d: mid widths sum to %d", size[0], size[1], l.mem.w+l.net.w)
			}
			if l.cpu.h+l.mem.h+l.proc.h != bodyH {
				t.Errorf("%dx%d: heights sum to %d, want %d", size[0], size[1], l.cpu.h+l.mem.h+l.proc.h, bodyH)
			}
		}
		if rows := m.procRows(); rows < 1 {
			t.Errorf("%dx%d: procRows = %d", size[0], size[1], rows)
		}
	}
}

func TestLayoutTooSmall(t *testing.T) {
	for _, size := range [][2]int{{63, 22}, {64, 21}, {40, 12}} {
		m := Model{width: size[0], height: size[1]}
		if !m.layout().tooSmall {
			t.Errorf("%dx%d should be too small", size[0], size[1])
		}
	}
}

func pids(procs []metrics.Proc) []int32 {
	out := make([]int32, len(procs))
	for i, p := range procs {
		out[i] = p.PID
	}
	return out
}

func names(procs []metrics.Proc) []string {
	out := make([]string, len(procs))
	for i, p := range procs {
		out[i] = p.Name
	}
	return out
}
