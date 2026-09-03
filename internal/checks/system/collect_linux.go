package system

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZanOzair/supportone/internal/platform"
)

// Roots are variables so tests can point them at recorded /proc and /sys trees.
var (
	procRoot = "/proc"
	sysRoot  = "/sys"
	etcRoot  = "/etc"
)

func collectOS(_ context.Context, _ platform.Runner) (osFacts, error) {
	release, err := os.ReadFile(filepath.Join(etcRoot, "os-release"))
	if err != nil {
		return osFacts{}, fmt.Errorf("system: read os-release: %w", err)
	}
	fields := parseOSRelease(release)

	facts := osFacts{
		Name:    firstNonEmpty(fields["NAME"], fields["PRETTY_NAME"], "Linux"),
		Version: firstNonEmpty(fields["VERSION"], fields["VERSION_ID"], "unknown"),
	}

	if kernel, err := os.ReadFile(filepath.Join(procRoot, "sys/kernel/osrelease")); err == nil {
		facts.Kernel = strings.TrimSpace(string(kernel))
	}
	uptimeData, err := os.ReadFile(filepath.Join(procRoot, "uptime"))
	if err != nil {
		return osFacts{}, fmt.Errorf("system: read uptime: %w", err)
	}
	if facts.Uptime, err = parseUptime(uptimeData); err != nil {
		return osFacts{}, err
	}

	// Linux does not record an install date the way Windows and macOS do.
	// InstallDate stays zero and the report says it was not reported.
	return facts, nil
}

func collectHardware(_ context.Context, _ platform.Runner) (hardwareFacts, error) {
	cpuinfo, err := os.ReadFile(filepath.Join(procRoot, "cpuinfo"))
	if err != nil {
		return hardwareFacts{}, fmt.Errorf("system: read cpuinfo: %w", err)
	}
	model, cores := parseCPUInfo(cpuinfo)

	facts := hardwareFacts{CPU: firstNonEmpty(model, "unknown"), Cores: cores}

	// DMI data is absent in most containers and on many ARM boards. Its
	// absence is reported, not treated as an error.
	facts.Vendor = readTrimmed(filepath.Join(sysRoot, "class/dmi/id/sys_vendor"))
	facts.Model = readTrimmed(filepath.Join(sysRoot, "class/dmi/id/product_name"))
	return facts, nil
}

func collectRAM(_ context.Context, _ platform.Runner) (ramFacts, error) {
	meminfo, err := os.ReadFile(filepath.Join(procRoot, "meminfo"))
	if err != nil {
		return ramFacts{}, fmt.Errorf("system: read meminfo: %w", err)
	}
	total, err := parseMemTotal(meminfo)
	if err != nil {
		return ramFacts{}, err
	}

	// Slot and speed detail needs DMI tables that require root. The check
	// reports the total it can read rather than asking for elevation it does
	// not otherwise need.
	return ramFacts{TotalBytes: total}, nil
}

func collectBattery(_ context.Context, _ platform.Runner) (batteryFacts, error) {
	entries, err := os.ReadDir(filepath.Join(sysRoot, "class/power_supply"))
	if err != nil {
		// No power_supply class at all: a machine with no battery interface.
		return batteryFacts{Present: false}, nil
	}

	for _, entry := range entries {
		dir := filepath.Join(sysRoot, "class/power_supply", entry.Name())
		if readTrimmed(filepath.Join(dir, "type")) != "Battery" {
			continue
		}

		facts := batteryFacts{Present: true}
		if cycles, err := parseSysfsUint([]byte(readTrimmed(filepath.Join(dir, "cycle_count")))); err == nil && cycles <= math.MaxInt32 {
			facts.CycleCount = int(cycles)
		}

		// Kernels expose either energy_* (µWh) or charge_* (µAh); either pair
		// gives the same ratio.
		for _, pair := range [][2]string{
			{"energy_full", "energy_full_design"},
			{"charge_full", "charge_full_design"},
		} {
			full, fullErr := parseSysfsUint([]byte(readTrimmed(filepath.Join(dir, pair[0]))))
			design, designErr := parseSysfsUint([]byte(readTrimmed(filepath.Join(dir, pair[1]))))
			if fullErr == nil && designErr == nil {
				facts.HealthPercent = batteryHealth(full, design)
				break
			}
		}
		return facts, nil
	}

	return batteryFacts{Present: false}, nil
}

func readTrimmed(path string) string {
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from compiled-in /proc and /sys locations.
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
