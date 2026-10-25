package metrics

import (
	"fmt"
	"math"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
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
// kernel counters into per-second rates. Collect and Reset are serialized by
// an internal mutex, so the UI can reset while a snapshot is still in flight.
type Collector struct {
	mu           sync.Mutex
	staticLoaded bool

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
	cpuValid      bool

	phys   map[string]bool
	physAt time.Time

	temp   float64
	tempAt time.Time
	mhz    float64
	mhzAt  time.Time

	power powerMeter

	gpuCards   []gpuCard
	gpuAt      time.Time
	nvidia     []nvidiaStat
	nvidiaAt   time.Time
	nvidiaFail bool

	users map[uint32]string
}

// New creates a collector and reads the static host and CPU information. The
// first Collect establishes the rate baselines and runs the expensive probes;
// doing that asynchronously keeps startup fast.
func New() *Collector {
	c := &Collector{}
	c.mu.Lock()
	c.initStatic()
	c.mu.Unlock()

	// Prime gopsutil's internal CPU baselines. Its first call always reports
	// an error because there is no previous sample; the first visible Collect
	// discards that near-zero measurement and then uses this baseline.
	_, _ = cpu.Percent(0, false)
	_, _ = cpu.Percent(0, true)
	return c
}

// initStatic fills fields that never change for the lifetime of the process.
// It is called with c.mu held.
func (c *Collector) initStatic() {
	if c.staticLoaded {
		return
	}
	c.staticLoaded = true

	if c.numCPU == 0 {
		c.numCPU = runtime.NumCPU()
	}
	if c.pageSize == 0 {
		c.pageSize = uint64(os.Getpagesize())
	}
	if c.prevNet == nil {
		c.prevNet = make(map[string]netSample)
	}
	if c.prevProcs == nil {
		c.prevProcs = make(map[int32]procSample)
	}
	if c.users == nil {
		c.users = make(map[uint32]string)
	}
	if c.power.powercapRoot == "" {
		c.power = newPowerMeter(defaultPowercapRoot, defaultHwmonRoot)
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
}

// Reset clears rate baselines. Calling this after a pause prevents the first
// frame from showing an average over the whole paused period.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()
}

// resetLocked clears the rate baselines. c.mu must be held.
func (c *Collector) resetLocked() {
	c.lastAt = time.Time{}
	c.prevDiskRead = 0
	c.prevDiskWrite = 0
	c.prevNet = make(map[string]netSample)
	c.prevProcs = make(map[int32]procSample)
	c.cpuValid = false
	c.tempAt = time.Time{}
	c.mhzAt = time.Time{}
	c.power.reset()
}

// Collect takes one snapshot. Errors are reported in Snapshot.Err; a partial
// snapshot is still returned.
func (c *Collector) Collect() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.collectLocked()
}

// CollectAfterReset clears the baselines and takes a snapshot as one atomic
// operation, so a concurrent collection cannot slip between the two steps.
func (c *Collector) CollectAfterReset() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked()
	return c.collectLocked()
}

