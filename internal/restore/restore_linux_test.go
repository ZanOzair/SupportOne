package restore

import (
	"context"
	"strings"
	"testing"
)

func TestLinuxReportsThatItHasNoRestorePoint(t *testing.T) {
	m := New()

	got := m.Check(context.Background())
	if got.Available {
		t.Error("Available = true; Linux has no mechanism this agent can use")
	}
	if got.Reason != KeyUnavailableOnPlatform {
		t.Errorf("Reason = %q, want %q", got.Reason, KeyUnavailableOnPlatform)
	}
	if got.Kind == "" {
		t.Error("Kind is empty; the user should still be told what was looked for")
	}
}

func TestLinuxCreateFailsRatherThanPretending(t *testing.T) {
	// Claiming a restore point that does not exist would be the one thing
	// worse than having none.
	point, err := New().Create(context.Background(), "before a fix")
	if err == nil {
		t.Fatal("Create returned no error on a platform with no restore mechanism")
	}
	if point != (Point{}) {
		t.Errorf("Create returned %+v alongside its error, want a zero Point", point)
	}
	if !strings.Contains(err.Error(), "rollback") {
		t.Errorf("the error does not point at what does protect the user: %v", err)
	}
}
