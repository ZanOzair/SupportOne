package performance

import (
	"strconv"
	"strings"
)

// parseLoadAvg reads the one-minute figure from /proc/loadavg, whose first
// field is what we want: "0.52 0.58 0.59 1/1234 5678".
func parseLoadAvg(raw []byte) (float64, bool) {
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

// parseMemInfo reads /proc/meminfo into bytes. Its values are in kibibytes
// despite the "kB" suffix, which is a long-standing kernel quirk.
func parseMemInfo(raw []byte) map[string]uint64 {
	out := make(map[string]uint64)

	for _, line := range strings.Split(string(raw), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		// ":0" cuts cleanly but names no field. An empty key in this map is
		// something no caller can look up, so it does not belong in it.
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		if len(fields) > 1 && strings.EqualFold(fields[1], "kB") {
			value *= 1024
		}
		out[name] = value
	}
	return out
}
