package fleet

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

var now = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

func snapshot(results ...checks.Result) checks.Snapshot {
	if len(results) == 0 {
		results = []checks.Result{{
			CheckID: "os.info", Severity: checks.SeverityOK, Summary: "check.os.info.ok",
		}}
	}
	return checks.Snapshot{
		Schema:       checks.SnapshotSchema,
		AgentVersion: "test",
		GeneratedAt:  now,
		Host:         platform.Host{OS: platform.Linux, Arch: "amd64"},
		Results:      results,
	}
}

func report(name string, results ...checks.Result) Report {
	return Report{Name: name, Snapshot: snapshot(results...), Redacted: true, AgentVersion: "test"}
}

func store(t *testing.T) *Store {
	t.Helper()

	s, err := OpenStore(filepath.Join(t.TempDir(), "machines"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return s
}

func TestPutAndGet(t *testing.T) {
	s := store(t)

	machine, err := s.Put(report("Reception PC"), now)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if machine.Name != "Reception PC" {
		t.Errorf("Name = %q", machine.Name)
	}
	if machine.Schema != Schema {
		t.Errorf("Schema = %d, want %d", machine.Schema, Schema)
	}

	got, err := s.Get(machine.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != machine.ID || len(got.History) != 1 {
		t.Errorf("Get returned %+v", got)
	}
}

// TestTheSameMachineUpdatesRatherThanDuplicating is what makes a fleet view a
// fleet rather than a log.
func TestTheSameMachineUpdatesRatherThanDuplicating(t *testing.T) {
	s := store(t)

	for i := 0; i < 3; i++ {
		if _, err := s.Put(report("Reception PC"), now.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
	}

	machines, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(machines) != 1 {
		t.Fatalf("List returned %d machines, want 1", len(machines))
	}
	if len(machines[0].History) != 3 {
		t.Errorf("history = %d entries, want 3", len(machines[0].History))
	}
	// Newest first: the current state is what a technician is looking at.
	if !machines[0].History[0].Received.After(machines[0].History[1].Received) {
		t.Error("history is not newest first")
	}
}

func TestNameMatchingIgnoresCaseAndSurroundingSpace(t *testing.T) {
	s := store(t)

	for _, name := range []string{"Reception PC", "reception pc", "  Reception PC  "} {
		if _, err := s.Put(report(name), now); err != nil {
			t.Fatalf("Put %q: %v", name, err)
		}
	}

	machines, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Otherwise one machine appears three times because someone typed it
	// differently on Tuesday.
	if len(machines) != 1 {
		t.Errorf("List returned %d machines, want 1", len(machines))
	}
}

func TestHistoryIsCapped(t *testing.T) {
	s := store(t)

	for i := 0; i < MaxHistory+10; i++ {
		if _, err := s.Put(report("Reception PC"), now.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
	}

	machine, err := s.Get(MachineID("Reception PC"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(machine.History) != MaxHistory {
		t.Errorf("history = %d entries, want the cap of %d", len(machine.History), MaxHistory)
	}
	// The cap drops the oldest, not the newest.
	if !machine.History[0].Received.Equal(now.Add(time.Duration(MaxHistory+9) * time.Hour)) {
		t.Errorf("the newest entry was dropped: %v", machine.History[0].Received)
	}
}

func TestListPutsTheWorstFirst(t *testing.T) {
	s := store(t)

	urgent := checks.Result{CheckID: "disk.smart", Severity: checks.SeverityUrgent, Summary: "check.disk.smart.failing"}
	attention := checks.Result{CheckID: "disk.volumes", Severity: checks.SeverityAttention, Summary: "check.disk.volumes.low"}
	fine := checks.Result{CheckID: "os.info", Severity: checks.SeverityOK, Summary: "check.os.info.ok"}

	if _, err := s.Put(report("Quiet", fine), now); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Put(report("Warning", attention, fine), now); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := s.Put(report("Failing", urgent, fine), now); err != nil {
		t.Fatalf("Put: %v", err)
	}

	machines, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"Failing", "Warning", "Quiet"}
	for i, name := range want {
		if machines[i].Name != name {
			t.Errorf("position %d = %q, want %q", i, machines[i].Name, name)
		}
	}
}

func TestAReportMustSayEnoughToBeWorthStoring(t *testing.T) {
	s := store(t)

	cases := map[string]Report{
		"no name":           {Snapshot: snapshot()},
		"only whitespace":   {Name: "   ", Snapshot: snapshot()},
		"an over-long name": {Name: strings.Repeat("a", MaxNameLength+1), Snapshot: snapshot()},
		"no snapshot":       {Name: "Reception PC"},
		"no results":        {Name: "Reception PC", Snapshot: checks.Snapshot{Schema: checks.SnapshotSchema}},
	}

	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Put(r, now); err == nil {
				t.Error("Put accepted it")
			}
		})
	}

	machines, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(machines) != 0 {
		t.Errorf("a refused report was stored: %+v", machines)
	}
}

// TestAnIDFromAURLCannotReachOutsideTheStore is the path-traversal guard: the
// ID reaches the store from a URL path segment.
func TestAnIDFromAURLCannotReachOutsideTheStore(t *testing.T) {
	s := store(t)

	for _, id := range []string{
		"../../etc/passwd",
		"..",
		"",
		strings.Repeat("g", idLength), // hex only
		strings.Repeat("a", idLength-1),
		strings.Repeat("a", idLength+1),
		"/etc/passwd",
		"a/../../b",
	} {
		if _, err := s.Get(id); err == nil {
			t.Errorf("Get accepted %q", id)
		}
	}
}

func TestGetOnAMachineThatIsNotThere(t *testing.T) {
	s := store(t)

	_, err := s.Get(MachineID("never reported"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// TestARecordIsWrittenAtomically: a reader must never see a half-written
// record, and a failed write must not destroy the previous one.
func TestARecordIsWrittenAtomically(t *testing.T) {
	s := store(t)

	if _, err := s.Put(report("Reception PC"), now); err != nil {
		t.Fatalf("Put: %v", err)
	}

	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	// No temporary file is left behind for a reader to trip over.
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("the store holds %d files, want 1", len(entries))
	}
}

func TestOneUnreadableRecordDoesNotHideTheFleet(t *testing.T) {
	s := store(t)

	if _, err := s.Put(report("Reception PC"), now); err != nil {
		t.Fatalf("Put: %v", err)
	}
	corrupt := filepath.Join(s.Dir(), strings.Repeat("a", idLength)+".json")
	if err := os.WriteFile(corrupt, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	machines, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(machines) != 1 || machines[0].Name != "Reception PC" {
		t.Errorf("List returned %+v, want the readable machine", machines)
	}
}

func TestMachineIDIsStableAndHidesTheName(t *testing.T) {
	id := MachineID("Reception PC")

	if id != MachineID("reception pc") {
		t.Error("the same machine gets two identifiers depending on capitalisation")
	}
	if id == MachineID("Reception PC 2") {
		t.Error("two machines share an identifier")
	}
	if len(id) != idLength || !validID(id) {
		t.Errorf("ID = %q, which is not the expected shape", id)
	}
	// The name must not be recoverable from a URL or a file listing.
	if strings.Contains(strings.ToLower(id), "reception") {
		t.Errorf("the machine's name is visible in its identifier: %q", id)
	}
}

func TestOpenStoreNeedsADirectory(t *testing.T) {
	if _, err := OpenStore(""); err == nil {
		t.Error("OpenStore accepted an empty directory")
	}
}

func TestCountsAndLatestOnAMachineWithNoReports(t *testing.T) {
	var empty Machine

	if _, ok := empty.Latest(); ok {
		t.Error("Latest reported a report that is not there")
	}
	if got := empty.Counts(); got != nil {
		t.Errorf("Counts = %v, want nil", got)
	}
}
