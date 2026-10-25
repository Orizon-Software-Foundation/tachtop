package ui

import (
	"strings"
	"testing"

	"github.com/Orizon-Software-Foundation/tachtop/internal/config"
	"github.com/Orizon-Software-Foundation/tachtop/internal/metrics"
	"github.com/Orizon-Software-Foundation/tachtop/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

func TestDetailsScroll(t *testing.T) {
	th, _ := theme.Get("tachtop")
	m := Model{
		width:       80,
		height:      24,
		pal:         th,
		detailsOpen: true,
		detailsPID:  1,
		details: &metrics.ProcDetails{
			PID:     1,
			Name:    "test",
			Cmdline: strings.Repeat("word ", 200),
		},
	}

	lines := m.detailsLines(m.width, m.height-2)
	page := m.detailsPage()
	if len(lines) <= page {
		t.Fatalf("expected scrollable content: %d lines, page %d", len(lines), page)
	}

	m.scrollDetails(5)
	if m.detailsOffset != 5 {
		t.Fatalf("offset = %d, want 5", m.detailsOffset)
	}
	m.scrollDetails(1 << 30)
	if want := len(lines) - page; m.detailsOffset != want {
		t.Fatalf("offset = %d, want %d", m.detailsOffset, want)
	}

	panel := m.detailsPanel()
	panelLines := strings.Split(panel, "\n")
	if len(panelLines) != m.height-2 {
		t.Fatalf("panel height = %d, want %d", len(panelLines), m.height-2)
	}
	for _, line := range panelLines {
		if lipgloss.Width(line) != m.width {
			t.Fatalf("panel line width = %d, want %d", lipgloss.Width(line), m.width)
		}
	}
}

func TestViewTooSmallFits(t *testing.T) {
	for _, size := range [][2]int{{10, 5}, {20, 10}, {40, 12}, {63, 22}} {
		m := Model{width: size[0], height: size[1]}
		got := m.View()
		lines := strings.Split(got, "\n")
		if len(lines) > size[1] {
			t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) > size[0] {
				t.Errorf("%dx%d: line %q is too wide", size[0], size[1], line)
			}
		}
	}
}

func TestCPUStatsIncludePower(t *testing.T) {
	th, _ := theme.Get("tachtop")
	m := Model{
		pal: th,
		cfg: config.Config{TempUnit: "c"},
		snap: metrics.Snapshot{
			CPUCores:    8,
			CPUMHz:      2400,
			CPUTemp:     61,
			CPUPower:    45.2,
			HasCPUPower: true,
			CPUPercent:  12,
		},
		histCPU: newSeries(150),
	}
	lines := plain(strings.Join(m.cpuLines(80, 14), "\n"))
	for _, want := range []string{"8 cores", "2.4 GHz", "61°C", "45W"} {
		if !strings.Contains(lines, want) {
			t.Errorf("CPU stats missing %q: %q", want, lines)
		}
	}

	m.snap.HasCPUPower = false
	lines = plain(strings.Join(m.cpuLines(80, 14), "\n"))
	if strings.Contains(lines, "45W") {
		t.Errorf("unavailable CPU power should be hidden: %q", lines)
	}
}

func TestHelpFitsMinimumTerminal(t *testing.T) {
	th, _ := theme.Get("tachtop")
	m := Model{width: 64, height: 22, pal: th}
	got := plain(m.helpPanel())
	if !strings.Contains(got, "Themes:") {
		t.Fatalf("help content is clipped at the minimum terminal size: %q", got)
	}
}

func TestNewNormalizesRefresh(t *testing.T) {
	th, _ := theme.Get("tachtop")
	m := New(Options{Config: config.Config{RefreshMS: 1}, Theme: th})
	if m.cfg.RefreshMS != config.MinRefreshMS {
		t.Errorf("low refresh = %d, want %d", m.cfg.RefreshMS, config.MinRefreshMS)
	}
	m = New(Options{Config: config.Config{RefreshMS: 1 << 30}, Theme: th})
	if m.cfg.RefreshMS != config.MaxRefreshMS {
		t.Errorf("high refresh = %d, want %d", m.cfg.RefreshMS, config.MaxRefreshMS)
	}
	m = New(Options{Config: config.Config{}, Theme: th})
	if m.cfg.RefreshMS != config.DefaultRefreshMS {
		t.Errorf("zero refresh = %d, want %d", m.cfg.RefreshMS, config.DefaultRefreshMS)
	}
}

func TestHelpShowsLoadedConfigPath(t *testing.T) {
	th, _ := theme.Get("tachtop")
	m := Model{width: 100, height: 30, pal: th, cfgPath: "/tmp/custom.json"}
	if got := plain(m.helpPanel()); !strings.Contains(got, "/tmp/custom.json") {
		t.Fatalf("help does not show config path: %q", got)
	}
}

func TestNetLinesFit(t *testing.T) {
	th, _ := theme.Get("tachtop")
	m := Model{
		width:   100,
		height:  24,
		pal:     th,
		hasSnap: true,
		snap: metrics.Snapshot{
			NetRx: 12345,
			NetTx: 6789,
			Networks: []metrics.Network{
				{Name: "eth0", Rx: 1000, Tx: 2000, BytesRecv: 1 << 30, BytesSent: 2 << 30, MTU: 1500, Flags: "UP,BROADCAST,RUNNING,MULTICAST"},
				{Name: "wlan0", Rx: 500, Tx: 100, BytesRecv: 512, BytesSent: 256, MTU: 1400, Flags: "UP,BROADCAST,MULTICAST"},
				{Name: "lo", Rx: 10, Tx: 10, BytesRecv: 0, BytesSent: 0, MTU: 65536},
			},
		},
		histNet: newSeries(150),
		netHist: map[string]*series{
			"eth0":  {limit: 150, values: []float64{1, 2, 3, 4, 5}},
			"wlan0": {limit: 150, values: []float64{5, 4, 3, 2, 1}},
		},
	}
	for h := 6; h <= 40; h++ {
		lines := m.netLines(100, h)
		for _, line := range lines {
			if lipgloss.Width(line) > 100 {
				t.Fatalf("height %d: line too wide: %q", h, line)
			}
		}
	}
}

func TestNetPerIfaceGraphs(t *testing.T) {
	th, _ := theme.Get("tachtop")
	m := Model{pal: th, netHist: map[string]*series{
		"eth0":  {limit: 10, values: []float64{10, 20, 30}},
		"wlan0": {limit: 10, values: []float64{5, 15, 25}},
	}}
	ifaces := []metrics.Network{
		{Name: "eth0"},
		{Name: "wlan0"},
	}
	lines := m.netPerIfaceGraphs(ifaces, 40, 6)
	if len(lines) == 0 {
		t.Fatal("expected per-interface graph lines")
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 40 {
			t.Errorf("graph line too wide: %q", line)
		}
	}

	// Too little room falls back to aggregate.
	short := m.netPerIfaceGraphs(ifaces, 40, 2)
	if len(short) == 0 {
		t.Fatal("expected fallback graph lines for tiny height")
	}
}
