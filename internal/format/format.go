// Package format provides small helpers for turning raw numbers and text into
// the short strings shown in the user interface.
package format

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
)

var byteUnits = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

// Bytes formats a number of bytes using binary (IEC) units.
// It is deliberately compact: "1.5 GiB", "512 MiB", "48 B".
func Bytes(n uint64) string {
	return human(float64(n), byteUnits, "")
}

// Rate formats a throughput value in bytes per second.
func Rate(bytesPerSecond float64) string {
	if bytesPerSecond < 0 {
		bytesPerSecond = 0
	}
	return human(bytesPerSecond, byteUnits, "/s")
}

// BytesShort formats bytes using one-letter units, for places where two
// values share a line and readability matters more than spelling out IEC
// names: "2.8G/8.0G", "512M", "48B".
func BytesShort(n uint64) string {
	const units = "BKMGTPE"
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v < 10 {
		return fmt.Sprintf("%.1f%c", v, units[i])
	}
	return fmt.Sprintf("%.0f%c", v, units[i])
}

func human(v float64, units []string, suffix string) string {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		v = 0
	}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s%s", v, units[i], suffix)
	}
	if v < 10 || (i >= 2 && v < 100) {
		return fmt.Sprintf("%.1f %s%s", v, units[i], suffix)
	}
	return fmt.Sprintf("%.0f %s%s", v, units[i], suffix)
}

// Duration formats a number of seconds as a short uptime-style duration.
func Duration(seconds uint64) string {
	d := time.Duration(seconds) * time.Second
	return DurationShort(d)
}

// DurationShort formats a time.Duration as "3d 4h", "2h 5m", "12m 30s" or "8s".
func DurationShort(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int64(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	mins := int64(d / time.Minute)
	secs := int64((d - time.Duration(mins)*time.Minute) / time.Second)

	switch {
	case days > 0:
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	case hours > 0:
		if mins > 0 {
			return fmt.Sprintf("%dh %dm", hours, mins)
		}
		return fmt.Sprintf("%dh", hours)
	case mins > 0:
		if days == 0 && secs > 0 && mins < 10 {
			return fmt.Sprintf("%dm %ds", mins, secs)
		}
		return fmt.Sprintf("%dm", mins)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

// Temperature formats a Celsius value. Fahrenheit is used when unit is "f".
// Values outside the range a machine sensor can plausibly report are shown as
// an en dash; zero and negative readings are valid and are formatted normally.
func Temperature(celsius float64, unit string) string {
	if math.IsNaN(celsius) || math.IsInf(celsius, 0) || celsius < -100 || celsius > 150 {
		return "–"
	}
	if strings.EqualFold(unit, "f") {
		return fmt.Sprintf("%.0f°F", celsius*9/5+32)
	}
	return fmt.Sprintf("%.0f°C", celsius)
}

// Wrap breaks plain text into lines of at most width terminal cells. It
// understands words and hard-wraps words that are longer than a line. Wide
// runes (CJK, emoji) count as two cells, matching how the terminal renders
// them.
func Wrap(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string
	for _, paragraph := range strings.Split(s, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, w := range words {
			wordWidth := runewidth.StringWidth(w)
			if wordWidth > width {
				if line != "" {
					lines = append(lines, line)
					line = ""
				}
				for wordWidth > width {
					cut, used := cutCells(w, width)
					lines = append(lines, cut)
					w = w[len(cut):]
					wordWidth -= used
				}
				line = w
				continue
			}
			if line == "" {
				line = w
			} else if runewidth.StringWidth(line)+1+wordWidth <= width {
				line += " " + w
			} else {
				lines = append(lines, line)
				line = w
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// cutCells returns the longest prefix of s that fits in width cells and the
// number of cells it occupies. A single wide rune is always returned even
// when it is wider than width, so callers make progress.
func cutCells(s string, width int) (string, int) {
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if used+rw > width && b.Len() > 0 {
			break
		}
		b.WriteRune(r)
		used += rw
		if used >= width {
			break
		}
	}
	return b.String(), used
}
