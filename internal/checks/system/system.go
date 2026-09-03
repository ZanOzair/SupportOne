// Package system reports what the machine is: its operating system, its
// hardware, its memory, and its battery.
//
// Every check here is read-only. Each platform has its own collector; the
// parsing of whatever the OS hands back is separated out so it can be tested
// against recorded output on any machine.
package system

import (
	"context"
	"time"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/platform"
)

// osFacts is what every platform's collector produces for os.info.
type osFacts struct {
	Name    string
	Version string
	Build   string
	Kernel  string
	Uptime  time.Duration

	// InstallDate is zero when the OS does not expose it. Zero renders as
	// "not reported", never as a guess.
	InstallDate time.Time
}

// hardwareFacts is what every platform's collector produces for
// hardware.inventory.
type hardwareFacts struct {
	Vendor string
	Model  string
	CPU    string
	Cores  int
}

// ramFacts is what every platform's collector produces for hardware.ram.
// Zero values mean the platform did not report that detail.
type ramFacts struct {
	TotalBytes uint64
	Slots      int
	SlotsUsed  int
	SpeedMHz   int
}

// batteryFacts is what every platform's collector produces for battery.health.
type batteryFacts struct {
	Present bool

	// HealthPercent is full charge capacity as a percentage of design
	// capacity. Zero means the machine did not report enough to compute it.
	HealthPercent int
	CycleCount    int
}

// Message keys for this package's results.
const (
	keyOSInfo             = "check.os.info.ok"
	keyHardwareInventory  = "check.hardware.inventory.ok"
	keyRAMOK              = "check.hardware.ram.ok"
	keyRAMLow             = "check.hardware.ram.low"
	keyBatteryAbsent      = "check.battery.health.absent"
	keyBatteryOK          = "check.battery.health.ok"
	keyBatteryWorn        = "check.battery.health.worn"
	keyBatteryFailing     = "check.battery.health.failing"
	keyBatteryUnreadable  = "check.battery.health.unreadable"
	keyHardwareUnreported = "check.hardware.inventory.unreported"
)

// lowMemoryBytes is the point below which a machine running a current desktop
// OS is memory-starved in a way the user will feel. It is a stated threshold,
// not a scare number: above it, the check reports OK.
const lowMemoryBytes = 4 << 30 // 4 GiB

type osInfoCheck struct{ run platform.Runner }

func (osInfoCheck) ID() string               { return "os.info" }
func (osInfoCheck) Platforms() []platform.OS { return platform.All() }
func (osInfoCheck) RequiresAdmin() bool      { return false }
func (c osInfoCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectOS(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}

	detail := map[string]any{
		"name":    facts.Name,
		"version": facts.Version,
		"kernel":  facts.Kernel,
		"uptime":  facts.Uptime.Round(time.Minute).String(),
	}
	if facts.Build != "" {
		detail["build"] = facts.Build
	}
	if !facts.InstallDate.IsZero() {
		detail["installed"] = facts.InstallDate.Format(time.RFC3339)
	}

	return checks.OK(keyOSInfo, facts.Name, facts.Version, checks.HumanDuration(facts.Uptime)).With(detail), nil
}

type hardwareInventoryCheck struct{ run platform.Runner }

func (hardwareInventoryCheck) ID() string               { return "hardware.inventory" }
func (hardwareInventoryCheck) Platforms() []platform.OS { return platform.All() }
func (hardwareInventoryCheck) RequiresAdmin() bool      { return false }
func (c hardwareInventoryCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectHardware(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}

	detail := map[string]any{"cpu": facts.CPU, "cores": facts.Cores}
	if facts.Vendor != "" {
		detail["vendor"] = facts.Vendor
	}
	if facts.Model != "" {
		detail["model"] = facts.Model
	}

	// A virtual machine or a board with no DMI data reports no model. Say so
	// rather than printing an empty name as if it were the answer.
	if facts.Model == "" {
		return checks.OK(keyHardwareUnreported, facts.CPU, facts.Cores).With(detail), nil
	}
	return checks.OK(keyHardwareInventory, facts.Vendor, facts.Model, facts.CPU, facts.Cores).With(detail), nil
}

type ramCheck struct{ run platform.Runner }

func (ramCheck) ID() string               { return "hardware.ram" }
func (ramCheck) Platforms() []platform.OS { return platform.All() }
func (ramCheck) RequiresAdmin() bool      { return false }
func (c ramCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectRAM(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}

	detail := map[string]any{"total_bytes": facts.TotalBytes}
	if facts.Slots > 0 {
		detail["slots"] = facts.Slots
		detail["slots_used"] = facts.SlotsUsed
	}
	if facts.SpeedMHz > 0 {
		detail["speed_mhz"] = facts.SpeedMHz
	}

	total := checks.HumanBytes(facts.TotalBytes)
	if facts.TotalBytes > 0 && facts.TotalBytes < lowMemoryBytes {
		return checks.Attention(keyRAMLow, total).With(detail), nil
	}
	return checks.OK(keyRAMOK, total).With(detail), nil
}

type batteryCheck struct{ run platform.Runner }

func (batteryCheck) ID() string               { return "battery.health" }
func (batteryCheck) Platforms() []platform.OS { return platform.All() }
func (batteryCheck) RequiresAdmin() bool      { return false }
func (c batteryCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectBattery(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}

	// A desktop with no battery is a fact, not a fault.
	if !facts.Present {
		return checks.OK(keyBatteryAbsent), nil
	}

	detail := map[string]any{}
	if facts.CycleCount > 0 {
		detail["cycle_count"] = facts.CycleCount
	}
	if facts.HealthPercent == 0 {
		return checks.Unknown(keyBatteryUnreadable).With(detail), nil
	}
	detail["health_percent"] = facts.HealthPercent

	switch {
	case facts.HealthPercent < 50:
		return checks.Urgent(keyBatteryFailing, facts.HealthPercent).With(detail), nil
	case facts.HealthPercent < 80:
		return checks.Attention(keyBatteryWorn, facts.HealthPercent).With(detail), nil
	default:
		return checks.OK(keyBatteryOK, facts.HealthPercent).With(detail), nil
	}
}

func init() {
	checks.MustRegister(osInfoCheck{run: platform.RunRead})
	checks.MustRegister(hardwareInventoryCheck{run: platform.RunRead})
	checks.MustRegister(ramCheck{run: platform.RunRead})
	checks.MustRegister(batteryCheck{run: platform.RunRead})
}

// firstNonEmpty returns the first value a platform actually reported. Every
// collector uses it so a blank field falls through to a stated fallback rather
// than rendering as an empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
