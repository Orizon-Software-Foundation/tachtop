package metrics

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// procStat is the useful part of a /proc/<pid>/stat line.
type procStat struct {
	name       string
	state      string
	ppid       int32
	cpuSeconds float64
	threads    int32
	startTicks uint64
	vmsBytes   uint64
	rssPages   uint64
}

// readProcStat parses /proc/<pid>/stat. The second field is the executable
// name in parentheses and may itself contain spaces, so the closing
// parenthesis is found from the right before splitting the rest.
func readProcStat(pid int32) (procStat, error) {
	var ps procStat
	path := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return ps, err
	}
	line := string(data)

	open := strings.IndexByte(line, '(')
	close := strings.LastIndexByte(line, ')')
	if open < 0 || close <= open || close+2 > len(line) {
		return ps, fmt.Errorf("malformed %s", path)
	}
	ps.name = line[open+1 : close]

	fields := strings.Fields(line[close+1:])
	if len(fields) < 22 {
		return ps, fmt.Errorf("short %s: %d fields", path, len(fields))
	}
	ps.state = fields[0]
	ps.ppid = parseInt32(fields[1])
	utime := parseUint64(fields[11])
	stime := parseUint64(fields[12])
	ps.cpuSeconds = float64(utime+stime) / userHZ
	ps.threads = parseInt32(fields[17])
	ps.startTicks = parseUint64(fields[19])
	ps.vmsBytes = parseUint64(fields[20])
	ps.rssPages = parseUint64(fields[21])
	return ps, nil
}

func (c *Collector) collectProcs(now time.Time, memTotal uint64) ([]Proc, int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, 0, err
	}

	dt := now.Sub(c.lastAt).Seconds()
	procs := make([]Proc, 0, len(entries)/2)
	next := make(map[int32]procSample, len(entries)/2)
	count := 0

	for _, entry := range entries {
		pid64, err := strconv.ParseInt(entry.Name(), 10, 32)
		if err != nil {
			continue
		}
		pid := int32(pid64)
		ps, err := readProcStat(pid)
		if err != nil {
			continue // the process exited while we were looking at it
		}
		count++

		var usage float64
		if prev, ok := c.prevProcs[pid]; ok && prev.start == ps.startTicks && dt > 0 {
			if delta := ps.cpuSeconds - prev.cpu; delta > 0 {
				usage = 100 * delta / dt
			}
		}
		next[pid] = procSample{cpu: ps.cpuSeconds, start: ps.startTicks}

		rss := ps.rssPages * c.pageSize
		memPercent := 0.0
		if memTotal > 0 {
			memPercent = float64(rss) / float64(memTotal) * 100
		}
		created := int64(0)
		if c.bootTime > 0 {
			created = int64(c.bootTime) + int64(float64(ps.startTicks)/userHZ)
		}

		procs = append(procs, Proc{
			PID:        pid,
			PPID:       ps.ppid,
			Name:       ps.name,
			User:       c.userFor(pid),
			State:      ps.state,
			CPU:        usage,
			MemPercent: memPercent,
			RSS:        rss,
			VMS:        ps.vmsBytes,
			Threads:    ps.threads,
			Created:    created,
		})
	}

	c.prevProcs = next
	return procs, count, nil
}

func parseInt32(s string) int32 {
	v, _ := strconv.ParseInt(s, 10, 32)
	return int32(v)
}

func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

type procInfo struct {
	ppid    int32
	name    string
	user    string
	state   string
	exe     string
	cmdline string
	cwd     string
	threads int32
	created int64
	rss     uint64
	vms     uint64
	swap    uint64
}

// processByPID collects the slower, on-demand details for one process.
// Individual fields that are not available (common for kernel threads) are
// simply left at their zero value.
func processByPID(pid int32) (procInfo, error) {
	var out procInfo
	p, err := process.NewProcess(pid)
	if err != nil {
		return out, err
	}
	out.name, _ = p.Name()
	out.ppid, _ = p.Ppid()
	out.user, _ = p.Username()
	if status, err := p.Status(); err == nil {
		out.state = stateName(strings.Join(status, ","))
	}
	out.exe, _ = p.Exe()
	out.cmdline, _ = p.Cmdline()
	out.cwd, _ = p.Cwd()
	out.threads, _ = p.NumThreads()
	if ms, err := p.CreateTime(); err == nil {
		out.created = ms / 1000 // gopsutil reports milliseconds
	}
	if mi, err := p.MemoryInfo(); err == nil {
		out.rss = mi.RSS
		out.vms = mi.VMS
		out.swap = mi.Swap
	}
	return out, nil
}

func stateName(code string) string {
	if len(code) > 1 {
		return strings.ToLower(code)
	}
	switch code {
	case "R":
		return "running"
	case "S":
		return "sleeping"
	case "D":
		return "uninterruptible"
	case "Z":
		return "zombie"
	case "T":
		return "stopped"
	case "t":
		return "tracing stop"
	case "X", "x":
		return "dead"
	case "I":
		return "idle"
	case "W":
		return "paging"
	case "P":
		return "parked"
	default:
		if code == "" {
			return "unknown"
		}
		return code
	}
}
