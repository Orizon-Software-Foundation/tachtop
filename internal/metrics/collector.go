package metrics

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

const userHZ = 100.0 // Linux exposes CPU times in USER_HZ (always 100).

type netSample struct {
	rx, tx uint64
}

type procSample struct {
	cpu   float64
	start uint64
}

// Collector keeps the previous counter values needed to turn cumulative
// kernel counters into per-second rates. It is not safe for concurrent use;
// tachtop calls it from a single background command at a time.
type Collector struct {
	hostname string
	platform string
	kernel   string
	bootTime uint64
	uptime   uint64
	cpuModel string
	cpuCores int
	baseMHz  float64
	numCPU   int
	pageSize uint64

	lastAt        time.Time
	prevDiskRead  uint64
	prevDiskWrite uint64
	prevNet       map[string]netSample
	prevProcs     map[int32]procSample

	phys   map[string]bool
	physAt time.Time

	temp   float64
	tempAt time.Time
	mhz    float64
	mhzAt  time.Time

	gpuCards []gpuCard
	gpuAt    time.Time
	nvidia   []nvidiaStat
	nvidiaAt time.Time

	users map[uint32]string

	primed bool
}

// New creates a collector and primes the cumulative counters so the first
// visible snapshot contains sensible rates instead of numbers measured since
// boot.
func New() *Collector {
	c := &Collector{
		numCPU:    runtime.NumCPU(),
		pageSize:  uint64(os.Getpagesize()),
		prevNet:   make(map[string]netSample),
		prevProcs: make(map[int32]procSample),
		users:     make(map[uint32]string),
	}

	if info, err := host.Info(); err == nil {
		c.hostname = info.Hostname
		c.platform = strings.TrimSpace(info.Platform)
		if info.PlatformVersion != "" {
			c.platform += " " + strings.TrimSpace(info.PlatformVersion)
		}
		c.kernel = info.KernelVersion
		c.bootTime = info.BootTime
	}
	if c.hostname == "" {
		c.hostname, _ = os.Hostname()
	}
	if n, err := cpu.Counts(true); err == nil && n > 0 {
		c.cpuCores = n
	} else {
		c.cpuCores = c.numCPU
	}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		c.cpuModel = infos[0].ModelName
		c.baseMHz = infos[0].Mhz
	}
	c.primed = true
	c.Collect()
	return c
}

// Reset clears rate baselines. Calling this after a pause prevents the first
// frame from showing an average over the whole paused period.
func (c *Collector) Reset() {
	c.lastAt = time.Time{}
	c.prevDiskRead = 0
	c.prevDiskWrite = 0
	c.prevNet = make(map[string]netSample)
	c.prevProcs = make(map[int32]procSample)
}

// Collect takes one snapshot. Errors are reported in Snapshot.Err; a partial
// snapshot is still returned.
func (c *Collector) Collect() Snapshot {
	now := time.Now()
	first := c.lastAt.IsZero()
	dt := now.Sub(c.lastAt).Seconds()
	if dt <= 0 {
		dt = 1
	}

	s := Snapshot{
		At:       now,
		Hostname: c.hostname,
		Platform: c.platform,
		Kernel:   c.kernel,
		CPUModel: c.cpuModel,
		CPUCores: c.cpuCores,
	}
	var errs []string

	if up, err := host.Uptime(); err == nil {
		c.uptime = up
	}
	s.Uptime = c.uptime
	if avg, err := load.Avg(); err == nil {
		s.Load = [3]float64{avg.Load1, avg.Load5, avg.Load15}
	}

	// CPU usage. gopsutil keeps its own previous counters per call pattern,
	// so asking for overall and per-core values in that order stays accurate.
	if p, err := cpu.Percent(0, false); err != nil {
		errs = append(errs, "cpu: "+err.Error())
	} else if len(p) > 0 {
		s.CPUPercent = clamp(p[0], 0, 100)
	}
	if p, err := cpu.Percent(0, true); err == nil {
		s.CorePercents = make([]float64, len(p))
		for i, v := range p {
			s.CorePercents[i] = clamp(v, 0, 100)
		}
	}

	if vm, err := mem.VirtualMemory(); err != nil {
		errs = append(errs, "memory: "+err.Error())
	} else {
		s.MemTotal = vm.Total
		s.MemUsed = vm.Used
		s.MemAvailable = vm.Available
		s.MemFree = vm.Free
		s.MemCached = vm.Cached
		s.MemBuffers = vm.Buffers
	}
	if sw, err := mem.SwapMemory(); err == nil {
		s.SwapTotal = sw.Total
		s.SwapUsed = sw.Used
	}
	if s.MemAvailable == 0 {
		s.MemAvailable = s.MemFree
	}

	s.CPUTemp = c.temperature(now)
	s.CPUMHz = c.frequency(now)
	s.Networks, s.NetRx, s.NetTx = c.collectNet(dt, first)
	s.Disks = c.collectDisks()
	s.DiskRead, s.DiskWrite = c.collectDiskIO(dt, first)
	s.GPUs = c.collectGPUs(now)

	procs, count, err := c.collectProcs(now, s.MemTotal)
	if err != nil {
		errs = append(errs, "processes: "+err.Error())
	}
	s.Procs = procs
	s.ProcCount = count

	s.Err = strings.Join(errs, "; ")
	c.lastAt = now
	return s
}

