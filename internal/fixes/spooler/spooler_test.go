package spooler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// recorder captures the service commands without running any of them, so the
// whole fix can be exercised on any machine.
type recorder struct {
	calls    []string
	stopErr  error
	startErr error
}

func (r *recorder) run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.calls = append(r.calls, call)

	switch {
	case strings.Contains(call, "stop") && r.stopErr != nil:
		return nil, r.stopErr
	case strings.Contains(call, "start") && r.startErr != nil:
		return nil, r.startErr
	}
	return nil, nil
}

// queue builds a spool directory holding the given job files.
func queue(t *testing.T, jobs map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range jobs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// jobsIn reads back the queue, ignoring the holding directory.
func jobsIn(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		out[e.Name()] = string(raw)
	}
	return out
}

func TestApplyClearsTheQueueBetweenStoppingAndStarting(t *testing.T) {
	dir := queue(t, map[string]string{
		"00001.SPL": "job one",
		"00001.SHD": "job one header",
		"00002.SPL": "job two",
	})

	rec := &recorder{}
	f := &Fix{SpoolDir: dir, run: rec.run}

	if err := f.Preflight(context.Background()); err != nil {
		t.Fatalf("Preflight: %v", err)
	}

	outcome, err := f.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !outcome.Applied {
		t.Error("Outcome.Applied = false after clearing the queue")
	}
	if f.Held() != 3 {
		t.Errorf("Held = %d, want 3", f.Held())
	}
	if left := jobsIn(t, dir); len(left) != 0 {
		t.Errorf("jobs left in the queue: %v", left)
	}

	// The order matters: files in a running spooler's directory are in use.
	want := []string{"net stop spooler", "net start spooler"}
	if len(rec.calls) != len(want) {
		t.Fatalf("commands = %v, want %v", rec.calls, want)
	}
	for i := range want {
		if rec.calls[i] != want[i] {
			t.Errorf("command %d = %q, want %q", i, rec.calls[i], want[i])
		}
	}
}

func TestRollbackPutsEveryQueuedJobBack(t *testing.T) {
	before := map[string]string{
		"00001.SPL": "the document that jammed",
		"00001.SHD": "its header",
		"00002.SPL": "the one behind it",
	}
	dir := queue(t, before)

	rec := &recorder{}
	f := &Fix{SpoolDir: dir, run: rec.run}

	if _, err := f.Apply(context.Background()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := f.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// The gate: the queue is exactly what it was, jammed document included.
	got := jobsIn(t, dir)
	if len(got) != len(before) {
		t.Fatalf("queue holds %d jobs after rollback, want %d", len(got), len(before))
	}
	for name, content := range before {
		if got[name] != content {
			t.Errorf("%s = %q after rollback, want %q", name, got[name], content)
		}
	}
	if f.Held() != 0 {
		t.Errorf("Held = %d after rollback, want 0", f.Held())
	}

	// The service is stopped and started for the rollback too: putting files
	// back under a running spooler is the same problem in reverse.
	want := []string{"net stop spooler", "net start spooler", "net stop spooler", "net start spooler"}
	if len(rec.calls) != len(want) {
		t.Fatalf("commands = %v, want %v", rec.calls, want)
	}
}

func TestTheServiceIsStartedAgainEvenWhenTheWorkFails(t *testing.T) {
	dir := queue(t, map[string]string{"00001.SPL": "job"})

	// A file where the holding directory needs to be: the fix cannot make its
	// quarantine, and fails after the service is already stopped.
	if err := os.WriteFile(filepath.Join(dir, quarantineName), nil, 0o600); err != nil {
		t.Fatalf("block the holding directory: %v", err)
	}

	rec := &recorder{}
	f := &Fix{SpoolDir: dir, run: rec.run}

	if _, err := f.Apply(context.Background()); err == nil {
		t.Fatal("Apply reported success though it could not make its holding directory")
	}
	// A machine left unable to print at all is worse than the jam.
	if last := rec.calls[len(rec.calls)-1]; last != "net start spooler" {
		t.Errorf("commands = %v, want the service started again after the failure", rec.calls)
	}
}

func TestAQueueIsNeverLeftHalfCleared(t *testing.T) {
	before := map[string]string{
		"00001.SPL": "the first job",
		"00002.SPL": "the one that will not move",
		"00003.SPL": "the third job",
	}
	dir := queue(t, before)

	rec := &recorder{}
	f := &Fix{SpoolDir: dir, run: rec.run}
	f.take = func(q *fixes.Quarantine, path string) error {
		if filepath.Base(path) == "00002.SPL" {
			return errors.New("the file is locked")
		}
		return q.Take(path)
	}

	if _, err := f.Apply(context.Background()); err == nil {
		t.Fatal("Apply reported success though one job could not be moved")
	}
	if f.Held() != 0 {
		t.Errorf("Held = %d after a failed apply, want nothing still set aside", f.Held())
	}

	// Half a queue is not a state the user asked for: what was taken goes back.
	got := jobsIn(t, dir)
	for name, content := range before {
		if got[name] != content {
			t.Errorf("%s = %q after the failure, want the queue untouched (%q)", name, got[name], content)
		}
	}
	if last := rec.calls[len(rec.calls)-1]; last != "net start spooler" {
		t.Errorf("commands = %v, want the service started again", rec.calls)
	}
}

func TestNothingIsTouchedWhenTheServiceWillNotStop(t *testing.T) {
	dir := queue(t, map[string]string{"00001.SPL": "job"})

	rec := &recorder{stopErr: errors.New("access is denied")}
	f := &Fix{SpoolDir: dir, run: rec.run}

	if _, err := f.Apply(context.Background()); err == nil {
		t.Fatal("Apply proceeded though the print service would not stop")
	}
	if got := jobsIn(t, dir); len(got) != 1 {
		t.Errorf("the queue was touched with the service still running: %v", got)
	}
	if len(rec.calls) != 1 {
		t.Errorf("commands = %v, want only the failed stop", rec.calls)
	}
}

func TestAnEmptyQueueIsRefusedRatherThanRestartedForNothing(t *testing.T) {
	rec := &recorder{}
	f := &Fix{SpoolDir: t.TempDir(), run: rec.run}

	err := f.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight offered to clear a queue that is already empty")
	}
	if err.Error() != KeyBlockedEmpty {
		t.Errorf("Preflight error = %q, want the message key %q", err, KeyBlockedEmpty)
	}

	if _, err := f.Apply(context.Background()); err == nil {
		t.Fatal("Apply restarted the print service to move nothing")
	}
	if len(rec.calls) != 0 {
		t.Errorf("commands = %v, want none for an empty queue", rec.calls)
	}
}

