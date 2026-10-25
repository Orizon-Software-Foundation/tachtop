package metrics

import (
	"os"
	"testing"
)

func TestParseAddr(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"0100007F:1F90", "127.0.0.1:8080"},
		{"00000000:0016", "0.0.0.0:22"},
		{"00000000000000000000000000000000:1F90", "[::]:8080"},
		{"2A032880:F236", "128.40.3.42:62006"},
		{"garbage", "garbage"},
	}
	for _, tt := range tests {
		if got := parseAddr(tt.in); got != tt.want {
			t.Errorf("parseAddr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTCPState(t *testing.T) {
	tests := []struct {
		code, want string
	}{
		{"01", "ESTABLISHED"},
		{"02", "SYN_SENT"},
		{"06", "TIME_WAIT"},
		{"0A", "LISTEN"},
		{"0B", "CLOSING"},
		{"FF", "FF"},
	}
	for _, tt := range tests {
		if got := tcpState(tt.code); got != tt.want {
			t.Errorf("tcpState(%q) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestParseConnFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/tcp"
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 00000000:0000 0A 0000 0000 00 00000000 00000000 0 0 12345
   1: 00000000:0050 0100007F:1234 01 0000 0000 00 00000000 00000000 0 0 67890
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	conns, err := parseConnFile(path, "tcp")
	if err != nil {
		t.Fatalf("parseConnFile: %v", err)
	}
	if len(conns) != 2 {
		t.Fatalf("got %d conns, want 2", len(conns))
	}
	if conns[0].Proto != "tcp" || conns[0].Local != "127.0.0.1:8080" {
		t.Errorf("conn0 = %+v", conns[0])
	}
	if conns[0].State != "LISTEN" || conns[0].Inode != 12345 {
		t.Errorf("conn0 state/inode = %+v", conns[0])
	}
	if conns[1].Remote != "127.0.0.1:4660" || conns[1].State != "ESTABLISHED" {
		t.Errorf("conn1 = %+v", conns[1])
	}

	// Malformed/short lines are skipped.
	bad := dir + "/badtcp"
	if err := os.WriteFile(bad, []byte("header\n   0: short line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseConnFile(bad, "tcp")
	if err != nil {
		t.Fatalf("parseConnFile bad: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 conns from bad input, got %d", len(got))
	}
}
