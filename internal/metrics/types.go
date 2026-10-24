// Package metrics collects live system statistics from Linux's /proc and
// /sys filesystems. It is intentionally read-only and needs no daemon.
package metrics

import "time"

// Snapshot is one complete sample of the system, ready for rendering.
type Snapshot struct {
	At time.Time

	Hostname string
	Platform string
	Kernel   string
	Uptime   uint64
	Load     [3]float64

	CPUPercent   float64
	CorePercents []float64
	CPUModel     string
	CPUCores     int
	CPUMHz       float64
	CPUTemp      float64

	MemTotal     uint64
	MemUsed      uint64
	MemAvailable uint64
	MemFree      uint64
	MemCached    uint64
	MemBuffers   uint64
	SwapTotal    uint64
	SwapUsed     uint64

	NetRx     float64
	NetTx     float64
	Networks  []Network
	DiskRead  float64
	DiskWrite float64
	Disks     []Disk

	GPUs []GPU

	Procs     []Proc
	ProcCount int

	// Err contains a human-readable warning for a partial snapshot.
	// tachtop keeps running when this is set.
	Err string
}

// Network holds throughput for one interface.
type Network struct {
	Name    string
	Rx      float64
	Tx      float64
	RxTotal uint64
	TxTotal uint64
}

// Disk holds usage for one mounted filesystem.
type Disk struct {
	Mount       string
	Device      string
	FSType      string
	Total       uint64
	Used        uint64
	Free        uint64
	UsedPercent float64
}

// GPU is one graphics adapter discovered through DRM. Linux does not expose
// every statistic for every vendor, so the Has* flags distinguish "zero" from
// "not available".
type GPU struct {
	Index   int
	Name    string
	Vendor  string
	Driver  string
	PCISlot string

	UtilPercent float64
	HasUtil     bool

	Temp    float64
	HasTemp bool

	CoreMHz    float64
	MaxCoreMHz float64
	MemMHz     float64
	MaxMemMHz  float64

	MemUsed  uint64
	MemTotal uint64

	Power      float64
	PowerLimit float64
}

// Proc is the subset of process information shown in the process table.
type Proc struct {
	PID        int32
	PPID       int32
	Name       string
	User       string
	State      string
	CPU        float64
	MemPercent float64
	RSS        uint64
	VMS        uint64
	Threads    int32
	Created    int64
}

// ProcDetails is collected on demand when the user opens the details view.
type ProcDetails struct {
	PID      int32
	PPID     int32
	Name     string
	User     string
	Status   string
	Exe      string
	Cmdline  string
	Cwd      string
	Threads  int32
	Created  int64
	RSS      uint64
	VMS      uint64
	Swap     uint64
	Warnings []string
}
