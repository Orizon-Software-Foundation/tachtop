package ui

import (
	"strings"
	"testing"

	"github.com/Orizon-Software-Foundation/tachtop/internal/config"
	"github.com/Orizon-Software-Foundation/tachtop/internal/metrics"
	"github.com/Orizon-Software-Foundation/tachtop/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

func testGPUs() []metrics.GPU {
	return []metrics.GPU{
		{
			Index: 0, Name: "NVIDIA GeForce RTX 5050 Laptop GPU", Vendor: "NVIDIA",
			UtilPercent: 7, HasUtil: true, Temp: 55, HasTemp: true,
			CoreMHz: 180, MaxCoreMHz: 3090,
			MemMHz: 7001, MaxMemMHz: 7001,
			MemUsed: 289 * 1024 * 1024, MemTotal: 8151 * 1024 * 1024,
			Power: 5.3, PowerLimit: 100,
		},
		{
			Index: 1, Name: "Intel Raptor Lake-P [Intel Graphics]", Vendor: "Intel",
			CoreMHz: 300, MaxCoreMHz: 1400,
		},
	}
}

func gpuTestModel() Model {
	th, _ := theme.Get("tachtop")
	return Model{
		pal:     th,
		cfg:     config.Config{TempUnit: "c"},
		hasSnap: true,
		snap:    metrics.Snapshot{GPUs: testGPUs()},
	}
}

func TestGPULineFits(t *testing.T) {
	m := gpuTestModel()
	for width := 20; width <= 180; width++ {
		line := m.gpuLine(m.snap.GPUs[0], width)
		if got := lipgloss.Width(line); got != width {
			t.Fatalf("width %d: line width = %d (%q)", width, got, plain(line))
		}
	}
}

func TestGPULineShowsDetails(t *testing.T) {
	m := gpuTestModel()
	wide := plain(m.gpuLine(m.snap.GPUs[0], 160))
	for _, want := range []string{"7%", "55°C", "180/3090MHz", "MEM 7001MHz", "VRAM", "5.3W/100W"} {
		if !strings.Contains(wide, want) {
			t.Errorf("wide line missing %q: %q", want, wide)
		}
	}

	narrow := plain(m.gpuLine(m.snap.GPUs[0], 62))
	for _, want := range []string{"7%", "55°C", "180/3090MHz"} {
		if !strings.Contains(narrow, want) {
			t.Errorf("narrow line missing %q: %q", want, narrow)
		}
	}

	intel := plain(m.gpuLine(m.snap.GPUs[1], 80))
	if !strings.Contains(intel, "300/1400MHz") {
		t.Errorf("Intel line missing clock: %q", intel)
	}
}

func TestGPUPanelHeight(t *testing.T) {
	m := gpuTestModel()
	if got := m.gpuPanel(80, 4); len(strings.Split(got, "\n")) != 4 {
		t.Fatalf("panel height = %d, want 4", len(strings.Split(got, "\n")))
	}
	for _, line := range strings.Split(m.gpuPanel(80, 4), "\n") {
		if lipgloss.Width(line) != 80 {
			t.Errorf("panel line width = %d", lipgloss.Width(line))
		}
	}
}

func TestLayoutWithGPUs(t *testing.T) {
	sizes := [][2]int{{64, 22}, {80, 24}, {84, 24}, {100, 30}, {107, 28}, {108, 28}, {130, 40}, {200, 60}}
	for _, size := range sizes {
		m := Model{width: size[0], height: size[1], hasSnap: true, snap: metrics.Snapshot{GPUs: testGPUs()}}
		l := m.layout()
		if l.tooSmall {
			t.Fatalf("%dx%d unexpectedly too small", size[0], size[1])
		}
		if !l.showGPU || l.gpu.w != size[0] || l.gpu.h < 3 {
			t.Fatalf("%dx%d: GPU box = %+v", size[0], size[1], l.gpu)
		}
		bodyH := m.height - 2
		if l.showDisk {
			if l.cpu.h+l.gpu.h+l.disk.h != bodyH {
				t.Errorf("%dx%d: heights sum to %d, want %d", size[0], size[1], l.cpu.h+l.gpu.h+l.disk.h, bodyH)
			}
			if l.disk.h < 8 {
				t.Errorf("%dx%d: disk height %d too small", size[0], size[1], l.disk.h)
			}
		} else if l.cpu.h+l.mem.h+l.gpu.h+l.proc.h != bodyH {
			t.Errorf("%dx%d: heights sum to %d, want %d", size[0], size[1], l.cpu.h+l.mem.h+l.gpu.h+l.proc.h, bodyH)
		}
		if rows := m.procRows(); rows < 1 {
			t.Errorf("%dx%d: procRows = %d", size[0], size[1], rows)
		}
	}
}

func TestLayoutWithManyGPUsIsExact(t *testing.T) {
	for _, count := range []int{1, 2, 5, 8, 16} {
		for width := 64; width <= 220; width++ {
			for height := 22; height <= 70; height++ {
				gpus := make([]metrics.GPU, count)
				m := Model{width: width, height: height, hasSnap: true, snap: metrics.Snapshot{GPUs: gpus}}
				l := m.layout()
				if l.tooSmall {
					t.Fatalf("%dx%d/%d GPUs: unexpectedly too small", width, height, count)
				}
				bodyH := height - 2
				if !l.showGPU || l.gpu.w != width || l.gpu.h < 3 {
					t.Fatalf("%dx%d/%d GPUs: GPU box = %+v", width, height, count, l.gpu)
				}
				if l.showDisk {
					topWidth := l.cpu.w + l.mem.w + l.net.w
					if width < 108 || bodyH < 26 {
						topWidth = l.cpu.w + l.mem.w
					}
					if topWidth != width || l.disk.w+l.proc.w != width {
						t.Fatalf("%dx%d/%d GPUs: top width=%d DISK+PROC=%d", width, height, count, topWidth, l.disk.w+l.proc.w)
					}
					if l.cpu.h+l.gpu.h+l.disk.h != bodyH {
						t.Fatalf("%dx%d/%d GPUs: heights sum to %d, want %d", width, height, count, l.cpu.h+l.gpu.h+l.disk.h, bodyH)
					}
					if width >= 108 && bodyH >= 26 {
						if l.mem.h != l.cpu.h || l.net.h != l.cpu.h {
							t.Fatalf("%dx%d/%d GPUs: wide top heights MEM=%d NET=%d CPU=%d", width, height, count, l.mem.h, l.net.h, l.cpu.h)
						}
					} else if l.mem.h+l.net.h != l.cpu.h {
						t.Fatalf("%dx%d/%d GPUs: MEM+NET heights %d, CPU height %d", width, height, count, l.mem.h+l.net.h, l.cpu.h)
					}
					if l.disk.h < 8 {
						t.Fatalf("%dx%d/%d GPUs: disk height %d", width, height, count, l.disk.h)
					}
				} else {
					if l.cpu.w != width || l.mem.w+l.net.w != width {
						t.Fatalf("%dx%d/%d GPUs: row widths CPU=%d MEM+NET=%d", width, height, count, l.cpu.w, l.mem.w+l.net.w)
					}
					if l.cpu.h+l.mem.h+l.gpu.h+l.proc.h != bodyH {
						t.Fatalf("%dx%d/%d GPUs: heights sum to %d, want %d", width, height, count, l.cpu.h+l.mem.h+l.gpu.h+l.proc.h, bodyH)
					}
					if l.mem.h != l.net.h {
						t.Fatalf("%dx%d/%d GPUs: MEM/NET heights differ", width, height, count)
					}
				}
				if l.cpu.w <= 0 || l.cpu.h <= 0 || l.mem.w <= 0 || l.mem.h <= 0 ||
					l.net.w <= 0 || l.net.h <= 0 || l.gpu.h <= 0 || l.proc.w <= 0 || l.proc.h <= 0 {
					t.Fatalf("%dx%d/%d GPUs: empty box in %+v", width, height, count, l)
				}
				if m.procRows() < 1 {
					t.Fatalf("%dx%d/%d GPUs: no process rows", width, height, count)
				}
			}
		}
	}
}

func TestGPUOverflowNote(t *testing.T) {
	gpus := make([]metrics.GPU, 6)
	m := Model{snap: metrics.Snapshot{GPUs: gpus}}
	lines := m.gpuLines(80, 5) // three inner rows: two GPUs plus a note
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if !strings.Contains(plain(lines[2]), "4 more") {
		t.Fatalf("missing overflow note: %q", plain(lines[2]))
	}
}

func TestWideLayoutShowsAllGPUsOnTallTerminal(t *testing.T) {
	gpus := make([]metrics.GPU, 16)
	m := Model{width: 130, height: 40, hasSnap: true, snap: metrics.Snapshot{GPUs: gpus}}
	l := m.layout()
	if l.gpu.h-2 < len(gpus) {
		t.Fatalf("GPU panel has %d content rows for %d GPUs", l.gpu.h-2, len(gpus))
	}
}
