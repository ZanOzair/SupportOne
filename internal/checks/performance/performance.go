// Package performance reports whether a machine is under load right now: how
// busy its processors are, and how close it is to running out of memory.
//
// One honest limit shapes everything here. A snapshot is a moment, and a
// moment of high CPU is not a fault — a machine compiling something, or
// playing a video, is supposed to be busy. So a busy processor is never
// reported as urgent, however busy it is, and the message says it is one
// reading. Memory is different: a machine with almost nothing free is
// struggling now, not maybe, so that can be urgent.
package performance

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// loadFacts is what every platform's collector produces.
//
// The two ways of describing a busy processor are kept apart rather than
// blended. Unix keeps a run-queue average over a minute; Windows offers an
// instantaneous percentage. They do not mean the same thing, and a check that
// pretended they did would be reporting a number it made up.
type loadFacts struct {
	Cores int

	// LoadAverage is the one-minute run-queue average, where the platform
	// keeps one.
	LoadAverage    float64
	HasLoadAverage bool

	// BusyPercent is an instantaneous processor reading, which is what
	// Windows offers instead.
	BusyPercent int
	HasBusy     bool

	MemTotalBytes     uint64
	MemAvailableBytes uint64

	// Swap is the page file or swap area. Zero total means none is
	// configured, which is a fact rather than a fault.
	SwapTotalBytes uint64
	SwapUsedBytes  uint64
}

// Message keys for this package's results.
const (
	keyLoadOK             = "check.performance.load.ok"
	keyLoadBusy           = "check.performance.load.busy"
	keyLoadBusyNow        = "check.performance.load.busy_now"
	keyLoadMemoryLow      = "check.performance.load.memory_low"
	keyLoadMemoryCritical = "check.performance.load.memory_critical"
	keyLoadSwapping       = "check.performance.load.swapping"
	keyLoadUnreadable     = "check.performance.load.unreadable"
)

// The thresholds, written down. Nothing here decides a verdict by a number
// this file does not name.
const (
	// memoryCriticalFraction: below this much of installed memory available,
	// the machine is paging to keep going and the user feels every click.
	memoryCriticalFraction = 0.10

	// memoryLowFraction: below this, there is no headroom left for anything
	// the user opens next.
	memoryLowFraction = 0.20

	// swapPressureFraction: more than half the page file in use alongside
	// low memory means the machine has already run out once.
	swapPressureFraction = 0.50

	// loadPerCoreBusy: a one-minute run queue this many times the core count
	// means work is waiting rather than running.
	loadPerCoreBusy = 2.0

	// busyPercentNow: an instantaneous reading at or above this is worth
	// mentioning, and never more than that.
	busyPercentNow = 90
)

type loadCheck struct{ run platform.Runner }

func (loadCheck) ID() string               { return "performance.load" }
func (loadCheck) Platforms() []platform.OS { return platform.All() }
func (loadCheck) RequiresAdmin() bool      { return false }

func (c loadCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectLoad(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	return loadVerdict(facts), nil
}

// loadVerdict decides from the facts alone, so every branch is testable
// without a machine under load.
func loadVerdict(facts loadFacts) checks.Result {
	detail := map[string]any{"cores": facts.Cores}
	if facts.HasLoadAverage {
		detail["load_average_1m"] = facts.LoadAverage
	}
	if facts.HasBusy {
		detail["busy_percent"] = facts.BusyPercent
	}
	if facts.MemTotalBytes > 0 {
		detail["memory_total_bytes"] = facts.MemTotalBytes
		detail["memory_available_bytes"] = facts.MemAvailableBytes
	}
	if facts.SwapTotalBytes > 0 {
		detail["swap_total_bytes"] = facts.SwapTotalBytes
		detail["swap_used_bytes"] = facts.SwapUsedBytes
	}

	// A machine that reported neither a load figure nor a memory total told
	// us nothing. Unknown, not OK.
	if facts.MemTotalBytes == 0 && !facts.HasLoadAverage && !facts.HasBusy {
		return checks.Unknown(keyLoadUnreadable).With(detail)
	}

	available := memoryFraction(facts)
	free := checks.HumanBytes(facts.MemAvailableBytes)
	total := checks.HumanBytes(facts.MemTotalBytes)

	// Worst first. Memory is the only thing here that can be urgent, because
	// it is the only one a single reading can establish.
	switch {
	case facts.MemTotalBytes > 0 && available < memoryCriticalFraction:
		return checks.Urgent(keyLoadMemoryCritical, free, total).With(detail)
	case facts.MemTotalBytes > 0 && available < memoryLowFraction:
		return checks.Attention(keyLoadMemoryLow, free, total).With(detail)
	case swapping(facts):
		return checks.Attention(keyLoadSwapping, checks.HumanBytes(facts.SwapUsedBytes)).With(detail)
	case facts.HasLoadAverage && facts.Cores > 0 && facts.LoadAverage/float64(facts.Cores) >= loadPerCoreBusy:
		return checks.Attention(keyLoadBusy, facts.LoadAverage, facts.Cores).With(detail)
	case facts.HasBusy && facts.BusyPercent >= busyPercentNow:
		return checks.Attention(keyLoadBusyNow, facts.BusyPercent).With(detail)
	default:
		return checks.OK(keyLoadOK, free, total).With(detail)
	}
}

// memoryFraction is how much of installed memory is still available. A
// platform that did not report a total yields 1, so it never triggers a
// verdict on a number nobody gave us.
func memoryFraction(facts loadFacts) float64 {
	if facts.MemTotalBytes == 0 {
		return 1
	}
	return float64(facts.MemAvailableBytes) / float64(facts.MemTotalBytes)
}

// swapping reports heavy page-file use. A little swap in use is normal on
// every modern OS; half of it is not.
func swapping(facts loadFacts) bool {
	if facts.SwapTotalBytes == 0 {
		return false
	}
	return float64(facts.SwapUsedBytes)/float64(facts.SwapTotalBytes) > swapPressureFraction
}

func init() {
	checks.MustRegister(loadCheck{run: platform.RunRead})
}