func TestAMissingSpoolDirectoryIsReported(t *testing.T) {
	f := &Fix{SpoolDir: filepath.Join(t.TempDir(), "not-there"), run: (&recorder{}).run}

	err := f.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight succeeded with no spool directory at all")
	}
	if !strings.Contains(err.Error(), KeyBlockedUnreadable) {
		t.Errorf("Preflight error = %q, want it to carry %q", err, KeyBlockedUnreadable)
	}
}

func TestTheHoldingDirectoryIsNeverQueuedAsAJob(t *testing.T) {
	dir := queue(t, map[string]string{"00001.SPL": "job"})
	if err := os.MkdirAll(filepath.Join(dir, quarantineName+"-old"), 0o700); err != nil {
		t.Fatalf("create an earlier holding directory: %v", err)
	}

	f := &Fix{SpoolDir: dir, run: (&recorder{}).run}
	queued, err := f.queued()
	if err != nil {
		t.Fatalf("queued: %v", err)
	}
	if len(queued) != 1 {
		t.Errorf("queued = %v, want just the one job", queued)
	}
}

func TestRollbackWithoutApplyIsHarmless(t *testing.T) {
	rec := &recorder{}
	f := &Fix{SpoolDir: t.TempDir(), run: rec.run}

	if err := f.Rollback(context.Background()); err != nil {
		t.Errorf("Rollback before Apply: %v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("commands = %v, want none when there is nothing to put back", rec.calls)
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
	if !fixes.RunsOn(got, platform.Windows) {
		t.Error("the fix does not declare Windows, the only platform it targets")
	}
	if fixes.RunsOn(got, platform.Linux) || fixes.RunsOn(got, platform.Darwin) {
		t.Error("the fix claims a CUPS platform, whose spool directory it does not understand")
	}
	if !got.RequiresAdmin() {
		t.Error("RequiresAdmin = false for a fix that stops a system service")
	}
	if !got.Reversible() {
		t.Error("Reversible = false for a fix whose rollback is tested above")
	}

	e := got.Describe()
	for _, key := range append([]string{e.Summary, e.Undo}, e.Changes...) {
		if !strings.HasPrefix(key, "fix.") {
			t.Errorf("%q is not a message key; explanations carry keys, not prose", key)
		}
	}
}
