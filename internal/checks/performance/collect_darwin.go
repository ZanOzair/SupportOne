package performance

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

const sysctlExe = "sysctl"

func collectLoad(ctx context.Context, run platform.Runner) (loadFacts, error) {
	var facts loadFacts

	if raw, err := run(ctx, sysctlExe, "-n", "hw.ncpu"); err == nil {
		facts.Cores, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
	}

	// vm.loadavg is the one figure macOS keeps that means the same thing as
	// Linux's: work waiting for a processor.
	raw, err := run(ctx, sysctlExe, "-n", "vm.loadavg")
	if err != nil {
		return loadFacts{}, fmt.Errorf("performance: read vm.loadavg: %w", err)
	}
	if value, ok := parseSysctlLoadAvg(raw); ok {
		facts.LoadAverage, facts.HasLoadAverage = value, true
	}

	if raw, err := run(ctx, sysctlExe, "-n", "hw.memsize"); err == nil {
		facts.MemTotalBytes, _ = strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	}

	// vm_stat counts pages rather than bytes, and "available" on macOS means
	// free plus the pages the system would reclaim without swapping.
	if raw, err := run(ctx, "vm_stat"); err == nil {
		facts.MemAvailableBytes = parseVMStatAvailable(raw)
	}

	if raw, err := run(ctx, sysctlExe, "-n", "vm.swapusage"); err == nil {
		facts.SwapTotalBytes, facts.SwapUsedBytes = parseSwapUsage(raw)
	}
	return facts, nil
}