func (c *Collector) collectNet(dt float64, first bool) ([]Network, float64, float64) {
	counters, err := gnet.IOCounters(true)
	if err != nil {
		return nil, 0, 0
	}

	next := make(map[string]netSample, len(counters))
	out := make([]Network, 0, len(counters))
	var totalRx, totalTx float64

	for _, n := range counters {
		// Loopback traffic is local and would double-count ordinary usage.
		if n.Name == "lo" {
			continue
		}
		next[n.Name] = netSample{rx: n.BytesRecv, tx: n.BytesSent}
		if n.BytesRecv == 0 && n.BytesSent == 0 {
			continue
		}

		var rx, tx float64
		if prev, ok := c.prevNet[n.Name]; ok && !first && dt > 0 {
			if n.BytesRecv >= prev.rx {
				rx = float64(n.BytesRecv-prev.rx) / dt
			}
			if n.BytesSent >= prev.tx {
				tx = float64(n.BytesSent-prev.tx) / dt
			}
		}
		totalRx += rx
		totalTx += tx
		out = append(out, Network{
			Name:    n.Name,
			Rx:      rx,
			Tx:      tx,
			RxTotal: n.BytesRecv,
			TxTotal: n.BytesSent,
		})
	}
	c.prevNet = next

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Rx+out[i].Tx > out[j].Rx+out[j].Tx
	})
	return out, totalRx, totalTx
}

var pseudoFilesystems = map[string]bool{
	"autofs": true, "binfmt_misc": true, "bpf": true, "cgroup": true,
	"cgroup2": true, "configfs": true, "debugfs": true, "devfs": true,
	"devpts": true, "devtmpfs": true, "fusectl": true, "hugetlbfs": true,
	"iso9660": true, "mqueue": true, "nsfs": true, "overlay": true,
	"proc": true, "pstore": true, "ramfs": true, "securityfs": true,
	"squashfs": true, "sysfs": true, "tmpfs": true, "tracefs": true,
}

