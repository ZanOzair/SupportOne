package fixes

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFile creates a file with content and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestQuarantineRestorePutsEveryFileBack(t *testing.T) {
	work := t.TempDir()
	source := filepath.Join(work, "temp")
	holding := filepath.Join(work, "quarantine")

	want := map[string]string{
		"one.tmp":            "first",
		"two.log":            "second",
		"nested/three.cache": "third",
	}
	for name, content := range want {
		writeFile(t, source, name, content)
	}

	q, err := NewQuarantine(holding, "temp.clear")
	if err != nil {
		t.Fatalf("NewQuarantine: %v", err)
	}

	for name := range want {
		if err := q.Take(filepath.Join(source, name)); err != nil {
			t.Fatalf("Take %s: %v", name, err)
		}
	}

	if q.Count() != len(want) {
		t.Errorf("Count = %d, want %d", q.Count(), len(want))
	}

	// The point of the fix: the files are gone from where they were.
	for name := range want {
		if _, err := os.Lstat(filepath.Join(source, name)); !os.IsNotExist(err) {
			t.Errorf("%s is still in place after being quarantined (err=%v)", name, err)
		}
	}

	// The point of the rollback: they come back, with their contents.
	if err := q.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatalf("read restored %s: %v", name, err)
		}
		if string(got) != content {
			t.Errorf("%s = %q after restore, want %q", name, got, content)
		}
	}

	if q.Count() != 0 {
		t.Errorf("Count = %d after Restore, want 0", q.Count())
	}
	if _, err := os.Lstat(q.Dir); !os.IsNotExist(err) {
		t.Errorf("quarantine directory %s still exists after an empty restore (err=%v)", q.Dir, err)
	}
}

func TestQuarantineRestoreKeepsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}

	work := t.TempDir()
	path := writeFile(t, work, "secret.conf", "value")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	q, err := NewQuarantine(filepath.Join(work, "q"), "temp.clear")
	if err != nil {
		t.Fatalf("NewQuarantine: %v", err)
	}
	if err := q.Take(path); err != nil {
		t.Fatalf("Take: %v", err)
	}
	if err := q.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat restored file: %v", err)
	}
	// Restoring a file must not widen who can read it.
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("restored permissions = %v, want %v", got, os.FileMode(0o600))
	}
}

func TestQuarantineKeepsFilesWithTheSameName(t *testing.T) {
	work := t.TempDir()
	a := writeFile(t, filepath.Join(work, "a"), "cache.tmp", "from a")
	b := writeFile(t, filepath.Join(work, "b"), "cache.tmp", "from b")

	q, err := NewQuarantine(filepath.Join(work, "q"), "temp.clear")
	if err != nil {
		t.Fatalf("NewQuarantine: %v", err)
	}
	for _, path := range []string{a, b} {
		if err := q.Take(path); err != nil {
			t.Fatalf("Take %s: %v", path, err)
		}
	}

	// Two files sharing a name must not overwrite each other in the holding
	// directory: one of them would be lost, and the rollback would be a lie.
	if err := q.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	for path, want := range map[string]string{a: "from a", b: "from b"} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestQuarantineTakeReportsAMissingFile(t *testing.T) {
	work := t.TempDir()
	q, err := NewQuarantine(filepath.Join(work, "q"), "temp.clear")
	if err != nil {
		t.Fatalf("NewQuarantine: %v", err)
	}

	// A fix that quietly skipped what it could not move would report success
	// for work it did not do.
	if err := q.Take(filepath.Join(work, "not-there.tmp")); err == nil {
		t.Fatal("Take of a missing path returned no error")
	}
	if q.Count() != 0 {
		t.Errorf("Count = %d after a failed Take, want 0", q.Count())
	}
}

func TestQuarantinePathsListsWhatIsHeld(t *testing.T) {
	work := t.TempDir()
	source := filepath.Join(work, "temp")
	first := writeFile(t, source, "b.tmp", "b")
	second := writeFile(t, source, "a.tmp", "a")

	q, err := NewQuarantine(filepath.Join(work, "q"), "temp.clear")
	if err != nil {
		t.Fatalf("NewQuarantine: %v", err)
	}
	for _, path := range []string{first, second} {
		if err := q.Take(path); err != nil {
			t.Fatalf("Take: %v", err)
		}
	}

	got := q.Paths()
	want := []string{second, first} // sorted: a.tmp before b.tmp
	if len(got) != len(want) {
		t.Fatalf("Paths returned %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Paths[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestQuarantineRestoreReportsWhatItCouldNotPutBack(t *testing.T) {
	work := t.TempDir()
	path := writeFile(t, filepath.Join(work, "temp"), "one.tmp", "content")

	q, err := NewQuarantine(filepath.Join(work, "q"), "temp.clear")
	if err != nil {
		t.Fatalf("NewQuarantine: %v", err)
	}
	if err := q.Take(path); err != nil {
		t.Fatalf("Take: %v", err)
	}

	// Something else removed the held file. A partial restore the user is told
	// about beats a silent one.
	held := filepath.Join(q.Dir, "one.tmp")
	if err := os.Remove(held); err != nil {
		t.Fatalf("remove held file: %v", err)
	}

	err = q.Restore()
	if err == nil {
		t.Fatal("Restore reported success after losing a file")
	}
	if !strings.Contains(err.Error(), q.Dir) {
		t.Errorf("Restore error does not say where the files are: %v", err)
	}
}

func TestNewQuarantineNamesTheDirectoryForTheFix(t *testing.T) {
	q, err := NewQuarantine(t.TempDir(), "print.clear-spooler")
	if err != nil {
		t.Fatalf("NewQuarantine: %v", err)
	}

	name := filepath.Base(q.Dir)
	if !strings.HasPrefix(name, "print-clear-spooler-") {
		t.Errorf("quarantine directory %q is not named for the fix that made it", name)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(q.Dir)
		if err != nil {
			t.Fatalf("stat quarantine directory: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("quarantine permissions = %v, want %v", got, os.FileMode(0o700))
		}
	}
}
