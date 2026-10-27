package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Orizon-Software-Foundation/tachtop/internal/config"
	"github.com/Orizon-Software-Foundation/tachtop/internal/metrics"
	"github.com/Orizon-Software-Foundation/tachtop/internal/theme"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sortMode int

const (
	sortCPU sortMode = iota
	sortMem
	sortPID
	sortName
)

func (s sortMode) String() string {
	switch s {
	case sortMem:
		return "mem"
	case sortPID:
		return "pid"
	case sortName:
		return "name"
	default:
		return "cpu"
	}
}

// Options configures the UI.
type Options struct {
	Config     config.Config
	Theme      theme.Theme
	Version    string
	ConfigPath string
}

// Model is the Bubble Tea application state.
type Model struct {
	cfg     config.Config
	pal     theme.Theme
	version string
	cfgPath string

	collector *metrics.Collector
	width     int
	height    int

	snap       metrics.Snapshot
	hasSnap    bool
	collecting bool
	paused     bool
	collectSeq int
	tickSeq    int

	histCPU  series
	histMem  series
	histNet  series
	histDisk series

	netHist map[string]*series

	sortMode sortMode
	sortDesc bool
	filter   string

	filtering   bool
	filterSaved string
	filterInput textinput.Model

	selectedPID int32
	procOffset  int

	helpOpen bool

	detailsOpen   bool
	detailsPID    int32
	details       *metrics.ProcDetails
	detailsOffset int

	kill *metrics.Proc

	toast    string
	toastErr bool
	toastSeq int
}

// New builds the initial model. The collector reads static host information
// here; the first asynchronous collection establishes the rate baselines, so
// startup never blocks on slow hardware probes.
func New(opts Options) Model {
	cfg := opts.Config
	if cfg.RefreshMS <= 0 {
		cfg.RefreshMS = config.DefaultRefreshMS
	}
	cfg.RefreshMS = clampInt(cfg.RefreshMS, config.MinRefreshMS, config.MaxRefreshMS)

	ti := textinput.New()
	ti.Placeholder = "name or pid"
	ti.CharLimit = 80
	ti.Width = 30
	ti.Prompt = ""
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(opts.Theme.Dim)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(opts.Theme.Accent)

	return Model{
		cfg:         cfg,
		pal:         opts.Theme,
		version:     opts.Version,
		cfgPath:     opts.ConfigPath,
		collector:   metrics.New(),
		histCPU:     newSeries(150),
		histMem:     newSeries(150),
		histNet:     newSeries(150),
		histDisk:    newSeries(150),
		netHist:     make(map[string]*series),
		sortMode:    sortCPU,
		sortDesc:    true,
		filterInput: ti,
	}
}

type snapshotMsg struct {
	seq  int
	snap metrics.Snapshot
}

type tickMsg struct {
	seq int
}

type detailsMsg struct {
	pid     int32
	details metrics.ProcDetails
}

type actionMsg struct {
	pid   int32
	name  string
	force bool
	err   error
}

type toastMsg struct {
	seq int
}

// Init starts the first collection.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		return snapshotMsg{seq: 0, snap: m.collector.Collect()}
	}
}

func (m Model) collectCmd() tea.Cmd {
	seq := m.collectSeq
	collector := m.collector
	return func() tea.Msg {
		return snapshotMsg{seq: seq, snap: collector.Collect()}
	}
}

// resetAndCollectCmd clears the paused baselines and takes a fresh snapshot in
// the background. The collector serializes the reset and collection, and doing
// this off the UI goroutine keeps the interface responsive.
func (m Model) resetAndCollectCmd() tea.Cmd {
	seq := m.collectSeq
	collector := m.collector
	return func() tea.Msg {
		return snapshotMsg{seq: seq, snap: collector.CollectAfterReset()}
	}
}

func (m *Model) scheduleTick() tea.Cmd {
	m.tickSeq++
	seq := m.tickSeq
	ms := m.cfg.RefreshMS
	if ms <= 0 {
		ms = config.DefaultRefreshMS
	}
	ms = clampInt(ms, config.MinRefreshMS, config.MaxRefreshMS)
	interval := time.Duration(ms) * time.Millisecond
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return tickMsg{seq: seq}
	})
}

