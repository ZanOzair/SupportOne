package temp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

var now = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

// tempDir builds a temporary directory whose entries have known ages.
func tempDir(t *testing.T, ages map[string]time.Duration) string {
	t.Helper()

	dir := t.TempDir()
	for name, age := range ages {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		stamp := now.Add(-age)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatalf("set times on %s: %v", name, err)
		}
	}
	return dir
}

func newFix(dir string) *Fix {
	return &Fix{Dir: dir, Now: func() time.Time { return now }}
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func has(list []string, want string) bool {
	for _, got := range list {
		if got == want {
			return true
		}
	}
	return false
}

func TestApplyMovesOnlyEntriesOldEnoughToBeLitter(t *testing.T) {
	dir := tempDir(t, map[string]time.Duration{
		"ancient.tmp": 30 * 24 * time.Hour,
		"stale.log":   8 * 24 * time.Hour,
		"fresh.tmp":   time.Minute,
		"yesterday":   24 * time.Hour,
	})

	f := newFix(dir)
	if err := f.Preflight(context.Background()); err != nil {
		t.Fatalf("Preflight: %v", err)
	}

	outcome, err := f.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !outcome.Applied {
		t.Error("Outcome.Applied = false after moving files")
	}

	left := names(t, dir)
	// A file an application touched a minute ago is a file it is probably
	// still using.
	for _, keep := range []string{"fresh.tmp", "yesterday"} {
		if !has(left, keep) {
			t.Errorf("%s was moved, though it is newer than the age threshold", keep)
		}
	}
	for _, gone := range []string{"ancient.tmp", "stale.log"} {
		if has(left, gone) {
			t.Errorf("%s is still in place, though it is old enough to clear", gone)
		}
	}
	if f.Held() != 2 {
		t.Errorf("Held = %d, want 2", f.Held())
	}
}

func TestRollbackRestoresEveryFileWithItsContents(t *testing.T) {
	dir := tempDir(t, map[string]time.Duration{
		"one.tmp":   30 * 24 * time.Hour,
		"two.tmp":   20 * 24 * time.Hour,
		"three.tmp": 10 * 24 * time.Hour,
	})

	before := make(map[string][]byte)
	for _, name := range names(t, dir) {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		before[name] = raw
	}

	f := newFix(dir)
	if _, err := f.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// This is the gate the whole design rests on: after a rollback the
	// machine is as it was.
	if err := f.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	for name, want := range before {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read restored %s: %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s = %q after rollback, want %q", name, got, want)
		}
	}
	if f.Held() != 0 {
		t.Errorf("Held = %d after rollback, want 0", f.Held())
	}
}

func TestRollbackLeavesNothingBehindInTheTemporaryDirectory(t *testing.T) {
	dir := tempDir(t, map[string]time.Duration{"old.tmp": 30 * 24 * time.Hour})

	f := newFix(dir)
	if _, err := f.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := f.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	for _, name := range names(t, dir) {
		if strings.HasPrefix(name, quarantineName) {
			continue // the empty holding root is removed by the next run
		}
		if name != "old.tmp" {
			t.Errorf("unexpected entry %q left in the temporary directory", name)
		}
	}
}

func TestNothingToClearIsRefusedRatherThanReportedAsWork(t *testing.T) {
	dir := tempDir(t, map[string]time.Duration{"fresh.tmp": time.Minute})

	f := newFix(dir)
	err := f.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight offered a change that would move nothing")
	}
	if err.Error() != KeyBlockedNothing {
		t.Errorf("Preflight error = %q, want the message key %q", err, KeyBlockedNothing)
	}

	if _, err := f.Apply(context.Background()); err == nil {
		t.Fatal("Apply reported success for moving nothing")
	}
}

func TestAnUnreadableDirectoryIsReported(t *testing.T) {
	f := newFix(filepath.Join(t.TempDir(), "does-not-exist"))

	err := f.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight succeeded on a directory that is not there")
	}
	if !strings.Contains(err.Error(), KeyBlockedUnreadable) {
		t.Errorf("Preflight error = %q, want it to carry %q", err, KeyBlockedUnreadable)
	}
}

