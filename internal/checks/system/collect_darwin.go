package system

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Command names and arguments are compiled-in constants; see platform.RunRead.
const (
	swVersExe        = "sw_vers"
	sysctlExe        = "sysctl"
	unameExe         = "uname"
	systemProfileExe = "system_profiler"
)

func collectOS(ctx context.Context, run platform.Runner) (osFacts, error) {
	versions, err := run(ctx, swVersExe)
	if err != nil {
		return osFacts{}, err
	}
	fields := parseSWVers(versions)

	facts := osFacts{
		Name:    firstNonEmpty(fields["ProductName"], "macOS"),
		Version: firstNonEmpty(fields["ProductVersion"], "unknown"),
		Build:   fields["BuildVersion"],
	}

	if kernel, err := run(ctx, unameExe, "-r"); err == nil {
		facts.Kernel = strings.TrimSpace(string(kernel))
	}
	boottime, err := run(ctx, sysctlExe, "-n", "kern.boottime")
	if err != nil {
		return osFacts{}, err
	}
	booted, err := parseBootTime(boottime)
	if err != nil {
		return osFacts{}, err
	}
	facts.Uptime = time.Since(booted)

	// macOS does not expose an install date without inspecting file
	// timestamps, which would be a guess. InstallDate stays zero.
	return facts, nil
}

func collectHardware(ctx context.Context, run platform.Runner) (hardwareFacts, error) {
	out, err := run(ctx, systemProfileExe, "-json", "SPHardwareDataType")
	if err != nil {
		return hardwareFacts{}, err
	}
	return parseMacHardware(out)
}

func collectRAM(ctx context.Context, run platform.Runner) (ramFacts, error) {
	out, err := run(ctx, sysctlExe, "-n", "hw.memsize")
	if err != nil {
		return ramFacts{}, err
	}
	total, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return ramFacts{}, fmt.Errorf("system: parse hw.memsize: %w", err)
	}

	// Apple silicon memory is on-package: there are no slots to report, and
	// reporting a slot count would be inventing one.
	return ramFacts{TotalBytes: total}, nil
}

func collectBattery(ctx context.Context, run platform.Runner) (batteryFacts, error) {
	out, err := run(ctx, systemProfileExe, "-json", "SPPowerDataType")
	if err != nil {
		return batteryFacts{}, err
	}
	return parseMacBattery(out)
}
