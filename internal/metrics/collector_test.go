package metrics

import (
	"math"
	"os"
	"testing"

	"github.com/shirou/gopsutil/v4/host"
)

func TestParseDefaultRoutes(t *testing.T) {
	data := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlp3s0	00000000	0100A8C0	0003	0	0	600	00000000	0	0	0
docker0	00000000	00000000	0001	0	0	0	00000000	0	0	0
eth0	0001A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
`
	got := parseDefaultRoutes(data)
	if !got["wlp3s0"] || !got["docker0"] {
		t.Fatalf("default routes = %v", got)
	}
	if got["eth0"] {
		t.Fatalf("non-default route was included: %v", got)
	}
}

func TestIsPseudoFilesystem(t *testing.T) {
	tests := map[string]bool{
		"ext4":       false,
		"btrfs":      false,
		"overlay":    false,
		"tmpfs":      true,
		"proc":       true,
		"fuse.sshfs": true,
		"nfs4":       true,
		"cifs":       true,
	}
	for fs, want := range tests {
		if got := isPseudoFilesystem(fs); got != want {
			t.Errorf("isPseudoFilesystem(%q) = %v, want %v", fs, got, want)
		}
	}
}

func TestCollectorZeroValueInit(t *testing.T) {
	var c Collector
	c.initStatic()
	if c.users == nil || c.prevNet == nil || c.prevProcs == nil {
		t.Fatal("initStatic did not initialise maps")
	}
	if got := c.userFor(int32(os.Getpid())); got == "" {
		t.Fatal("userFor returned an empty user")
	}
}

func TestCollectorZeroValueCollect(t *testing.T) {
	var c Collector
	c.Reset()
	s := c.Collect()
	if s.At.IsZero() || s.Hostname == "" {
		t.Fatalf("zero-value collector returned an incomplete snapshot: %+v", s)
	}
}

func TestCollectorResetDuringCollect(t *testing.T) {
	c := New()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Collect()
	}()
	for i := 0; i < 20; i++ {
		c.Reset()
	}
	<-done
}

func TestSignalInvalidPID(t *testing.T) {
	for _, pid := range []int32{0, -1} {
		if err := Signal(pid, false); err == nil {
			t.Errorf("Signal(%d) should fail", pid)
		}
	}
}

func TestSignalCheckedRejectsReplacedProcess(t *testing.T) {
	pid := int32(os.Getpid())
	ps, err := readProcStat(pid)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := host.BootTime()
	if err != nil {
		t.Skip("boot time unavailable")
	}
	created := int64(boot) + int64(float64(ps.startTicks)/userHZ)

	c := New()
	if err := c.SignalChecked(pid, created+1, false); err == nil {
		t.Fatal("expected a start-time mismatch")
	}
}

func TestClampNaN(t *testing.T) {
	if got := clamp(math.NaN(), 0, 100); got != 0 {
		t.Fatalf("clamp(NaN) = %v", got)
	}
}
