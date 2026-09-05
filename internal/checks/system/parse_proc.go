package system

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseOSRelease reads the key=value pairs of /etc/os-release, unquoting values
// as the format allows.
func parseOSRelease(data []byte) map[string]string {
	out := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// A line like "=value" cuts cleanly but names nothing. Storing it
		// would put an empty key in the map, which no lookup can ever want.
		if key = strings.TrimSpace(key); key == "" {
			continue
		}
		out[key] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return out
}

// parseUptime reads the seconds field of /proc/uptime.
func parseUptime(data []byte) (time.Duration, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("system: /proc/uptime is empty")
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("system: parse /proc/uptime: %w", err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// parseCPUInfo reads the CPU model and the number of logical processors from
// /proc/cpuinfo. Field names differ by architecture, so several are accepted.
func parseCPUInfo(data []byte) (model string, cores int) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "processor":
			cores++
		case "model name", "Model", "cpu model", "Hardware":
			if model == "" {
				model = value
			}
		}
	}
	return model, cores
}

// parseMemTotal reads MemTotal from /proc/meminfo, whose unit is kibibytes.
func parseMemTotal(data []byte) (uint64, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok || strings.TrimSpace(key) != "MemTotal" {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			break
		}
		kib, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("system: parse MemTotal: %w", err)
		}
		return kib * 1024, nil
	}
	return 0, fmt.Errorf("system: MemTotal not found in /proc/meminfo")
}

// batteryHealth turns a battery's full-charge and design capacities into a
// percentage. It returns 0 when the pair cannot give an honest answer, which
// the check reports as unreadable rather than as a healthy battery.
func batteryHealth(full, design uint64) int {
	if full == 0 || design == 0 {
		return 0
	}
	percent := int(float64(full) / float64(design) * 100)
	if percent <= 0 {
		return 0
	}
	// A battery can read slightly above its design capacity when new.
	if percent > 100 {
		return 100
	}
	return percent
}

// parseSysfsUint reads a sysfs attribute holding a single unsigned number.
func parseSysfsUint(data []byte) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}
