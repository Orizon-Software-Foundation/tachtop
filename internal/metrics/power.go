package metrics

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultPowercapRoot = "/sys/class/powercap"
	defaultHwmonRoot    = "/sys/class/hwmon"
)

// energySample is one cumulative energy reading. max is the counter's wrap
// point in microjoules, or zero when the source does not report one.
type energySample struct {
	at  time.Time
	uj  uint64
	max uint64
}

// powerMeter turns cumulative energy counters into CPU package power. RAPL
// powercap is preferred because it reports the package domain directly;
// hwmon is a fallback for AMD drivers such as amd_energy and zenpower.
type powerMeter struct {
	powercapRoot string
	hwmonRoot    string
	samples      map[string]energySample
}

func newPowerMeter(powercapRoot, hwmonRoot string) powerMeter {
	return powerMeter{
		powercapRoot: powercapRoot,
		hwmonRoot:    hwmonRoot,
		samples:      make(map[string]energySample),
	}
}

func (p *powerMeter) reset() {
	p.samples = make(map[string]energySample)
}

// sample returns CPU package power in watts. has is false while the first
// energy baseline is being established or when no readable sensor exists.
func (p *powerMeter) sample(now time.Time) (watts float64, has bool) {
	if p.samples == nil {
		p.samples = make(map[string]energySample)
	}
	if p.powercapRoot == "" && p.hwmonRoot == "" {
		return 0, false
	}

	watts, has, readable := p.raplPower(now)
	if readable {
		return watts, has
	}
	return p.hwmonPower(now)
}

type energyCounter struct {
	key  string
	path string
	max  uint64
}

// raplPower reads the RAPL package domains. readable reports whether at least
// one energy counter could be read, so an unreadable powercap (common without
// root) can fall back to hwmon.
func (p *powerMeter) raplPower(now time.Time) (watts float64, has, readable bool) {
	for _, counter := range p.raplCounters() {
		uj, ok := readUint(counter.path)
		if !ok {
			continue
		}
		readable = true

		prev, hadPrev := p.samples[counter.key]
		max := counter.max
		if max == 0 {
			max = prev.max
		}
		p.samples[counter.key] = energySample{at: now, uj: uj, max: max}
		if !hadPrev {
			continue
		}

		w, ok := energyWatts(prev, uj, max, now)
		if ok {
			watts += w
			has = true
		}
	}
	return watts, has, readable
}

// raplCounters finds one package domain per name. Both the MSR and MMIO
// powercap trees can expose the same package, so duplicate names are merged
// to avoid counting the same energy twice.
func (p *powerMeter) raplCounters() []energyCounter {
	if p.powercapRoot == "" {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join(p.powercapRoot, "*"))
	sort.Strings(matches)

	chosen := make(map[string]string)
	for _, dir := range matches {
		name := readTrim(filepath.Join(dir, "name"))
		if !strings.HasPrefix(name, "package-") {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "energy_uj")); err != nil {
			continue
		}
		if prev, ok := chosen[name]; !ok || preferEnergySource(dir, prev) {
			chosen[name] = dir
		}
	}

	out := make([]energyCounter, 0, len(chosen))
	for name, dir := range chosen {
		max, _ := readUint(filepath.Join(dir, "max_energy_range_uj"))
		out = append(out, energyCounter{
			key:  "rapl:" + name,
			path: filepath.Join(dir, "energy_uj"),
			max:  max,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

// preferEnergySource chooses the non-MMIO duplicate when both trees expose the
// same package. MMIO and MSR copies contain identical counters, so either is
// correct; preferring the MSR tree matches tools such as turbostat.
func preferEnergySource(a, b string) bool {
	aMMIO := strings.Contains(filepath.Base(a), "-mmio")
	bMMIO := strings.Contains(filepath.Base(b), "-mmio")
	if aMMIO != bMMIO {
		return !aMMIO
	}
	return a < b
}

// hwmonPower handles CPU power sensors that are not exposed through powercap.
// Drivers such as zenpower report an instantaneous power1_input in microwatts;
// amd_energy exposes cumulative energyN_input counters in microjoules.
func (p *powerMeter) hwmonPower(now time.Time) (float64, bool) {
	if p.hwmonRoot == "" {
		return 0, false
	}
	matches, _ := filepath.Glob(filepath.Join(p.hwmonRoot, "hwmon*"))
	sort.Strings(matches)

	var watts float64
	var has bool
	for _, dir := range matches {
		name := readTrim(filepath.Join(dir, "name"))
		if !isCPUHwmon(name) {
			continue
		}

		if v, ok := firstFloat(dir, "power1_input", "power1_average"); ok {
			if v < 0 {
				v = 0
			}
			watts += v / 1_000_000
			has = true
			continue
		}

		energyFiles, _ := filepath.Glob(filepath.Join(dir, "energy*_input"))
		sort.Strings(energyFiles)
		for _, path := range energyFiles {
			uj, ok := readUint(path)
			if !ok {
				continue
			}
			key := "hwmon:" + filepath.Base(dir) + ":" + filepath.Base(path)
			prev, hadPrev := p.samples[key]
			p.samples[key] = energySample{at: now, uj: uj}
			if !hadPrev {
				continue
			}
			w, ok := energyWatts(prev, uj, 0, now)
			if ok {
				watts += w
				has = true
			}
		}
	}
	return watts, has
}

// energyWatts converts two counter readings into average power. Without a
// reported wrap point, a decrease is treated as a counter reset and ignored.
func energyWatts(prev energySample, current, max uint64, now time.Time) (float64, bool) {
	dt := now.Sub(prev.at).Seconds()
	if dt <= 0 {
		return 0, false
	}

	delta := current - prev.uj
	if current < prev.uj {
		if max == 0 || max < prev.uj {
			return 0, false
		}
		delta = max - prev.uj + current
	}
	return float64(delta) / 1_000_000 / dt, true
}

// isCPUHwmon keeps GPU and motherboard power sensors out of the CPU panel.
func isCPUHwmon(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, cpuName := range []string{
		"amd_energy", "zenpower", "fam15h_power", "k10temp", "coretemp", "cpu",
	} {
		if name == cpuName {
			return true
		}
	}
	return false
}

func firstFloat(dir string, names ...string) (float64, bool) {
	for _, name := range names {
		if v, ok := readFloat(filepath.Join(dir, name)); ok {
			return v, true
		}
	}
	return 0, false
}
