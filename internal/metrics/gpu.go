package metrics

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// gpuCard is the static, slow-changing part of a detected DRM card.
type gpuCard struct {
	card   string // cardN, used to build paths below /sys/class/drm
	dev    string // /sys/class/drm/cardN/device
	vendor string // NVIDIA, AMD, Intel or Other
	driver string
	slot   string // normalised PCI slot, e.g. 0:01:00.0
	name   string
	hwmon  string
}

// nvidiaStat is one row from nvidia-smi.
type nvidiaStat struct {
	index    int
	slot     string
	name     string
	util     float64
	hasUtil  bool
	temp     float64
	hasTemp  bool
	core     float64
	maxCore  float64
	mem      float64
	maxMem   float64
	memUsed  uint64
	memTotal uint64
	power    float64
	limit    float64
}

// nvidiaQuery is deliberately conservative: every field used here is
// supported by drivers from the last several years. Unsupported values come
// back as "[N/A]" and are simply ignored.
const nvidiaQuery = "index,name,pci.bus_id,utilization.gpu,memory.used,memory.total," +
	"temperature.gpu,clocks.current.graphics,clocks.max.graphics," +
	"clocks.current.memory,clocks.max.memory,power.draw,power.limit"

// collectGPUs returns one entry per DRM card. NVIDIA data comes from
// nvidia-smi, AMD and Intel data from sysfs. Missing pieces are left at zero
// with the matching Has flag cleared.
func (c *Collector) collectGPUs(now time.Time) []GPU {
	cards := c.gpuCardsFor(now)
	if len(cards) == 0 {
		return nil
	}

	var nv []nvidiaStat
	for _, card := range cards {
		if card.vendor == "NVIDIA" {
			nv = c.nvidiaStats(now)
			break
		}
	}
	bySlot := make(map[string]nvidiaStat, len(nv))
	for _, n := range nv {
		bySlot[n.slot] = n
	}

	out := make([]GPU, 0, len(cards))
	for i, card := range cards {
		g := GPU{
			Index:   i,
			Name:    card.name,
			Vendor:  card.vendor,
			Driver:  card.driver,
			PCISlot: card.slot,
		}

		if n, ok := bySlot[card.slot]; ok {
			if n.name != "" {
				g.Name = n.name
			}
			g.UtilPercent, g.HasUtil = n.util, n.hasUtil
			g.Temp, g.HasTemp = n.temp, n.hasTemp
			g.CoreMHz, g.MaxCoreMHz = n.core, n.maxCore
			g.MemMHz, g.MaxMemMHz = n.mem, n.maxMem
			g.MemUsed, g.MemTotal = n.memUsed, n.memTotal
			g.Power, g.PowerLimit = n.power, n.limit
		}

		switch card.vendor {
		case "AMD":
			fillAMD(&g, card)
		case "Intel":
			fillIntel(&g, card)
		}
		if !g.HasTemp {
			g.Temp = hwmonTemp(card.hwmon)
			g.HasTemp = g.Temp > 0
		}
		if g.CoreMHz == 0 {
			g.CoreMHz, g.MaxCoreMHz = readClockPair(card)
		}
		out = append(out, g)
	}
	return out
}

// gpuCardsFor caches detection. GPUs are hot-pluggable (eGPU, driver reload),
// so the cache expires after a short while.
func (c *Collector) gpuCardsFor(now time.Time) []gpuCard {
	if c.gpuAt.IsZero() || now.Sub(c.gpuAt) > 30*time.Second {
		c.gpuCards = detectGPUs()
		c.gpuAt = now
	}
	return c.gpuCards
}

