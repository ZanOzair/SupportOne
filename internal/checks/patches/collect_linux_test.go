package patches

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// logTree writes a recorded /var/log and points the collector at it.
func logTree(t *testing.T, files map[string]string) {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	previous := logRoot
	logRoot = dir
	t.Cleanup(func() { logRoot = previous })
}

func TestCollectPatchesReadsEveryDpkgLogOnDisk(t *testing.T) {
	logTree(t, map[string]string{
		"dpkg.log": "2026-08-14 09:31:02 install curl:amd64 <none> 8.5.0-1\n",
		// A rotated log still on disk is part of the record, and reading it
		// is what makes the horizon honest.
		"dpkg.log.1": "2026-06-01 08:00:00 upgrade libc6:amd64 2.38-1 2.39-1\n",
	})

	got, err := collectPatches(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectPatches: %v", err)
	}
	if got.Source != "dpkg log" {
		t.Errorf("Source = %q", got.Source)
	}
	if len(got.Applied) != 2 {
		t.Fatalf("read %d entries, want 2 across both logs", len(got.Applied))
	}
	if got.Horizon.IsZero() {
		t.Error("the record's horizon was not reported")
	}
	if got.Horizon.Year() != 2026 || got.Horizon.Month() != 6 {
		t.Errorf("Horizon = %v, want the oldest entry", got.Horizon)
	}
}

func TestALogThatRecordsNothingIsStillARecord(t *testing.T) {
	logTree(t, map[string]string{"dpkg.log": "2026-08-14 09:31:02 startup archives unpack\n"})

	got, err := collectPatches(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectPatches: %v", err)
	}
	// The distinction the verdict rests on: a record that exists and is
	// empty is a real answer, not a missing one.
	if got.Source == "" {
		t.Error("Source is empty though a log was read")
	}
	if len(got.Applied) != 0 {
		t.Errorf("Applied = %v, want none", got.Applied)
	}
}

func TestWithNoDpkgLogItFallsBackToRPM(t *testing.T) {
	logTree(t, nil)

	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("curl-8.5.0-1.fc40.x86_64   Thu 14 Aug 2026 09:31:02 AM UTC\n"), nil
	}

	got, err := collectPatches(context.Background(), run)
	if err != nil {
		t.Fatalf("collectPatches: %v", err)
	}
	if got.Source != "rpm database" {
		t.Errorf("Source = %q, want the rpm fallback", got.Source)
	}
	if len(got.Applied) != 1 {
		t.Errorf("Applied = %v, want the one package", got.Applied)
	}
}

func TestNoRecordAtAllIsReportedRatherThanGuessed(t *testing.T) {
	logTree(t, nil)

	run := func(context.Context, string, ...string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	if _, err := collectPatches(context.Background(), run); err == nil {
		t.Error("collectPatches succeeded with no record of any kind")
	}
}