// Update handles messages. The model is a value; returning it propagates all
// mutations to Bubble Tea.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisible()
		return m, nil

	case tickMsg:
		if m.paused || m.collecting || msg.seq != m.tickSeq {
			return m, nil
		}
		m.collecting = true
		m.collectSeq++
		return m, m.collectCmd()

	case snapshotMsg:
		if msg.seq != m.collectSeq {
			return m, nil
		}
		return m.applySnapshot(msg.snap)

	case detailsMsg:
		if m.detailsOpen && msg.pid == m.detailsPID {
			m.details = &msg.details
		}
		return m, nil

	case actionMsg:
		if msg.err != nil {
			return m, m.flash(fmt.Sprintf("Signal to %d failed: %v", msg.pid, msg.err), true)
		}
		action := "SIGTERM"
		if msg.force {
			action = "SIGKILL"
		}
		return m, m.flash(fmt.Sprintf("Sent %s to %d (%s)", action, msg.pid, msg.name), false)

	case toastMsg:
		if msg.seq == m.toastSeq {
			m.toast = ""
			m.toastErr = false
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) applySnapshot(snap metrics.Snapshot) (tea.Model, tea.Cmd) {
	m.collecting = false
	m.hasSnap = true
	m.snap = snap

	m.histCPU.add(snap.CPUPercent)
	if snap.MemTotal > 0 {
		m.histMem.add(float64(snap.MemUsed) / float64(snap.MemTotal) * 100)
	} else {
		m.histMem.add(0)
	}
	m.histNet.add(snap.NetRx + snap.NetTx)
	m.histDisk.add(snap.DiskRead + snap.DiskWrite)

	// Per-interface rate history for the detailed network panel.
	seen := make(map[string]bool, len(snap.Networks))
	for _, n := range snap.Networks {
		h, ok := m.netHist[n.Name]
		if !ok {
			h = &series{limit: 150, values: make([]float64, 0, 150)}
			m.netHist[n.Name] = h
		}
		h.add(n.Rx + n.Tx)
		seen[n.Name] = true
	}
	for name := range m.netHist {
		if !seen[name] {
			delete(m.netHist, name)
		}
	}

	m.reconcileSelection()
	m.ensureVisible()

	if m.paused {
		return m, nil
	}
	return m, m.scheduleTick()
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.kill != nil {
		switch key {
		case "y", "Y":
			force := key == "Y"
			pid, name, created := m.kill.PID, m.kill.Name, m.kill.Created
			collector := m.collector
			m.kill = nil
			return m, func() tea.Msg {
				return actionMsg{
					pid:   pid,
					name:  name,
					force: force,
					err:   collector.SignalChecked(pid, created, force),
				}
			}
		case "n", "esc":
			m.kill = nil
			return m, m.flash("Cancelled", false)
		}
		return m, nil
	}

	if m.helpOpen {
		if key == "esc" || key == "q" || key == "?" {
			m.helpOpen = false
		}
		return m, nil
	}

	if m.detailsOpen {
		switch key {
		case "esc", "q", "enter":
			m.closeDetails()
		case "up":
			m.scrollDetails(-1)
		case "down":
			m.scrollDetails(1)
		case "pgup", "ctrl+b":
			m.scrollDetails(-m.detailsPage())
		case "pgdown", "ctrl+f":
			m.scrollDetails(m.detailsPage())
		case "home":
			m.detailsOffset = 0
		case "end":
			m.scrollDetails(1 << 30)
		}
		return m, nil
	}

	if m.filtering {
		switch key {
		case "esc":
			m.filtering = false
			m.filterInput.Blur()
			m.filterInput.SetValue(m.filterSaved)
			m.reconcileSelection()
			m.ensureVisible()
			return m, nil
		case "enter":
			m.filtering = false
			m.filterInput.Blur()
			m.filter = strings.TrimSpace(m.filterInput.Value())
			m.procOffset = 0
			m.reconcileSelection()
			m.ensureVisible()
			return m, nil
		}
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.reconcileSelection()
		m.ensureVisible()
		return m, cmd
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.helpOpen = true
	case "p":
		return m, m.togglePause()
	case "/":
		m.beginFilter()
	case "c":
		if m.filter != "" {
			m.filter = ""
			m.filterInput.SetValue("")
			m.procOffset = 0
			m.reconcileSelection()
			m.ensureVisible()
		}
	case "s":
		m.cycleSort()
	case "r":
		m.sortDesc = !m.sortDesc
		m.reconcileSelection()
		m.ensureVisible()
	case "up":
		m.moveSelection(-1)
	case "down":
		m.moveSelection(1)
	case "pgup", "ctrl+b":
		m.moveSelection(-m.procRows())
	case "pgdown", "ctrl+f":
		m.moveSelection(m.procRows())
	case "home":
		m.selectEdge(false)
	case "end":
		m.selectEdge(true)
	case "enter":
		return m, m.openDetails()
	case "k":
		return m, m.beginKill()
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.filterInput.SetValue("")
			m.procOffset = 0
			m.reconcileSelection()
			m.ensureVisible()
		}
	}

	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if m.detailsOpen {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollDetails(-3)
		case tea.MouseButtonWheelDown:
			m.scrollDetails(3)
		}
		return m, nil
	}
	if m.filtering || m.helpOpen || m.kill != nil {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonLeft:
		m.selectProcessAt(msg.X, msg.Y)
	case tea.MouseButtonWheelUp:
		if m.procPanelContains(msg.X, msg.Y) {
			m.moveSelection(-3)
		}
	case tea.MouseButtonWheelDown:
		if m.procPanelContains(msg.X, msg.Y) {
			m.moveSelection(3)
		}
	}
	return m, nil
}

