package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseNvidiaCSV(t *testing.T) {
	out := "0, NVIDIA GeForce RTX 5050 Laptop GPU, 00000000:01:00.0, 7, 289, 8151, 55, 180, 3090, 405, 7001, 5.28, [N/A]\n" +
		"1, NVIDIA Test, 00000000:02:00.0, [N/A], [N/A], [N/A], [N/A], [N/A], [N/A], [N/A], [N/A], [N/A], [N/A]\n"
	stats := parseNvidiaCSV(out)
	if len(stats) != 2 {
		t.Fatalf("got %d stats", len(stats))
	}
	g := stats[0]
	if g.name != "NVIDIA GeForce RTX 5050 Laptop GPU" || g.slot != "0:01:00.0" {
		t.Fatalf("unexpected first row: %+v", g)
	}
	if !g.hasUtil || g.util != 7 {
		t.Errorf("util = %v/%v", g.util, g.hasUtil)
	}
	if g.memUsed != 289*1024*1024 || g.memTotal != 8151*1024*1024 {
		t.Errorf("memory = %d/%d", g.memUsed, g.memTotal)
	}
	if !g.hasTemp || g.temp != 55 || g.core != 180 || g.maxCore != 3090 {
		t.Errorf("temp/clock = %+v", g)
	}
	if g.power != 5.28 || g.limit != 0 {
		t.Errorf("power = %v/%v", g.power, g.limit)
	}
	if stats[1].hasUtil || stats[1].hasTemp || stats[1].memTotal != 0 {
		t.Errorf("N/A fields should be unavailable: %+v", stats[1])
	}
}

func TestParseNvidiaCSVQuotedName(t *testing.T) {
	out := "0, \"NVIDIA, Test GPU\", 00000000:01:00.0, 7, 289, 8151, 55, 180, 3090, 405, 7001, 5.28, 100\n"
	stats := parseNvidiaCSV(out)
	if len(stats) != 1 {
		t.Fatalf("got %d stats", len(stats))
	}
	if stats[0].name != "NVIDIA, Test GPU" {
		t.Fatalf("name = %q", stats[0].name)
	}
	if stats[0].limit != 100 {
		t.Fatalf("limit = %v", stats[0].limit)
	}
}

func TestNormalizeSlot(t *testing.T) {
	tests := map[string]string{
		"00000000:01:00.0": "0:01:00.0",
		"0000:01:00.0":     "0:01:00.0",
		"0:01:00.0":        "0:01:00.0",
		"":                 "",
	}
	for in, want := range tests {
		if got := normalizeSlot(in); got != want {
			t.Errorf("normalizeSlot(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFillAMDUsesProductNameFallback(t *testing.T) {
	dev := t.TempDir()
	if err := os.WriteFile(filepath.Join(dev, "product_name"), []byte("AMD Radeon RX 7800 XT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := GPU{Name: "AMD GPU"}
	fillAMD(&g, gpuCard{dev: dev})
	if g.Name != "AMD Radeon RX 7800 XT" {
		t.Fatalf("name = %q", g.Name)
	}

	// A specific lspci name is more useful than the generic product_name.
	g = GPU{Name: "AMD Radeon 780M"}
	fillAMD(&g, gpuCard{dev: dev})
	if g.Name != "AMD Radeon 780M" {
		t.Fatalf("lspci name was overwritten: %q", g.Name)
	}
}

func TestReadPpDpm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pp_dpm_sclk")
	body := "0: 500Mhz\n1: 2100Mhz *\n2: 2500Mhz\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cur, max := readPpDpm(path)
	if cur != 2100 || max != 2500 {
		t.Fatalf("readPpDpm = %v/%v", cur, max)
	}
	if cur, max := readPpDpm(filepath.Join(t.TempDir(), "missing")); cur != 0 || max != 0 {
		t.Fatalf("missing table = %v/%v", cur, max)
	}
}

func TestQuotedFields(t *testing.T) {
	fields := quotedFields(`0000:01:00.0 "VGA compatible controller" "NVIDIA Corporation" "GB207M [GeForce RTX 5050 Max-Q / Mobile]" -ra1 -p00`)
	want := []string{"0000:01:00.0", "VGA compatible controller", "NVIDIA Corporation", "GB207M [GeForce RTX 5050 Max-Q / Mobile]", "-ra1", "-p00"}
	if len(fields) != len(want) {
		t.Fatalf("fields = %#v", fields)
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Errorf("field %d = %q, want %q", i, fields[i], want[i])
		}
	}
}

// TestCollectGPUsLive exercises real sysfs and vendor tools when a GPU is
// present. It is skipped on machines without one.
func TestCollectGPUsLive(t *testing.T) {
	c := &Collector{}
	gpus := c.collectGPUs(time.Now())
	if len(gpus) == 0 {
		t.Skip("no GPUs detected")
	}
	for _, g := range gpus {
		t.Logf("#%d %s vendor=%s util=%.0f(%v) temp=%.0f(%v) clock=%.0f/%.0f vram=%d/%d",
			g.Index, g.Name, g.Vendor,
			g.UtilPercent, g.HasUtil, g.Temp, g.HasTemp, g.CoreMHz, g.MaxCoreMHz,
			g.MemUsed, g.MemTotal)
		if g.Name == "" {
			t.Errorf("GPU %d has no name", g.Index)
		}
		if g.HasUtil && (g.UtilPercent < 0 || g.UtilPercent > 100) {
			t.Errorf("GPU %d utilization = %v", g.Index, g.UtilPercent)
		}
		if g.HasTemp && (g.Temp < 0 || g.Temp > 150) {
			t.Errorf("GPU %d temperature = %v", g.Index, g.Temp)
		}
	}
}