func TestTheHoldingDirectoryIsNeverClearedIntoItself(t *testing.T) {
	dir := tempDir(t, map[string]time.Duration{"old.tmp": 30 * 24 * time.Hour})

	// An earlier run's quarantine, old enough that the age filter alone would
	// take it.
	held := filepath.Join(dir, quarantineName)
	if err := os.MkdirAll(held, 0o700); err != nil {
		t.Fatalf("create holding directory: %v", err)
	}
	stamp := now.Add(-90 * 24 * time.Hour)
	if err := os.Chtimes(held, stamp, stamp); err != nil {
		t.Fatalf("age the holding directory: %v", err)
	}

	f := newFix(dir)
	candidates, err := f.candidates(context.Background())
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	for _, path := range candidates {
		if strings.Contains(filepath.Base(path), quarantineName) {
			t.Fatalf("the fix would move its own holding area: %s", path)
		}
	}
	if len(candidates) != 1 {
		t.Errorf("candidates = %v, want just the one aged file", candidates)
	}
}

func TestApplyLeavesAloneAndCountsWhatItCannotMove(t *testing.T) {
	dir := tempDir(t, map[string]time.Duration{
		"movable.tmp": 30 * 24 * time.Hour,
		"in-use.tmp":  30 * 24 * time.Hour,
	})

	f := newFix(dir)
	f.take = func(q *fixes.Quarantine, path string) error {
		if filepath.Base(path) == "in-use.tmp" {
			return errors.New("the file is open in another program")
		}
		return q.Take(path)
	}

	outcome, err := f.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// A temporary file in use is the normal case, not a failure: the fix does
	// what it can and says what it left.
	if !outcome.Applied {
		t.Error("Outcome.Applied = false though one file was moved")
	}
	// One file moved, so the singular variant: a report that says "1 files"
	// tells the user the tool is not paying attention.
	if want := fixes.PluralKey(KeyOutcomeInUse, 1); outcome.Detail != want {
		t.Errorf("Detail = %q, want %q", outcome.Detail, want)
	}
	if len(outcome.DetailArgs) != 2 || outcome.DetailArgs[0] != 1 || outcome.DetailArgs[1] != 1 {
		t.Errorf("DetailArgs = %v, want one moved and one left alone", outcome.DetailArgs)
	}

	// The file it could not take is exactly where it was.
	if _, err := os.Stat(filepath.Join(dir, "in-use.tmp")); err != nil {
		t.Errorf("the file that could not be moved is gone: %v", err)
	}

	// And the rollback still returns the one it did take.
	if err := f.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "movable.tmp")); err != nil {
		t.Errorf("the moved file was not restored: %v", err)
	}
}

func TestRollbackWithoutApplyIsHarmless(t *testing.T) {
	// The interface can offer "put it back" without first proving something
	// was taken.
	if err := newFix(t.TempDir()).Rollback(context.Background()); err != nil {
		t.Errorf("Rollback before Apply: %v", err)
	}
}

func TestApplyStopsWhenItsContextIsCancelled(t *testing.T) {
	dir := tempDir(t, map[string]time.Duration{"old.tmp": 30 * 24 * time.Hour})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := newFix(dir).Apply(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Apply err = %v, want context.Canceled", err)
	}
}

func TestTheFixDescribesItselfWellEnoughToBeRegistered(t *testing.T) {
	registry := fixes.NewRegistry()
	if err := registry.Register(New()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := registry.Get(ID)
	if !ok {
		t.Fatalf("the fix is not reachable by its own ID %q", ID)
	}
	if !fixes.RunsOn(got, platform.Current()) {
		t.Errorf("the fix does not declare %s, though it needs nothing platform-specific", platform.Current())
	}
	if got.RequiresAdmin() {
		t.Error("RequiresAdmin = true for a fix that touches only the user's own files")
	}
	if !got.Reversible() {
		t.Error("Reversible = false for a fix whose rollback is tested above")
	}

	e := got.Describe()
	if e.Undo == "" {
		t.Error("the explanation does not say how the change is undone")
	}
	for _, key := range append([]string{e.Summary, e.Undo}, e.Changes...) {
		if !strings.HasPrefix(key, "fix.") {
			t.Errorf("%q is not a message key; explanations carry keys, not prose", key)
		}
	}
}
