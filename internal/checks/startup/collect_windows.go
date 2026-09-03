package startup

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in query; see platform.RunRead.
const (
	psExe = "powershell"

	queryStartup = `Get-CimInstance Win32_StartupCommand | Select-Object Name,Command,Location,User | ConvertTo-Json -Compress`
)

func collectStartupItems(ctx context.Context, run platform.Runner) ([]item, error) {
	out, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryStartup)
	if err != nil {
		return nil, err
	}
	return parseWindowsStartup(out)
}