func (c *Collector) collectDisks() []Disk {
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil
	}

	// Bind mounts and btrfs subvolumes share a device and therefore report the
	// same usage. Keep the most useful mountpoint for each device.
	chosen := make(map[string]disk.PartitionStat)
	for _, part := range parts {
		if part.Mountpoint == "" || pseudoFilesystems[strings.ToLower(part.Fstype)] ||
			strings.HasPrefix(part.Mountpoint, "/snap/") ||
			strings.HasPrefix(part.Mountpoint, "/var/lib/docker/") ||
			strings.HasPrefix(part.Mountpoint, "/var/lib/containers/") {
			continue
		}
		key := part.Device
		if key == "" {
			key = part.Mountpoint
		}
		if prev, ok := chosen[key]; !ok || preferMount(part.Mountpoint, prev.Mountpoint) {
			chosen[key] = part
		}
	}

	out := make([]Disk, 0, len(chosen))
	for _, part := range chosen {
		usage, err := disk.Usage(part.Mountpoint)
		if err != nil || usage.Total == 0 {
			continue
		}
		out = append(out, Disk{
			Mount:       part.Mountpoint,
			Device:      part.Device,
			FSType:      part.Fstype,
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: clamp(usage.UsedPercent, 0, 100),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Mount == "/" {
			return true
		}
		if out[j].Mount == "/" {
			return false
		}
		return out[i].Mount < out[j].Mount
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func (c *Collector) collectDiskIO(dt float64, first bool) (float64, float64) {
	counters, err := disk.IOCounters()
	if err != nil {
		return 0, 0
	}
	physical := c.physicalDisks()

	var read, written uint64
	for name, io := range counters {
		if len(physical) > 0 {
			if !physical[name] {
				continue
			}
		} else if skipBlockDevice(name) {
			continue
		}
		read += io.ReadBytes
		written += io.WriteBytes
	}

	defer func() {
		c.prevDiskRead = read
		c.prevDiskWrite = written
	}()
	if first || dt <= 0 || read < c.prevDiskRead || written < c.prevDiskWrite {
		return 0, 0
	}
	return float64(read-c.prevDiskRead) / dt, float64(written-c.prevDiskWrite) / dt
}

func (c *Collector) physicalDisks() map[string]bool {
	now := time.Now()
	if c.phys != nil && now.Sub(c.physAt) < 30*time.Second {
		return c.phys
	}
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil
	}
	phys := make(map[string]bool, len(entries))
	for _, e := range entries {
		name := e.Name()
		if skipBlockDevice(name) || strings.HasPrefix(name, "dm-") || strings.HasPrefix(name, "md") {
			continue
		}
		phys[name] = true
	}
	c.phys = phys
	c.physAt = now
	return phys
}

// preferMount chooses between two mountpoints on the same device.
func preferMount(a, b string) bool {
	if a == "/" {
		return true
	}
	if b == "/" {
		return false
	}
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func skipBlockDevice(name string) bool {
	for _, prefix := range []string{"loop", "ram", "zram", "sr", "fd"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (c *Collector) temperature(now time.Time) float64 {
	if now.Sub(c.tempAt) < 5*time.Second {
		return c.temp
	}
	c.tempAt = now
	c.temp = 0

	values, err := sensors.SensorsTemperatures()
	if err != nil {
		return 0
	}
	bestRank := 0
	for _, v := range values {
		if v.Temperature <= 0 || v.Temperature > 150 {
			continue
		}
		rank := sensorRank(v.SensorKey)
		if rank == 0 {
			continue
		}
		if rank > bestRank || (rank == bestRank && v.Temperature > c.temp) {
			bestRank = rank
			c.temp = v.Temperature
		}
	}
	return c.temp
}

func sensorRank(key string) int {
	k := strings.ToLower(key)
	switch {
	case strings.Contains(k, "package"):
		return 4
	case strings.Contains(k, "tctl"), strings.Contains(k, "tdie"):
		return 4
	case strings.Contains(k, "coretemp"), strings.Contains(k, "k10temp"):
		return 3
	case strings.Contains(k, "cpu"):
		return 2
	case strings.Contains(k, "soc"), strings.Contains(k, "acpitz"):
		return 1
	default:
		return 0
	}
}

func (c *Collector) frequency(now time.Time) float64 {
	if now.Sub(c.mhzAt) < 5*time.Second {
		return c.mhz
	}
	c.mhzAt = now
	c.mhz = c.baseMHz

	paths, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")
	var sum float64
	var n int
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		khz, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil || khz <= 0 {
			continue
		}
		sum += khz / 1000 // kHz -> MHz
		n++
	}
	if n > 0 {
		c.mhz = sum / float64(n)
	}
	return c.mhz
}

func (c *Collector) userFor(pid int32) string {
	info, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	if err != nil {
		return "?"
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "?"
	}
	if name, ok := c.users[stat.Uid]; ok {
		return name
	}
	name := "?"
	if u, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10)); err == nil {
		name = u.Username
	}
	c.users[stat.Uid] = name
	return name
}

// Details gathers extra information for the details dialog. Missing fields
// are left empty; kernel threads legitimately have no executable or cmdline.
func Details(pid int32) ProcDetails {
	d := ProcDetails{PID: pid}
	p, err := processByPID(pid)
	if err != nil {
		d.Warnings = append(d.Warnings, err.Error())
		return d
	}
	d.PPID = p.ppid
	d.Name = p.name
	d.User = p.user
	d.Status = p.state
	d.Exe = p.exe
	d.Cmdline = p.cmdline
	d.Cwd = p.cwd
	d.Threads = p.threads
	d.Created = p.created
	d.RSS = p.rss
	d.VMS = p.vms
	d.Swap = p.swap
	return d
}

// Signal sends SIGTERM, or SIGKILL when force is true.
func Signal(pid int32, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	return syscall.Kill(int(pid), sig)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
