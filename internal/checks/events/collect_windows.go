package events

import (
	"context"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in query; see platform.RunRead. Levels 1 and 2 are critical and
// error. The System log is readable by ordinary users; the Security log, which
// is not, is deliberately not read.
const (
	psExe = "powershell"

	queryEvents = `Get-WinEvent -FilterHashtable @{LogName='System'; Level=1,2; StartTime=(Get-Date).AddDays(-7)} ` +
		`-MaxEvents 200 -ErrorAction SilentlyContinue | ` +
		`Select-Object TimeCreated,Id,ProviderName,LevelDisplayName,Message | ConvertTo-Json -Compress`
)

const lookback = 7 * 24 * time.Hour

func collectEvents(ctx context.Context, run platform.Runner) ([]logEvent, time.Duration, error) {
	out, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryEvents)
	if err != nil {
		return nil, lookback, err
	}
	events, err := parseWindowsEvents(out)
	return events, lookback, err
}