func (m *Model) togglePause() tea.Cmd {
	m.paused = !m.paused
	m.tickSeq++
	if m.paused {
		m.collectSeq++ // ignore any snapshot already in flight
		return m.flash("Paused", false)
	}

	// Reset and Collect run in the background. The collector serializes them,
	// so an in-flight snapshot cannot race the reset, and the UI stays
	// responsive while the old collection finishes.
	m.collectSeq++
	m.collecting = true
	return tea.Batch(m.resetAndCollectCmd(), m.flash("Resumed", false))
}

func (m *Model) beginFilter() {
	m.filtering = true
	m.filterSaved = m.filter
	m.filterInput.SetValue(m.filter)
	m.filterInput.CursorEnd()
	m.filterInput.Focus()
}

func (m *Model) openDetails() tea.Cmd {
	p := m.selectedProc()
	if p == nil {
		return m.flash("No process selected", true)
	}
	pid := p.PID
	if m.selectedPID == 0 {
		m.selectedPID = pid
	}
	m.detailsOpen = true
	m.details = nil
	m.detailsPID = pid
	m.detailsOffset = 0
	return func() tea.Msg {
		return detailsMsg{pid: pid, details: metrics.Details(pid)}
	}
}

func (m *Model) closeDetails() {
	m.detailsOpen = false
	m.details = nil
	m.detailsPID = 0
	m.detailsOffset = 0
}

func (m *Model) beginKill() tea.Cmd {
	p := m.selectedProc()
	if p == nil {
		return m.flash("No process selected", true)
	}
	if p.PID == int32(os.Getpid()) {
		return m.flash("Refusing to signal tachtop itself", true)
	}
	proc := *p
	if m.selectedPID == 0 {
		m.selectedPID = p.PID
	}
	m.kill = &proc
	return nil
}

func (m *Model) flash(text string, isErr bool) tea.Cmd {
	m.toast = text
	m.toastErr = isErr
	m.toastSeq++
	seq := m.toastSeq
	return tea.Tick(3500*time.Millisecond, func(time.Time) tea.Msg {
		return toastMsg{seq: seq}
	})
}
