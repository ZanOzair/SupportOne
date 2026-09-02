package consent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	t := time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func openTestLog(t *testing.T) (*Log, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "audit.log")
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	l.now = fixedClock()
	t.Cleanup(func() { _ = l.Close() })
	return l, path
}

func TestAppendWritesOneLinePerEvent(t *testing.T) {
	l, path := openTestLog(t)

	events := []Event{
		{Type: EventAgentStart},
		{Type: EventCheckRun, Subject: "os.info", Fields: map[string]string{"severity": "ok", "duration": "12ms"}},
		{Type: EventConsentGiven, Subject: "net.flush-dns"},
	}
	for _, ev := range events {
		if err := l.Append(ev); err != nil {
			t.Fatalf("Append(%s): %v", ev.Type, err)
		}
	}

	lines := readLines(t, path)
	if len(lines) != len(events) {
		t.Fatalf("got %d lines, want %d: %q", len(lines), len(events), lines)
	}
	want := "2026-09-02T13:20:00Z\tCHECK_RUN\tos.info\tduration=12ms\tseverity=ok"
	if lines[1] != want {
		t.Errorf("check line = %q, want %q", lines[1], want)
	}
}

func TestAppendIsAppendOnlyAcrossReopen(t *testing.T) {
	l, path := openTestLog(t)
	if err := l.Append(Event{Type: EventAgentStart}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	reopened.now = fixedClock()
	if err := reopened.Append(Event{Type: EventAgentStop}); err != nil {
		t.Fatalf("Append after reopen: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), lines)
	}
	if !strings.Contains(lines[0], string(EventAgentStart)) {
		t.Errorf("first entry was overwritten: %q", lines[0])
	}
}

func TestAppendEscapesControlCharacters(t *testing.T) {
	l, path := openTestLog(t)
	err := l.Append(Event{
		Type:    EventDataSent,
		Subject: "https://example.invalid\nFAKE\tENTRY",
		Fields:  map[string]string{"bytes": "42\n1970-01-01T00:00:00Z\tAGENT_START"},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	if lines := readLines(t, path); len(lines) != 1 {
		t.Fatalf("forged entries: got %d lines, want 1: %q", len(lines), lines)
	}
}

func TestOpenCreatesPrivateFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows; ACLs are checked separately")
	}
	_, path := openTestLog(t)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("audit log mode = %o, want 600", perm)
	}
}

func TestAppendIsConcurrencySafe(t *testing.T) {
	l, path := openTestLog(t)

	const writers = 20
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			if err := l.Append(Event{Type: EventCheckRun, Subject: "os.info"}); err != nil {
				t.Errorf("Append: %v", err)
			}
		}()
	}
	wg.Wait()

	if lines := readLines(t, path); len(lines) != writers {
		t.Fatalf("got %d lines, want %d", len(lines), writers)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	trimmed := strings.TrimSuffix(string(data), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
