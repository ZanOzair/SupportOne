package backup

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

const tmutilExe = "tmutil"

// collectBackup reads Time Machine's own record.
//
// Both commands are read-only. `destinationinfo` says whether a destination is
// configured and needs no special rights; `latestbackup` needs Full Disk
// Access on recent macOS, so its absence costs the date rather than the whole
// answer.
func collectBackup(ctx context.Context, run platform.Runner) (backupFacts, error) {
	facts := backupFacts{Supported: true, Mechanism: "Time Machine"}

	raw, err := run(ctx, tmutilExe, "destinationinfo")
	if err != nil {
		// tmutil is present on every macOS; a failure here means it could not
		// answer, not that there is no backup.
		return backupFacts{Supported: true, Mechanism: "Time Machine"}, nil
	}

	facts.Configured, facts.Destination = parseDestinationInfo(raw)
	if !facts.Configured {
		return facts, nil
	}

	if latest, err := run(ctx, tmutilExe, "latestbackup"); err == nil {
		facts.Last = parseLatestBackup(latest)
	}
	return facts, nil
}
