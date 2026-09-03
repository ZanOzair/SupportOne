package drivers

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in query; see platform.RunRead.
const (
	psExe = "powershell"

	queryProblemDevices = `Get-CimInstance Win32_PnPEntity -Filter "ConfigManagerErrorCode <> 0" | ` +
		`Select-Object Name,DeviceID,ConfigManagerErrorCode | ConvertTo-Json -Compress`
)

func collectProblemDevices(ctx context.Context, run platform.Runner) ([]device, error) {
	out, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryProblemDevices)
	if err != nil {
		return nil, err
	}
	return parseProblemDevices(out)
}
