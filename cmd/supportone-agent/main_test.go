package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "supportone-agent") {
		t.Errorf("stdout = %q, want build information", stdout.String())
	}
}

func TestConflictingOutputFlagsAreRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--json", "--text"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("run succeeded, want an error when two outputs are requested")
	}
}

func TestUnknownArgumentIsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"snapshot"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("run succeeded, want error for unexpected argument")
	}
}

func TestJSONSnapshotIsWellFormed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--json", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	var snap checks.Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snap); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v\n%s", err, stdout.String())
	}
	if snap.Schema != checks.SnapshotSchema {
		t.Errorf("schema = %d, want %d", snap.Schema, checks.SnapshotSchema)
	}
	if snap.GeneratedAt.IsZero() {
		t.Error("generated_at is not set")
	}
}

func TestSnapshotWritesAuditTrail(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.log")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--text", "--audit-log", auditPath, "--lang", "en"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries := readFile(t, auditPath)
	for _, want := range []string{"AGENT_START", "AGENT_STOP"} {
		if !strings.Contains(entries, want) {
			t.Errorf("audit log is missing %s:\n%s", want, entries)
		}
	}
}

func TestDryRunSaysNothingWillChange(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--text", "--dry-run", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "nothing on this computer will be changed") {
		t.Errorf("dry run output does not state that nothing changes:\n%s", stdout.String())
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// The tests below exercise the terminal repair flow against the real
// compiled-in registries. They use temp.clear, whose preflight refuses when
// there is nothing to clear, so nothing on the machine running the tests is
// changed by them.

func TestListFixesNamesWhatEachOneChanges(t *testing.T) {
	var stdout, stderr bytes.Buffer

	args := []string{"--list-fixes", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "temp.clear") {
		t.Errorf("the listing does not name temp.clear:\n%s", out)
	}
	// A listing that gives IDs and no explanation is a listing nobody can act
	// on.
	if !strings.Contains(out, "temporary files") {
		t.Errorf("the listing does not say what the repair does:\n%s", out)
	}
}

func TestListWizardsNamesTheProblemsTheyAreFor(t *testing.T) {
	var stdout, stderr bytes.Buffer

	args := []string{"--list-wizards", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "wizard.connection") {
		t.Errorf("the listing does not name the connection walkthrough:\n%s", stdout.String())
	}
}

func TestAFixIsDescribedBeforeAnythingIsAsked(t *testing.T) {
	stale := staleTempDir(t)

	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "temp.clear", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	// Empty input: the prompt gets nothing, and silence is not consent.
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"This will change:", "To undo:", "Nothing was changed."} {
		if !strings.Contains(out, want) {
			t.Errorf("the output is missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("the file was moved without a confirmation: %v", err)
	}
}

func TestAFixIsNotAppliedOnAnythingButItsOwnID(t *testing.T) {
	stale := staleTempDir(t)

	for _, answer := range []string{"y\n", "yes\n", "\n", "temp clear\n", "TEMP.CLEARED\n"} {
		var stdout, stderr bytes.Buffer
		args := []string{"--fix", "temp.clear", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

		if err := run(args, strings.NewReader(answer), &stdout, &stderr); err != nil {
			t.Fatalf("run with %q: %v", answer, err)
		}
		if !strings.Contains(stdout.String(), "Nothing was changed.") {
			t.Errorf("answering %q did not abort:\n%s", answer, stdout.String())
		}
		if _, err := os.Stat(stale); err != nil {
			t.Fatalf("answering %q moved the file: %v", answer, err)
		}
	}
}

func TestADryRunDescribesTheChangeAndMakesNone(t *testing.T) {
	stale := staleTempDir(t)

	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "temp.clear", "--dry-run", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	// Even typing the ID cannot make a dry run change anything: it is never
	// asked for.
	if err := run(args, strings.NewReader("temp.clear\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "nothing on this computer will be changed") {
		t.Errorf("the output does not say it is a dry run:\n%s", stdout.String())
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("a dry run moved the file: %v", err)
	}
}

func TestConfirmingByIDAppliesTheFixAndUndoPutsItBack(t *testing.T) {
	// This is the only test here that actually changes anything, so it runs
	// only where the restore mechanism provably creates nothing. On Windows
	// or macOS an available restore point would mean a real checkpoint or
	// snapshot on the machine running the tests, which no test should make.
	if runtime.GOOS != "linux" {
		t.Skip("this test applies a fix; it runs only where no restore point would be created")
	}

	stale := staleTempDir(t)

	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "temp.clear", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	// Linux has no restore point, so the second question is asked too, and
	// then the offer to undo.
	input := strings.NewReader("temp.clear\nyes\nundo\n")
	if err := run(args, input, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "There is no restore point") {
		t.Errorf("the user was not told there would be no restore point:\n%s", out)
	}
	if !strings.Contains(out, "The change was undone.") {
		t.Errorf("the undo was not reported:\n%s", out)
	}

	// The file is back, with what was in it.
	raw, err := os.ReadFile(stale)
	if err != nil {
		t.Fatalf("the file was not restored: %v", err)
	}
	if string(raw) != "litter" {
		t.Errorf("the restored file holds %q, want %q", raw, "litter")
	}
}

func TestAFixWithNothingToDoSaysSoRatherThanRunning(t *testing.T) {
	emptyTempDir(t)

	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "temp.clear", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	if err := run(args, strings.NewReader("temp.clear\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "will not run right now") {
		t.Errorf("the output does not say the repair was refused:\n%s", stdout.String())
	}
}

func TestAnIDThatIsNotCompiledInIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "rm.everything", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("run accepted a fix ID that is not in the registry")
	}
}

func TestFixAndWizardAreNotAskedForTogether(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--fix", "temp.clear", "--wizard", "wizard.connection"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("run accepted --fix and --wizard together")
	}
}

func TestAWizardRunsToTheEndAndHandsOver(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--wizard", "wizard.connection", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	// Every prompt gets an empty line, so nothing is changed whatever the
	// machine running the tests looks like.
	if err := run(args, strings.NewReader("\n\n\n\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "What was checked:") && !strings.Contains(out, "came back clean") {
		t.Errorf("the walkthrough did not hand over what it found:\n%s", out)
	}
}

func TestAWizardThatDoesNotRunHereIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--wizard", "wizard.nope", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("run accepted a walkthrough ID that is not in the registry")
	}
}

// staleTempDir points this process's temporary directory at a fresh one
// holding a single file old enough for temp.clear to move, and returns that
// file's path. All three variables are set because each platform reads a
// different one.
func staleTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, dir)
	}

	path := filepath.Join(dir, "old.tmp")
	if err := os.WriteFile(path, []byte("litter"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return path
}

// emptyTempDir points this process's temporary directory at one with nothing
// in it.
func emptyTempDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, dir)
	}
}
