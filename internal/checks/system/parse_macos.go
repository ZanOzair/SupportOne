package system

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// flexInt accepts a number or a quoted number. system_profiler reports some
// fields as strings on Apple silicon and as numbers on Intel.
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	s := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	// Apple silicon reports "proc 10:8:2" — total, performance, efficiency.
	if rest, ok := strings.CutPrefix(s, "proc "); ok {
		s, _, _ = strings.Cut(rest, ":")
	}
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("system: parse numeric field %q: %w", s, err)
	}
	*f = flexInt(v)
	return nil
}

// parseSWVers reads the "Key: value" lines sw_vers prints.
func parseSWVers(data []byte) map[string]string {
	out := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out
}

// parseBootTime reads the seconds field of `sysctl -n kern.boottime`, whose
// output looks like: { sec = 1693651200, usec = 0 } Sat Sep  2 13:20:00 2026
func parseBootTime(data []byte) (time.Time, error) {
	s := string(data)
	start := strings.Index(s, "sec = ")
	if start < 0 {
		return time.Time{}, fmt.Errorf("system: kern.boottime has no sec field")
	}
	rest := s[start+len("sec = "):]
	digits := rest
	if i := strings.IndexAny(rest, ",} "); i >= 0 {
		digits = rest[:i]
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(digits), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("system: parse kern.boottime: %w", err)
	}
	return time.Unix(seconds, 0).UTC(), nil
}

// macHardware mirrors the SPHardwareDataType entry system_profiler emits.
// Apple silicon and Intel Macs populate different fields for the same facts.
type macHardware struct {
	MachineName   string  `json:"machine_name"`
	MachineModel  string  `json:"machine_model"`
	ChipType      string  `json:"chip_type"`
	CPUType       string  `json:"cpu_type"`
	NumProcessors flexInt `json:"number_processors"`
	Cores         flexInt `json:"number_cores"`
}

type macHardwareReport struct {
	Hardware []macHardware `json:"SPHardwareDataType"`
}

func parseMacHardware(data []byte) (hardwareFacts, error) {
	var report macHardwareReport
	if err := json.Unmarshal(data, &report); err != nil {
		return hardwareFacts{}, fmt.Errorf("system: parse SPHardwareDataType: %w", err)
	}
	if len(report.Hardware) == 0 {
		return hardwareFacts{}, fmt.Errorf("system: SPHardwareDataType returned nothing")
	}
	hw := report.Hardware[0]

	cores := int(hw.NumProcessors)
	if cores == 0 {
		cores = int(hw.Cores)
	}
	return hardwareFacts{
		Vendor: "Apple",
		Model:  firstNonEmpty(hw.MachineName, hw.MachineModel),
		CPU:    firstNonEmpty(hw.ChipType, hw.CPUType, "unknown"),
		Cores:  cores,
	}, nil
}

// macBatteryHealth mirrors the health block of SPPowerDataType.
type macBatteryHealth struct {
	CycleCount      flexInt `json:"sppower_battery_cycle_count"`
	Health          string  `json:"sppower_battery_health"`
	MaximumCapacity string  `json:"sppower_battery_health_maximum_capacity"`
}

type macPowerEntry struct {
	Name   string           `json:"_name"`
	Health macBatteryHealth `json:"sppower_battery_health_info"`
}

type macPowerReport struct {
	Power []macPowerEntry `json:"SPPowerDataType"`
}

func parseMacBattery(data []byte) (batteryFacts, error) {
	var report macPowerReport
	if err := json.Unmarshal(data, &report); err != nil {
		return batteryFacts{}, fmt.Errorf("system: parse SPPowerDataType: %w", err)
	}

	for _, entry := range report.Power {
		health := entry.Health
		if health.CycleCount == 0 && health.MaximumCapacity == "" && health.Health == "" {
			continue
		}

		facts := batteryFacts{Present: true, CycleCount: int(health.CycleCount)}
		if percent, ok := parsePercent(health.MaximumCapacity); ok {
			facts.HealthPercent = percent
		}
		return facts, nil
	}

	// No battery block at all: a desktop Mac.
	return batteryFacts{Present: false}, nil
}

// parsePercent reads values like "91%" that macOS reports as strings.
func parsePercent(s string) (int, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return 0, false
	}
	if v > 100 {
		return 100, true
	}
	return v, true
}
