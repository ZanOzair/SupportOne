package performance

import (
	"strconv"
	"strings"
)

// parseSysctlLoadAvg reads the one-minute figure out of "{ 1.85 1.95 2.01 }".
func parseSysctlLoadAvg(raw []byte) (float64, bool) {
	fields := strings.Fields(strings.Trim(strings.TrimSpace(string(raw)), "{}"))
	if len(fields) == 0 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

// parseVMStatAvailable adds the page counts macOS would hand to a new process
// without swapping: free, plus inactive and speculative pages, which the
// system reclaims on demand.
func parseVMStatAvailable(raw []byte) uint64 {
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 {
		return 0
	}

	pageSize := parsePageSize(lines[0])
	if pageSize == 0 {
		return 0
	}

	wanted := map[string]bool{
		"Pages free":        true,
		"Pages inactive":    true,
		"Pages speculative": true,
	}

	var pages uint64
	for _, line := range lines[1:] {
		name, rest, found := strings.Cut(line, ":")
		if !found || !wanted[strings.TrimSpace(name)] {
			continue
		}
		count, err := strconv.ParseUint(strings.Trim(strings.TrimSpace(rest), "."), 10, 64)
		if err != nil {
			continue
		}
		pages += count
	}
	return pages * pageSize
}

// parsePageSize reads the size out of vm_stat's header line:
// "Mach Virtual Memory Statistics: (page size of 16384 bytes)".
func parsePageSize(header string) uint64 {
	const key = "page size of "
	at := strings.Index(header, key)
	if at < 0 {
		return 0
	}
	fields := strings.Fields(header[at+len(key):])
	if len(fields) == 0 {
		return 0
	}
	size, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return size
}

// parseSwapUsage reads vm.swapusage:
// "total = 2048.00M  used = 512.00M  free = 1536.00M  (encrypted)".
func parseSwapUsage(raw []byte) (total, used uint64) {
	fields := strings.Fields(string(raw))
	for i, field := range fields {
		if i+2 >= len(fields) || fields[i+1] != "=" {
			continue
		}
		value := parseSizeSuffix(fields[i+2])
		switch field {
		case "total":
			total = value
		case "used":
			used = value
		}
	}
	return total, used
}

// parseSizeSuffix reads "512.00M" and its siblings into bytes.
func parseSizeSuffix(s string) uint64 {
	if s == "" {
		return 0
	}

	var multiplier uint64
	switch s[len(s)-1] {
	case 'K':
		multiplier = 1 << 10
	case 'M':
		multiplier = 1 << 20
	case 'G':
		multiplier = 1 << 30
	case 'T':
		multiplier = 1 << 40
	default:
		// No suffix: the number is already bytes.
		value, err := strconv.ParseFloat(s, 64)
		if err != nil || value < 0 {
			return 0
		}
		return uint64(value)
	}

	value, err := strconv.ParseFloat(s[:len(s)-1], 64)
	if err != nil || value < 0 {
		return 0
	}
	return uint64(value * float64(multiplier))
}
