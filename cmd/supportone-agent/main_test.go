package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--version"}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "supportone-agent") {
		t.Errorf("stdout = %q, want build information", stdout.String())
	}
}

func TestConflictingOutputFlagsAreRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--json", "--text"}, &stdout, &stderr); err == nil {
		t.Fatal("run succeeded, want an error when two outputs are requested")
	}
}

func TestUnknownArgumentIsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"snapshot"}, &stdout, &stderr); err == nil {
		t.Fatal("run succeeded, want error for unexpected argument")
	}
}

func TestJSONSnapshotIsWellFormed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--json", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, &stdout, &stderr); err != nil {
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
	if err := run([]string{"--text", "--audit-log", auditPath, "--lang", "en"}, &stdout, &stderr); err != nil {
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
	if err := run(args, &stdout, &stderr); err != nil {
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