func detectGPUs() []gpuCard {
	matches, _ := filepath.Glob("/sys/class/drm/card[0-9]*")
	sort.Slice(matches, func(i, j int) bool {
		return cardNumber(matches[i]) < cardNumber(matches[j])
	})
	names := lspciNames()

	var cards []gpuCard
	for _, path := range matches {
		card := filepath.Base(path)
		if !isCardName(card) {
			continue
		}
		dev := filepath.Join(path, "device")
		if _, err := os.Stat(dev); err != nil {
			continue
		}

		vendorID := readTrim(filepath.Join(dev, "vendor"))
		driver := ""
		if target, err := filepath.EvalSymlinks(filepath.Join(dev, "driver")); err == nil {
			driver = filepath.Base(target)
		}
		slot := normalizeSlot(readUevent(dev, "PCI_SLOT_NAME"))
		if slot == "" {
			// Last resort: the symlink target ends in the PCI address.
			if target, err := filepath.EvalSymlinks(dev); err == nil {
				slot = normalizeSlot(filepath.Base(target))
			}
		}

		vendor := vendorName(vendorID)
		name := names[slot]
		if name == "" {
			name = vendor + " GPU"
		}
		cardInfo := gpuCard{
			card:   card,
			dev:    dev,
			vendor: vendor,
			driver: driver,
			slot:   slot,
			name:   name,
			hwmon:  findGPUHardwareMonitor(dev, path),
		}
		cards = append(cards, cardInfo)
	}
	return cards
}

func cardNumber(path string) int {
	base := filepath.Base(path)
	n, _ := strconv.Atoi(strings.TrimPrefix(base, "card"))
	return n
}

func isCardName(name string) bool {
	if !strings.HasPrefix(name, "card") {
		return false
	}
	rest := strings.TrimPrefix(name, "card")
	if rest == "" {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func vendorName(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "0x10de":
		return "NVIDIA"
	case "0x1002":
		return "AMD"
	case "0x8086":
		return "Intel"
	default:
		return "Other"
	}
}

// normalizeSlot turns 00000000:01:00.0 and 0000:01:00.0 into 0:01:00.0 so
// nvidia-smi and sysfs addresses can be compared.
func normalizeSlot(slot string) string {
	slot = strings.ToLower(strings.TrimSpace(slot))
	if slot == "" {
		return ""
	}
	if i := strings.IndexAny(slot, " \t/"); i >= 0 {
		slot = slot[:i]
	}
	if i := strings.IndexByte(slot, ':'); i > 0 {
		domain := strings.TrimLeft(slot[:i], "0")
		if domain == "" {
			domain = "0"
		}
		slot = domain + slot[i:]
	}
	return slot
}

// nvidiaStats caches nvidia-smi output briefly so a fast refresh interval
// does not spawn a process for every frame.
func (c *Collector) nvidiaStats(now time.Time) []nvidiaStat {
	if now.Sub(c.nvidiaAt) < 750*time.Millisecond {
		return c.nvidia
	}
	c.nvidiaAt = now
	c.nvidia = nil

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu="+nvidiaQuery, "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}
	c.nvidia = parseNvidiaCSV(string(out))
	return c.nvidia
}

func parseNvidiaCSV(out string) []nvidiaStat {
	var stats []nvidiaStat
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 13 {
			continue
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		idx, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		n := nvidiaStat{
			index:   idx,
			name:    cleanText(fields[1]),
			slot:    normalizeSlot(fields[2]),
			memUsed: 0,
		}
		n.util, n.hasUtil = parseOptional(fields[3])
		if mb, ok := parseOptional(fields[4]); ok {
			n.memUsed = uint64(mb * 1024 * 1024)
		}
		if mb, ok := parseOptional(fields[5]); ok {
			n.memTotal = uint64(mb * 1024 * 1024)
		}
		n.temp, n.hasTemp = parseOptional(fields[6])
		n.core, _ = parseOptional(fields[7])
		n.maxCore, _ = parseOptional(fields[8])
		n.mem, _ = parseOptional(fields[9])
		n.maxMem, _ = parseOptional(fields[10])
		n.power, _ = parseOptional(fields[11])
		n.limit, _ = parseOptional(fields[12])
		stats = append(stats, n)
	}
	return stats
}

