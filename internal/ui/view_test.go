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
