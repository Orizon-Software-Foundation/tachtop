package metrics

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePowercapDomain(t *testing.T, dir, name string, energy, max uint64) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "name"), name+"\n")
	writeTestFile(t, filepath.Join(dir, "energy_uj"), strconv.FormatUint(energy, 10)+"\n")
	writeTestFile(t, filepath.Join(dir, "max_energy_range_uj"), strconv.FormatUint(max, 10)+"\n")
}

func TestPowerMeterRAPL(t *testing.T) {
	root := t.TempDir()
	powercap := filepath.Join(root, "powercap")
	hwmon := filepath.Join(root, "hwmon")

	writePowercapDomain(t, filepath.Join(powercap, "intel-rapl:0"), "package-0", 1_000_000, 100_000_000)
	// The MMIO tree mirrors the same package. If both were counted, or if the
	// MMIO copy were chosen, the first interval would show the wrong value.
	writePowercapDomain(t, filepath.Join(powercap, "intel-rapl-mmio:0"), "package-0", 9_000_000, 100_000_000)
	// Subdomains are part of the package and must not be added again.
	writePowercapDomain(t, filepath.Join(powercap, "intel-rapl:0:0"), "core", 1_000_000, 100_000_000)
	writePowercapDomain(t, filepath.Join(powercap, "intel-rapl:0:1"), "uncore", 1_000_000, 100_000_000)

	meter := newPowerMeter(powercap, hwmon)
	t0 := time.Unix(1_700_000_000, 0)
	if watts, ok := meter.sample(t0); ok || watts != 0 {
		t.Fatalf("first sample = %vW, %v; want no baseline", watts, ok)
	}

	// The package counter advanced by 4 J over two seconds: 2 W.
	writeTestFile(t, filepath.Join(powercap, "intel-rapl:0", "energy_uj"), "5000000\n")
	writeTestFile(t, filepath.Join(powercap, "intel-rapl:0:0", "energy_uj"), "99000000\n")
	watts, ok := meter.sample(t0.Add(2 * time.Second))
	if !ok {
		t.Fatal("second sample has no reading")
	}
	if diff := watts - 2.0; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("package power = %vW, want 2W", watts)
	}

	// RAPL's package counter wraps at max_energy_range_uj. A decrease must
	// still produce a sensible average rather than a negative or zero value.
	writeTestFile(t, filepath.Join(powercap, "intel-rapl:0", "energy_uj"), "1000000\n")
	watts, ok = meter.sample(t0.Add(4 * time.Second))
	if !ok {
		t.Fatal("wrapped sample has no reading")
	}
	if diff := watts - 48.0; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("wrapped package power = %vW, want 48W", watts)
	}
}

func TestPowerMeterFallsBackWhenRAPLUnreadable(t *testing.T) {
	root := t.TempDir()
	powercap := filepath.Join(root, "powercap")
	hwmon := filepath.Join(root, "hwmon")

	// energy_uj is a directory, so stat succeeds but reading the counter
	// fails. This mirrors the common case where powercap exists but is not
	// readable by an unprivileged user.
	dir := filepath.Join(powercap, "intel-rapl:0")
	writeTestFile(t, filepath.Join(dir, "name"), "package-0\n")
	if err := os.MkdirAll(filepath.Join(dir, "energy_uj"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(hwmon, "hwmon0", "name"), "zenpower\n")
	writeTestFile(t, filepath.Join(hwmon, "hwmon0", "power1_input"), "45000000\n")

	meter := newPowerMeter(powercap, hwmon)
	watts, ok := meter.sample(time.Unix(1_700_000_000, 0))
	if !ok {
		t.Fatal("hwmon fallback has no reading")
	}
	if diff := watts - 45.0; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("fallback power = %vW, want 45W", watts)
	}
}

func TestPowerMeterHwmonPowerAndGPUIsolation(t *testing.T) {
	root := t.TempDir()
	hwmon := filepath.Join(root, "hwmon")

	writeTestFile(t, filepath.Join(hwmon, "hwmon0", "name"), "amdgpu\n")
	writeTestFile(t, filepath.Join(hwmon, "hwmon0", "power1_input"), "120000000\n")
	writeTestFile(t, filepath.Join(hwmon, "hwmon1", "name"), "zenpower\n")
	writeTestFile(t, filepath.Join(hwmon, "hwmon1", "power1_average"), "38000000\n")
	writeTestFile(t, filepath.Join(hwmon, "hwmon2", "name"), "coretemp\n")
	writeTestFile(t, filepath.Join(hwmon, "hwmon2", "temp1_input"), "60000\n")

	meter := newPowerMeter("", hwmon)
	watts, ok := meter.sample(time.Unix(1_700_000_000, 0))
	if !ok {
		t.Fatal("hwmon power has no reading")
	}
	if diff := watts - 38.0; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("CPU power = %vW, want 38W", watts)
	}
}

func TestPowerMeterHwmonEnergy(t *testing.T) {
	root := t.TempDir()
	hwmon := filepath.Join(root, "hwmon")
	dir := filepath.Join(hwmon, "hwmon0")
	writeTestFile(t, filepath.Join(dir, "name"), "amd_energy\n")
	writeTestFile(t, filepath.Join(dir, "energy1_input"), "1000000\n")
	writeTestFile(t, filepath.Join(dir, "energy2_input"), "2000000\n")

	meter := newPowerMeter("", hwmon)
	t0 := time.Unix(1_700_000_000, 0)
	if watts, ok := meter.sample(t0); ok || watts != 0 {
		t.Fatalf("first sample = %vW, %v; want no baseline", watts, ok)
	}

	writeTestFile(t, filepath.Join(dir, "energy1_input"), "3000000\n")
	writeTestFile(t, filepath.Join(dir, "energy2_input"), "4000000\n")
	watts, ok := meter.sample(t0.Add(2 * time.Second))
	if !ok {
		t.Fatal("second sample has no reading")
	}
	if diff := watts - 2.0; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("hwmon energy power = %vW, want 2W", watts)
	}
}

func TestPowerMeterReset(t *testing.T) {
	root := t.TempDir()
	hwmon := filepath.Join(root, "hwmon")
	dir := filepath.Join(hwmon, "hwmon0")
	writeTestFile(t, filepath.Join(dir, "name"), "amd_energy\n")
	writeTestFile(t, filepath.Join(dir, "energy1_input"), "1000000\n")

	meter := newPowerMeter("", hwmon)
	t0 := time.Unix(1_700_000_000, 0)
	meter.sample(t0)
	writeTestFile(t, filepath.Join(dir, "energy1_input"), "3000000\n")
	if _, ok := meter.sample(t0.Add(2 * time.Second)); !ok {
		t.Fatal("expected a reading before reset")
	}

	meter.reset()
	if watts, ok := meter.sample(t0.Add(4 * time.Second)); ok || watts != 0 {
		t.Fatalf("sample after reset = %vW, %v; want no baseline", watts, ok)
	}
	writeTestFile(t, filepath.Join(dir, "energy1_input"), "7000000\n")
	if watts, ok := meter.sample(t0.Add(6 * time.Second)); !ok || watts != 2.0 {
		t.Fatalf("sample after new baseline = %vW, %v; want 2W", watts, ok)
	}
}

func TestEnergyWattsRejectsInvalidDeltas(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if _, ok := energyWatts(energySample{at: now, uj: 10}, 20, 100, now); ok {
		t.Fatal("zero elapsed time should not produce a reading")
	}
	if _, ok := energyWatts(energySample{at: now, uj: 10}, 5, 0, now.Add(time.Second)); ok {
		t.Fatal("counter reset without a wrap point should not produce a reading")
	}
}
