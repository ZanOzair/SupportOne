package patches

import (
	"context"
	"fmt"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in query; see platform.RunRead for why this is not shell
// construction.
const (
	psExe = "powershell"

	// Win32_QuickFixEngineering is the record Windows keeps of servicing
	// updates. It does not cover driver updates or Store apps, which is why
	// the source is named in the result rather than implied.
	queryQuickFix = `Get-CimInstance Win32_QuickFixEngineering -ErrorAction SilentlyContinue | ` +
		`Select-Object HotFixID, Description, InstalledOn | ConvertTo-Json -Compress`
)

func collectPatches(ctx context.Context, run platform.Runner) (patchFacts, error) {
	raw, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryQuickFix)
	if err != nil {
		return patchFacts{}, fmt.Errorf("patches: read the update record: %w", err)
	}

	applied, err := parseQuickFix(raw)
	if err != nil {
		return patchFacts{}, fmt.Errorf("patches: decode the update record: %w", err)
	}
	return patchFacts{Source: "Windows servicing record", Applied: applied, Horizon: oldest(applied)}, nil
}
