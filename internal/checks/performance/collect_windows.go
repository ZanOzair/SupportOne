package performance

import (
	"context"
	"fmt"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in queries. Nothing here is assembled from user input or model
// output; see platform.RunRead.
const (
	psExe = "powershell"

	// Windows keeps no run-queue average, so LoadPercentage — an
	// instantaneous reading averaged across processors — is what there is.
	// The verdict treats it as the weaker evidence it is.
	queryLoad = `$os = Get-CimInstance Win32_OperatingSystem; ` +
		`$cs = Get-CimInstance Win32_ComputerSystem; ` +
		`$busy = (Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average; ` +
		`$page = Get-CimInstance Win32_PageFileUsage -ErrorAction SilentlyContinue | ` +
		`Measure-Object -Property AllocatedBaseSize, CurrentUsage -Sum; ` +
		`[pscustomobject]@{` +
		`Cores=$cs.NumberOfLogicalProcessors; ` +
		`BusyPercent=[int]$busy; ` +
		`TotalKB=$os.TotalVisibleMemorySize; ` +
		`FreeKB=$os.FreePhysicalMemory; ` +
		`PageTotalMB=($page | Where-Object Property -eq 'AllocatedBaseSize').Sum; ` +
		`PageUsedMB=($page | Where-Object Property -eq 'CurrentUsage').Sum` +
		`} | ConvertTo-Json -Compress`
)

func collectLoad(ctx context.Context, run platform.Runner) (loadFacts, error) {
	raw, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryLoad)
	if err != nil {
		return loadFacts{}, fmt.Errorf("performance: read load: %w", err)
	}
	return parseWindowsLoad(raw)
}
