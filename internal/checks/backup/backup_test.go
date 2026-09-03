package backup

import (
	"context"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

var now = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) time.Time { return now.Add(-d) }

func TestBackupVerdict(t *testing.T) {
	cases := []struct {
		name     string
		facts    backupFacts
		severity checks.Severity
		summary  string
	}{
		{
			"backed up this morning",
			backupFacts{Supported: true, Mechanism: "Time Machine", Configured: true, Last: ago(3 * time.Hour)},
			checks.SeverityOK, keyBackupOK,
		},
		{
			"just inside the fresh window",
			backupFacts{Supported: true, Mechanism: "Time Machine", Configured: true, Last: ago(6 * 24 * time.Hour)},
			checks.SeverityOK, keyBackupOK,
		},
		{
			"just outside it",
			backupFacts{Supported: true, Mechanism: "Time Machine", Configured: true, Last: ago(8 * 24 * time.Hour)},
			checks.SeverityAttention, keyBackupStale,
		},
		{
			"more than a month",
			backupFacts{Supported: true, Mechanism: "Time Machine", Configured: true, Last: ago(45 * 24 * time.Hour)},
			checks.SeverityUrgent, keyBackupVeryStale,
		},
		{
			"set up but never run",
			backupFacts{Supported: true, Mechanism: "Time Machine", Configured: true},
			checks.SeverityAttention, keyBackupNeverRun,
		},
		{
			"nothing configured",
			backupFacts{Supported: true, Mechanism: "Volume Shadow Copy"},
			checks.SeverityAttention, keyBackupNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := backupVerdict(tc.facts, now)
			if got.Severity != tc.severity {
				t.Errorf("severity = %q, want %q", got.Severity, tc.severity)
			}
			if got.Summary != tc.summary {
				t.Errorf("summary = %q, want %q", got.Summary, tc.summary)
			}
		})
	}
}

// TestAPlatformWeDidNotLookAtIsUnknownNotAWarning is the point of the
// Supported flag: "I did not look" and "I looked and found nothing" are
// different answers, and only one of them is about the user's backups.
func TestAPlatformWeDidNotLookAtIsUnknownNotAWarning(t *testing.T) {
	got := backupVerdict(backupFacts{Supported: false}, now)

	if got.Severity != checks.SeverityUnknown {
		t.Errorf("severity = %q, want unknown", got.Severity)
	}
	if got.Summary != keyBackupNotApplicable {
		t.Errorf("summary = %q, want %q", got.Summary, keyBackupNotApplicable)
	}
}

func TestAFutureBackupDateIsNotBelieved(t *testing.T) {
	facts := backupFacts{
		Supported: true, Mechanism: "Volume Shadow Copy", Configured: true,
		Last: now.Add(48 * time.Hour),
	}

	got := backupVerdict(facts, now)
	// Reporting a backup from the future as fresh would be reporting a number
	// we do not believe.
	if got.Severity != checks.SeverityUnknown {
		t.Errorf("severity = %q, want unknown", got.Severity)
	}
	if got.Summary != keyBackupUnreadable {
		t.Errorf("summary = %q, want %q", got.Summary, keyBackupUnreadable)
	}
}

func TestTheVerdictCarriesItsEvidence(t *testing.T) {
	facts := backupFacts{
		Supported: true, Mechanism: "Time Machine", Configured: true,
		Last: ago(2 * time.Hour), Destination: "Backup Drive",
	}

	got := backupVerdict(facts, now)
	for _, key := range []string{"mechanism", "configured", "destination", "last_backup"} {
		if _, ok := got.Detail[key]; !ok {
			t.Errorf("the evidence does not carry %q: %v", key, got.Detail)
		}
	}
}

func TestLinuxIsHonestAboutNotLooking(t *testing.T) {
	// Only meaningful on Linux, where collect_linux.go is the compiled
	// collector; elsewhere this is a no-op assertion about a real mechanism.
	facts, err := collectBackup(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectBackup: %v", err)
	}
	if facts.Supported {
		t.Skip("this platform has a backup mechanism this check reads")
	}
	if got := backupVerdict(facts, now); got.Severity != checks.SeverityUnknown {
		t.Errorf("severity = %q, want unknown", got.Severity)
	}
}

func TestTheCheckDeclaresItself(t *testing.T) {
	c := statusCheck{}
	if c.ID() != "backup.status" {
		t.Errorf("ID = %q", c.ID())
	}
	if c.RequiresAdmin() {
		t.Error("RequiresAdmin = true; the mechanisms this reads answer without elevation")
	}
	if len(c.Platforms()) != 3 {
		t.Errorf("Platforms = %v, want all three — Linux answers that it did not look", c.Platforms())
	}
}
