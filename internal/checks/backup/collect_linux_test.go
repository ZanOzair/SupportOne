package backup

import (
	"context"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

// TestLinuxIsHonestAboutNotLooking lives in a file the Linux build
// constrains, because it calls the collector directly and only Linux's takes
// no runner — the Windows and macOS collectors would dereference a nil one.
func TestLinuxIsHonestAboutNotLooking(t *testing.T) {
	facts, err := collectBackup(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectBackup: %v", err)
	}
	if facts.Supported {
		t.Fatal("Supported = true; Linux has no backup mechanism this check reads")
	}

	// "I did not look" is unknown, not a warning: it is not a finding about
	// the user's backups at all.
	got := backupVerdict(facts, now)
	if got.Severity != checks.SeverityUnknown {
		t.Errorf("severity = %q, want unknown", got.Severity)
	}
	if got.Summary != keyBackupNotApplicable {
		t.Errorf("summary = %q, want %q", got.Summary, keyBackupNotApplicable)
	}
}
