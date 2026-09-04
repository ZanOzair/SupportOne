package patches

import (
	"context"
	"fmt"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

const softwareUpdateExe = "softwareupdate"

// collectPatches reads macOS's own install history.
//
// `--history` reads a local record and makes no network request; `--list`,
// which does go online, is deliberately not used. The record covers system
// updates, not App Store applications, which is why the source is named.
func collectPatches(ctx context.Context, run platform.Runner) (patchFacts, error) {
	raw, err := run(ctx, softwareUpdateExe, "--history")
	if err != nil {
		return patchFacts{}, fmt.Errorf("patches: read the update record: %w", err)
	}

	applied := parseSoftwareUpdateHistory(raw)
	return patchFacts{Source: "macOS update history", Applied: applied, Horizon: oldest(applied)}, nil
}