func parseOptional(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "N/A") || strings.Contains(s, "N/A") ||
		strings.EqualFold(s, "Not Supported") {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func fillAMD(g *GPU, card gpuCard) {
	// lspci usually gives the marketing name. product_name is a good
	// fallback when lspci is not installed or has no entry for the card.
	genericName := g.Name == "" || strings.EqualFold(g.Name, "AMD GPU")
	if product := cleanText(readTrim(filepath.Join(card.dev, "product_name"))); product != "" &&
		!strings.EqualFold(product, "AMD GPU") && genericName {
		g.Name = product
	}
	if !g.HasUtil {
		if v, ok := readFloat(filepath.Join(card.dev, "gpu_busy_percent")); ok {
			g.UtilPercent, g.HasUtil = clamp(v, 0, 100), true
		}
	}
	if g.MemTotal == 0 {
		g.MemUsed, _ = readUint(filepath.Join(card.dev, "mem_info_vram_used"))
		g.MemTotal, _ = readUint(filepath.Join(card.dev, "mem_info_vram_total"))
	}
	if g.CoreMHz == 0 {
		g.CoreMHz, g.MaxCoreMHz = readPpDpm(filepath.Join(card.dev, "pp_dpm_sclk"))
	}
	if g.MemMHz == 0 {
		g.MemMHz, g.MaxMemMHz = readPpDpm(filepath.Join(card.dev, "pp_dpm_mclk"))
	}
	if !g.HasTemp {
		g.Temp = hwmonTemp(card.hwmon)
		g.HasTemp = g.Temp > 0
	}
	if g.Power == 0 {
		g.Power = hwmonPower(card.hwmon)
		g.PowerLimit = hwmonPowerLimit(card.hwmon)
	}
}

func fillIntel(g *GPU, card gpuCard) {
	if g.CoreMHz == 0 {
		g.CoreMHz, g.MaxCoreMHz = readClockPair(card)
	}
	if !g.HasTemp {
		g.Temp = hwmonTemp(card.hwmon)
		g.HasTemp = g.Temp > 0
	}
}

// readClockPair understands the three common Linux GPU frequency layouts:
// i915 gt files, i915 legacy files and xe tile/gt/freq files.
func readClockPair(card gpuCard) (float64, float64) {
	dirs, _ := filepath.Glob(filepath.Join(card.dev, "drm", "card*", "gt", "gt*"))
	xeDirs, _ := filepath.Glob(filepath.Join(card.dev, "tile*", "gt*", "freq0"))

	var cur, max float64
	for _, dir := range append(dirs, xeDirs...) {
		if v, ok := readFloat(filepath.Join(dir, "rps_cur_freq_mhz")); ok && v > cur {
			cur = v
		}
		if v, ok := readFloat(filepath.Join(dir, "rps_max_freq_mhz")); ok && v > max {
			max = v
		}
		if v, ok := readFloat(filepath.Join(dir, "cur_freq")); ok && v > cur {
			cur = v
		}
		if v, ok := readFloat(filepath.Join(dir, "max_freq")); ok && v > max {
			max = v
		}
		if v, ok := readFloat(filepath.Join(dir, "act_freq")); ok && v > cur {
			cur = v
		}
	}
	if cur == 0 {
		cur, _ = readFloat(filepath.Join(card.dev, "gt_cur_freq_mhz"))
	}
	if cur == 0 {
		cur, _ = readFloat(filepath.Join(card.dev, "gt_act_freq_mhz"))
	}
	if max == 0 {
		max, _ = readFloat(filepath.Join(card.dev, "gt_max_freq_mhz"))
	}
	if max == 0 {
		max = cur
	}
	return cur, max
}

// readPpDpm parses AMD power-play tables such as:
//
//	0: 500Mhz
//	1: 2100Mhz *
//	2: 2500Mhz
func readPpDpm(path string) (current, max float64) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		active := strings.HasSuffix(line, "*")
		line = strings.TrimSuffix(strings.TrimSpace(line), "*")
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		value := strings.TrimSpace(line[idx+1:])
		value = strings.TrimSuffix(strings.ToLower(value), "mhz")
		value = strings.TrimSpace(value)
		v, err := strconv.ParseFloat(value, 64)
		if err != nil || v <= 0 {
			continue
		}
		if v > max {
			max = v
		}
		if active {
			current = v
		}
	}
	if current == 0 {
		current = max
	}
	return current, max
}

