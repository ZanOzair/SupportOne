package performance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// procRoot is a variable so tests can point it at a recorded /proc tree.
var procRoot = "/proc"

func collectLoad(_ context.Context, _ platform.Runner) (loadFacts, error) {
	facts := loadFacts{Cores: runtime.NumCPU()}

	loadavg, err := os.ReadFile(filepath.Join(procRoot, "loadavg"))
	if err != nil {
		return loadFacts{}, fmt.Errorf("performance: read loadavg: %w", err)
	}
	if value, ok := parseLoadAvg(loadavg); ok {
		facts.LoadAverage, facts.HasLoadAverage = value, true
	}

	meminfo, err := os.ReadFile(filepath.Join(procRoot, "meminfo"))
	if err != nil {
		return loadFacts{}, fmt.Errorf("performance: read meminfo: %w", err)
	}
	mem := parseMemInfo(meminfo)

	facts.MemTotalBytes = mem["MemTotal"]
	// MemAvailable is the kernel's own estimate of what a new process could
	// get without swapping. It is a better answer than MemFree, which counts
	// cache as used.
	facts.MemAvailableBytes = mem["MemAvailable"]
	if facts.MemAvailableBytes == 0 {
		facts.MemAvailableBytes = mem["MemFree"]
	}

	facts.SwapTotalBytes = mem["SwapTotal"]
	if facts.SwapTotalBytes > 0 {
		facts.SwapUsedBytes = facts.SwapTotalBytes - mem["SwapFree"]
	}
	return facts, nil
}
