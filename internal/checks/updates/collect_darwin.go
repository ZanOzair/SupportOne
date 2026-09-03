package updates

import (
	"context"

	"github.com/ZanOzair/supportone/internal/platform"
)

const defaultsExe = "defaults"

// softwareUpdatePrefs holds the record macOS keeps of its own update runs.
// Reading it stays local; `softwareupdate -l` would ask Apple, which this
// agent does not do on its own.
const softwareUpdatePrefs = "/Library/Preferences/com.apple.SoftwareUpdate"

func collectUpdates(ctx context.Context, run platform.Runner) (updateFacts, error) {
	facts := updateFacts{Pending: -1, Source: "macOS software update history"}

	out, err := run(ctx, defaultsExe, "read", softwareUpdatePrefs, "LastFullSuccessfulDate")
	if err != nil {
		// The key is absent on a machine that has never completed an update.
		return facts, nil
	}
	facts.LastInstalled = parseMacSoftwareUpdateDate(out)
	return facts, nil
}
