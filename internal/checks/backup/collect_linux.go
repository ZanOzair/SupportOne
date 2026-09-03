package backup

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// collectBackup reports that Linux has no backup mechanism this check reads.
//
// There is no equivalent every distribution ships. Timeshift, Déjà Dup, Borg,
// restic, rsnapshot and a hand-written rsync cron job are all common, and each
// records its state somewhere different. Guessing at one and reporting "no
// backup" when another is running perfectly would be worse than saying plainly
// that this check does not know how to look here.
func collectBackup(context.Context, platform.Runner) (backupFacts, error) {
	return backupFacts{Supported: false}, nil
}