// collectLocked takes one snapshot. c.mu must be held.
func (c *Collector) collectLocked() Snapshot {
	c.initStatic()

	now := time.Now()
	first := c.lastAt.IsZero()
	dt := now.Sub(c.lastAt).Seconds()
	if first || dt <= 0 {
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
	// The first sample after New or Reset is discarded: it only covers the few
	// milliseconds since the baseline was taken, and reporting it would show a
	// meaningless spike (or an average over the pause).
	cpuP, cpuErr := cpu.Percent(0, false)
	coreP, coreErr := cpu.Percent(0, true)
	if c.cpuValid {
		if cpuErr != nil {
			errs = append(errs, "cpu: "+cpuErr.Error())
		} else if len(cpuP) > 0 {
			s.CPUPercent = clamp(cpuP[0], 0, 100)
		}
		if coreErr != nil {
			errs = append(errs, "cpu cores: "+coreErr.Error())
		} else {
			s.CorePercents = make([]float64, len(coreP))
			for i, v := range coreP {
				s.CorePercents[i] = clamp(v, 0, 100)
			}
		}
	}
	c.cpuValid = true

	if vm, err := mem.VirtualMemory(); err != nil {
		errs = append(errs, "memory: "+err.Error())
	} else {
		s.MemTotal = vm.Total
		s.MemUsed = vm.Used
		s.MemAvailable = vm.Available
		s.MemFree = vm.Free
		s.MemCached = vm.Cached
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
	s.CPUPower, s.HasCPUPower = c.power.sample(now)
	s.Networks, s.NetRx, s.NetTx = c.collectNet(dt, first)
	s.Disks = c.collectDisks()
	s.DiskRead, s.DiskWrite = c.collectDiskIO(dt, first)
	s.GPUs = c.collectGPUs(now)
	s.Connections = Connections()

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
	rates := make(map[string][2]float64, len(counters))

	for _, n := range counters {
		// Loopback traffic is local and would double-count ordinary usage.
		if n.Name == "lo" {
			continue
		}
		next[n.Name] = netSample{rx: n.BytesRecv, tx: n.BytesSent}

		var rx, tx float64
		if prev, ok := c.prevNet[n.Name]; ok && !first && dt > 0 {
			if n.BytesRecv >= prev.rx {
				rx = float64(n.BytesRecv-prev.rx) / dt
			}
			if n.BytesSent >= prev.tx {
				tx = float64(n.BytesSent-prev.tx) / dt
			}
		}
		rates[n.Name] = [2]float64{rx, tx}

		iface := Network{
			Name:        n.Name,
			Rx:          rx,
			Tx:          tx,
			BytesRecv:   n.BytesRecv,
			BytesSent:   n.BytesSent,
			PacketsRecv: n.PacketsRecv,
			PacketsSent: n.PacketsSent,
			ErrsIn:      n.Errin,
			ErrsOut:     n.Errout,
			DropsIn:     n.Dropin,
			DropsOut:    n.Dropout,
		}
		if inf, err := net.InterfaceByName(n.Name); err == nil {
			iface.MTU = inf.MTU
			iface.Flags = interfaceFlags(inf.Flags)
		}
		out = append(out, iface)
	}
	c.prevNet = next

	// The headline total follows the default route. Summing every interface
	// counts the same bytes twice on bridges, bonds, VPNs and container
	// veth pairs; following the route the kernel actually uses avoids that.
	defaults := defaultRouteInterfaces()
	var totalRx, totalTx float64
	counted := false
	for name := range defaults {
		if r, ok := rates[name]; ok {
			totalRx += r[0]
			totalTx += r[1]
			counted = true
		}
	}
	if !counted {
		for _, n := range out {
			totalRx += n.Rx
			totalTx += n.Tx
		}
	}

	// A stable order stops the interface rows from jumping every refresh.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, totalRx, totalTx
}

// interfaceFlags converts net.Flags to a comma-separated list of names.
func interfaceFlags(f net.Flags) string {
	var parts []string
	if f&net.FlagUp != 0 {
		parts = append(parts, "UP")
	}
	if f&net.FlagBroadcast != 0 {
		parts = append(parts, "BROADCAST")
	}
	if f&net.FlagLoopback != 0 {
		parts = append(parts, "LOOPBACK")
	}
	if f&net.FlagPointToPoint != 0 {
		parts = append(parts, "POINTTOPOINT")
	}
	if f&net.FlagMulticast != 0 {
		parts = append(parts, "MULTICAST")
	}
	if f&net.FlagRunning != 0 {
		parts = append(parts, "RUNNING")
	}
	return strings.Join(parts, ",")
}

// defaultRouteInterfaces returns the interfaces that carry a default route.
// /proc/net/route lists destinations in little-endian hex; 00000000 is the
// default route. Multiple names are possible with policy routing.
func defaultRouteInterfaces() map[string]bool {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil
	}
	return parseDefaultRoutes(string(data))
}

func parseDefaultRoutes(data string) map[string]bool {
	out := make(map[string]bool)
	for i, line := range strings.Split(data, "\n") {
		if i == 0 {
			continue // header
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != "00000000" {
			continue
		}
		out[fields[0]] = true
	}
	return out
}

var pseudoFilesystems = map[string]bool{
	"autofs": true, "binfmt_misc": true, "bpf": true, "cgroup": true,
	"cgroup2": true, "configfs": true, "debugfs": true, "devfs": true,
	"devpts": true, "devtmpfs": true, "efivarfs": true, "fusectl": true,
	"hugetlbfs": true, "iso9660": true, "mqueue": true, "nsfs": true,
	"proc": true, "pstore": true, "ramfs": true, "securityfs": true,
	"squashfs": true, "sysfs": true, "tmpfs": true, "tracefs": true,
}

// isPseudoFilesystem reports whether a filesystem should stay out of the disk
// panel. Network filesystems are excluded as well: statfs on them can block
// the whole collection.
func isPseudoFilesystem(fstype string) bool {
	fs := strings.ToLower(strings.TrimSpace(fstype))
	if pseudoFilesystems[fs] {
		return true
	}
	for _, prefix := range []string{
		"fuse.", "nfs", "cifs", "9p", "ceph", "rpc_pipefs", "selinuxfs", "binder",
	} {
		if strings.HasPrefix(fs, prefix) {
			return true
		}
	}
	return false
}

func (c *Collector) collectDisks() []Disk {
	parts, err := disk.Partitions(false)
	if err == nil {
		if out := c.disksFromPartitions(parts); len(out) > 0 {
			return out
		}
	}
	// Containers often only expose an overlay root, which gopsutil's
	// physical-only mode skips. Retry with every mount and filter pseudo
	// filesystems and bind-mounted files in disksFromPartitions.
	parts, err = disk.Partitions(true)
	if err != nil {
		return nil
	}
	return c.disksFromPartitions(parts)
}

func (c *Collector) disksFromPartitions(parts []disk.PartitionStat) []Disk {
	// Bind mounts and btrfs subvolumes share a device and therefore report the
	// same usage. Keep the most useful mountpoint for each device.
	chosen := make(map[string]disk.PartitionStat)
	for _, part := range parts {
		if part.Mountpoint == "" || isPseudoFilesystem(part.Fstype) ||
			strings.HasPrefix(part.Mountpoint, "/snap/") ||
			strings.HasPrefix(part.Mountpoint, "/var/lib/docker/") ||
			strings.HasPrefix(part.Mountpoint, "/var/lib/containers/") {
			continue
		}
		// In containers /etc/hosts, /etc/hostname and /etc/resolv.conf are
		// bind-mounted files. They would report the host filesystem's usage.
		if info, err := os.Stat(part.Mountpoint); err != nil || !info.IsDir() {
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
			Total:       usage.Total,
			Used:        usage.Used,
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
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	return syscall.Kill(int(pid), sig)
}

// SignalChecked sends a signal only if the process still has the expected
// start time. This closes the window between showing the confirmation prompt
// and delivering the signal, during which the PID could have been reused.
func (c *Collector) SignalChecked(pid int32, expectedStart int64, force bool) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	if expectedStart > 0 {
		ps, err := readProcStat(pid)
		if err != nil {
			return fmt.Errorf("process %d is no longer running", pid)
		}
		if boot, err := host.BootTime(); err == nil && boot > 0 {
			started := int64(boot) + int64(float64(ps.startTicks)/userHZ)
			if started != expectedStart {
				return fmt.Errorf("process %d was replaced", pid)
			}
		}
	}
	return Signal(pid, force)
}

func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
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