// findGPUHardwareMonitor searches the PCI device and DRM card directories for
// a hwmon node. Some drivers expose sensors on the card rather than the PCI
// device, and not every node has a temperature channel.
func findGPUHardwareMonitor(paths ...string) string {
	var first string
	for _, path := range paths {
		matches, _ := filepath.Glob(filepath.Join(path, "hwmon", "hwmon*"))
		for _, dir := range matches {
			if first == "" {
				first = dir
			}
			if _, err := os.Stat(filepath.Join(dir, "temp1_input")); err == nil {
				return dir
			}
		}
	}
	return first
}

func hwmonTemp(dir string) float64 {
	if dir == "" {
		return 0
	}
	// temp1 is the edge sensor on most AMD cards; temp2 is junction and can
	// run much hotter. Prefer temp1, then fall back to the first readable one.
	if v, ok := readFloat(filepath.Join(dir, "temp1_input")); ok && v > 0 {
		return v / 1000
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
	for _, path := range matches {
		if v, ok := readFloat(path); ok && v > 0 && v/1000 < 150 {
			return v / 1000
		}
	}
	return 0
}

func hwmonPower(dir string) float64 {
	if dir == "" {
		return 0
	}
	if v, ok := readFloat(filepath.Join(dir, "power1_average")); ok && v > 0 {
		return v / 1_000_000
	}
	if v, ok := readFloat(filepath.Join(dir, "power1_input")); ok && v > 0 {
		return v / 1_000_000
	}
	return 0
}

func hwmonPowerLimit(dir string) float64 {
	if dir == "" {
		return 0
	}
	if v, ok := readFloat(filepath.Join(dir, "power1_cap")); ok && v > 0 {
		return v / 1_000_000
	}
	return 0
}

func readFloat(path string) (float64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func readUint(path string) (uint64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func readTrim(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readUevent(dir, key string) string {
	data, err := os.ReadFile(filepath.Join(dir, "uevent"))
	if err != nil {
		return ""
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// lspciNames maps PCI slots to readable device names. lspci is optional; when
// it is unavailable, generic names such as "AMD GPU" are used instead.
func lspciNames() map[string]string {
	out := make(map[string]string)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "lspci", "-D", "-mm").Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := quotedFields(line)
		if len(fields) < 3 {
			continue
		}
		slot := normalizeSlot(fields[0])
		vendor := ""
		device := ""
		if len(fields) >= 4 {
			vendor = shortVendor(fields[2])
			device = cleanText(fields[3])
		} else {
			device = cleanText(fields[1])
		}
		if device == "" {
			device = vendor + " GPU"
		} else if vendor != "" {
			device = vendor + " " + device
		}
		out[slot] = device
	}
	return out
}

// quotedFields returns the bare and quoted fields of an lspci -mm line.
func quotedFields(line string) []string {
	var fields []string
	for i := 0; i < len(line); {
		switch line[i] {
		case ' ', '\t':
			i++
		case '"':
			j := i + 1
			for j < len(line) && line[j] != '"' {
				j++
			}
			fields = append(fields, line[i+1:j])
			i = j + 1
		default:
			j := i
			for j < len(line) && line[j] != ' ' && line[j] != '\t' {
				j++
			}
			fields = append(fields, line[i:j])
			i = j
		}
	}
	return fields
}

func shortVendor(vendor string) string {
	switch {
	case strings.Contains(vendor, "NVIDIA"):
		return "NVIDIA"
	case strings.Contains(vendor, "AMD"), strings.Contains(vendor, "ATI"):
		return "AMD"
	case strings.Contains(vendor, "Intel"):
		return "Intel"
	default:
		return cleanText(vendor)
	}
}

// cleanText removes control characters from vendor-provided strings.
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
}
