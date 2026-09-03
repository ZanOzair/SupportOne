// Package backup reports whether this machine has a backup, and when it last
// ran.
//
// The honest limit here is larger than in most checks, and the messages say so
// rather than burying it. SupportOne looks for the backup mechanism the
// operating system itself ships — Time Machine on macOS, Volume Shadow Copy on
// Windows — and nothing else. Someone using Backblaze, Veeam, rsync to a NAS,
// or a phone photo sync has a backup this check cannot see. So a negative
// result is reported as "no backup SupportOne can see", never as "you have no
// backup": the second would be a claim this check cannot support, and telling
// someone they are unprotected when they are not is its own kind of harm.
package backup

import (
	"context"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// backupFacts is what every platform's collector produces.
type backupFacts struct {
	// Supported is false where this check knows of no mechanism to read. It
	// is not the same as finding nothing.
	Supported bool

	// Mechanism names what was looked at, so the user can tell which backup
	// the answer is about.
	Mechanism string

	// Configured says a mechanism is set up, whether or not it has run.
	Configured bool

	// Last is when the most recent backup completed. Zero means none was
	// found, or the date could not be read.
	Last time.Time

	// Destination is a volume or share name. It is a name and never a path:
	// a path can carry a username.
	Destination string
}

// Message keys for this package's results.
const (
	keyBackupOK            = "check.backup.status.ok"
	keyBackupStale         = "check.backup.status.stale"
	keyBackupVeryStale     = "check.backup.status.very_stale"
	keyBackupNeverRun      = "check.backup.status.never_run"
	keyBackupNone          = "check.backup.status.none"
	keyBackupUnreadable    = "check.backup.status.unreadable"
	keyBackupNotApplicable = "check.backup.status.not_applicable"
)

// The thresholds, written down.
const (
	// freshWindow: a backup newer than this is doing its job.
	freshWindow = 7 * 24 * time.Hour

	// staleWindow: past this, the work since the last backup is worth more
	// than most people would want to lose.
	staleWindow = 30 * 24 * time.Hour
)

type statusCheck struct {
	run platform.Runner

	// now is swappable in tests.
	now func() time.Time
}

func (statusCheck) ID() string               { return "backup.status" }
func (statusCheck) Platforms() []platform.OS { return platform.All() }
func (statusCheck) RequiresAdmin() bool      { return false }

func (c statusCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectBackup(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	return backupVerdict(facts, c.at()), nil
}

func (c statusCheck) at() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// backupVerdict decides from the facts alone.
func backupVerdict(facts backupFacts, now time.Time) checks.Result {
	// A platform with no mechanism this check reads is unknown, never OK and
	// never a warning: we did not look, so we have nothing to report.
	if !facts.Supported {
		return checks.Unknown(keyBackupNotApplicable)
	}

	detail := map[string]any{"mechanism": facts.Mechanism, "configured": facts.Configured}
	if facts.Destination != "" {
		detail["destination"] = facts.Destination
	}
	if !facts.Last.IsZero() {
		detail["last_backup"] = facts.Last.UTC().Format(time.RFC3339)
	}

	if !facts.Configured {
		return checks.Attention(keyBackupNone, facts.Mechanism).With(detail)
	}
	if facts.Last.IsZero() {
		// Set up but never run — or run and the date is unreadable. Either
		// way there is no backup we can point at.
		return checks.Attention(keyBackupNeverRun, facts.Mechanism).With(detail)
	}

	age := now.Sub(facts.Last)
	if age < 0 {
		// A future timestamp means a clock is wrong somewhere. Reporting it
		// as a fresh backup would be reporting a number we do not believe.
		return checks.Unknown(keyBackupUnreadable, facts.Mechanism).With(detail)
	}
	human := checks.HumanDuration(age)

	switch {
	case age > staleWindow:
		return checks.Urgent(keyBackupVeryStale, facts.Mechanism, human).With(detail)
	case age > freshWindow:
		return checks.Attention(keyBackupStale, facts.Mechanism, human).With(detail)
	default:
		return checks.OK(keyBackupOK, facts.Mechanism, human).With(detail)
	}
}

func init() {
	checks.MustRegister(statusCheck{run: platform.RunRead})
}
