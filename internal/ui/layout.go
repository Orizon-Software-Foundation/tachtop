package ui

import "math"

// box is a panel size in terminal cells.
type box struct {
	w, h int
}

// screenLayout describes where each panel goes. Panels are composed with
// lipgloss rather than absolute coordinates, so only sizes are needed.
type screenLayout struct {
	tooSmall bool

	cpu  box
	mem  box
	net  box
	gpu  box
	disk box
	proc box

	showDisk bool
	showNet  bool
	showGPU  bool
}

func (m Model) layout() screenLayout {
	var l screenLayout
	bodyH := m.height - 2
	if m.width < 64 || m.height < 22 || bodyH < 18 {
		l.tooSmall = true
		return l
	}

	gpuCount := 0
	if m.hasSnap {
		gpuCount = len(m.snap.GPUs)
	}

	switch {
	case m.width >= 108 && bodyH >= 26:
		// Keep a usable top row and bottom row around the GPU strip. If
		// there are many adapters, use the spare height instead of hiding
		// them behind an overflow note.
		gpuH := gpuPanelHeight(gpuCount, maxInt(1, bodyH-11-9-2))
		if gpuH > 0 {
			gpuH = clampInt(gpuH, 3, maxInt(3, bodyH-11-9))
		}
		topH := clampInt(int(math.Round(float64(bodyH)*0.54)), 11, bodyH-9)
		if gpuH > 0 {
			topH = clampInt(int(math.Round(float64(bodyH)*0.46)), 11, bodyH-gpuH-9)
		}
		cpuW := m.width * 45 / 100
		memW := m.width * 28 / 100
		netW := m.width - cpuW - memW
		diskW := clampInt(m.width*30/100, 30, 50)
		l.cpu = box{cpuW, topH}
		l.mem = box{memW, topH}
		l.net = box{netW, topH}
		l.gpu = box{m.width, gpuH}
		l.disk = box{diskW, bodyH - topH - gpuH}
		l.proc = box{m.width - diskW, bodyH - topH - gpuH}
		l.showDisk = true
		l.showNet = true
		l.showGPU = gpuH > 0

	case m.width >= 84 && bodyH >= 22:
		gpuH := gpuPanelHeight(gpuCount, maxInt(1, bodyH-10-8-2))
		if gpuH > 0 {
			gpuH = clampInt(gpuH, 3, maxInt(3, bodyH-10-8))
		}
		topH := clampInt(int(math.Round(float64(bodyH)*0.52)), 10, bodyH-9)
		if gpuH > 0 {
			topH = clampInt(int(math.Round(float64(bodyH)*0.44)), 10, bodyH-gpuH-8)
		}
		cpuW := m.width * 57 / 100
		memW := m.width - cpuW
		memH := topH / 2
		netH := topH - memH
		diskW := clampInt(m.width*32/100, 26, 40)
		l.cpu = box{cpuW, topH}
		l.mem = box{memW, memH}
		l.net = box{memW, netH}
		l.gpu = box{m.width, gpuH}
		l.disk = box{diskW, bodyH - topH - gpuH}
		l.proc = box{m.width - diskW, bodyH - topH - gpuH}
		l.showDisk = true
		l.showNet = true
		l.showGPU = gpuH > 0

	default:
		cpuH := clampInt(int(math.Round(float64(bodyH)*0.34)), 7, 12)
		midH := clampInt(int(math.Round(float64(bodyH)*0.25)), 5, 9)
		gpuH := 0
		if gpuCount > 0 {
			cpuH = clampInt(int(math.Round(float64(bodyH)*0.28)), 6, 9)
			midH = clampInt(int(math.Round(float64(bodyH)*0.20)), 4, 6)
			maxRows := maxInt(1, bodyH-cpuH-midH-4-2)
			gpuH = gpuPanelHeight(gpuCount, maxRows)
			gpuH = clampInt(gpuH, 3, maxInt(3, bodyH-cpuH-midH-4))
		}
		procH := bodyH - cpuH - midH - gpuH
		minProc := 7
		if gpuH > 0 {
			minProc = 4
		}
		if procH < minProc {
			take := minInt(minProc-procH, maxInt(0, cpuH-6))
			cpuH -= take
			procH += take
		}
		if procH < minProc {
			take := minInt(minProc-procH, maxInt(0, midH-4))
			midH -= take
			procH += take
		}
		half := m.width / 2
		l.cpu = box{m.width, cpuH}
		l.mem = box{half, midH}
		l.net = box{m.width - half, midH}
		l.gpu = box{m.width, gpuH}
		l.proc = box{m.width, procH}
		l.showDisk = false
		l.showNet = true
		l.showGPU = gpuH > 0
	}
	return l
}

// gpuPanelHeight returns the height of the one-line-per-GPU strip. maxRows
// caps how much vertical space GPUs may take from the rest of the layout.
func gpuPanelHeight(count, maxRows int) int {
	if count <= 0 || maxRows <= 0 {
		return 0
	}
	return 2 + minInt(count, maxRows)
}

func (m Model) procRows() int {
	l := m.layout()
	if l.tooSmall {
		return 1
	}
	rows := l.proc.h - 3 // top border, bottom border and column header
	if rows < 1 {
		rows = 1
	}
	return rows
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
