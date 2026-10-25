// Package metrics collects live system statistics from Linux's /proc and
// /sys filesystems.
package metrics

import (
	"encoding/hex"
	"net"
	"os"
	"strconv"
	"strings"
)

// Connections returns the active TCP and UDP sockets observed from
// /proc/net/tcp*, /proc/net/udp*. Fields that are not available are left at
// their zero value.
func Connections() []Connection {
	var out []Connection
	for _, proto := range []string{"tcp", "tcp6", "udp", "udp6"} {
		path := "/proc/net/" + proto
		conns, err := parseConnFile(path, proto)
		if err != nil {
			continue
		}
		out = append(out, conns...)
	}
	return out
}

// parseConnFile reads one /proc/net/{tcp,tcp6,udp,udp6} file.
func parseConnFile(path, proto string) ([]Connection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Connection
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // header or blank
		}
		fields := strings.Fields(line)
		if len(fields) < 12 {
			continue
		}
		// sl local_address rem_address st tx_queue rx_queue tr ... uid ... inode
		local := parseAddr(fields[1])
		remote := parseAddr(fields[2])
		state := fields[3]
		inode, _ := strconv.ParseUint(fields[11], 10, 64)

		isTCP := strings.HasPrefix(proto, "tcp")
		c := Connection{
			Proto:  proto,
			Local:  local,
			Remote: remote,
			State:  tcpState(state),
			Inode:  inode,
		}
		if !isTCP {
			c.State = "-"
		}
		out = append(out, c)
	}
	return out, nil
}

// parseAddr converts a hex "ip:port" address from /proc/net/* into "ip:port".
func parseAddr(s string) string {
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return s
	}
	ipHex := s[:idx]
	portHex := s[idx+1:]
	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return s
	}
	var ip net.IP
	switch len(ipHex) {
	case 8: // IPv4 (stored little-endian in /proc/net/*)
		b, err := hex.DecodeString(ipHex)
		if err != nil || len(b) != 4 {
			return s
		}
		// Reverse the 4 bytes to network byte order.
		for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		ip = net.IPv4(b[0], b[1], b[2], b[3])
	case 32: // IPv6
		b, err := hex.DecodeString(ipHex)
		if err != nil || len(b) != 16 {
			return s
		}
		ip = net.IP(b)
	default:
		return s
	}
	return net.JoinHostPort(ip.String(), strconv.FormatUint(port, 10))
}

// tcpState converts a /proc/net/tcp state code to a name.
func tcpState(code string) string {
	switch strings.ToUpper(code) {
	case "01":
		return "ESTABLISHED"
	case "02":
		return "SYN_SENT"
	case "03":
		return "SYN_RECV"
	case "04":
		return "FIN_WAIT1"
	case "05":
		return "FIN_WAIT2"
	case "06":
		return "TIME_WAIT"
	case "07":
		return "CLOSE"
	case "08":
		return "CLOSE_WAIT"
	case "09":
		return "LAST_ACK"
	case "0A":
		return "LISTEN"
	case "0B":
		return "CLOSING"
	default:
		return code
	}
}
