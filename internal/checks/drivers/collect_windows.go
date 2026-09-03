package drivers

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

// Compiled-in query; see platform.RunRead.
const (
	psExe = "powershell"

	queryProblemDevices = `Get-CimInstance Win32_PnPEntity -Filter "ConfigManagerErrorCode <> 0" | ` +
		`Select-Object Name,DeviceID,ConfigManagerErrorCode | ConvertTo-Json -Compress`
)

func (c problemCheck) Run(ctx context.Context) (checks.Result, error) {
	out, err := c.run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryProblemDevices)
	if err != nil {
		return checks.UnknownFor(err), nil
	}

	devices, err := parseProblemDevices(out)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	return problemVerdict(devices), nil
}
