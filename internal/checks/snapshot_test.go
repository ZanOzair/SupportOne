package checks

import (
	"context"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

func TestRunAllNamesAdminChecksItSkipped(t *testing.T) {
	r := NewRegistry()
	mustRegister(t, r, stubCheck{id: "os.info", platforms: []platform.OS{platform.Linux}})
	mustRegister(t, r, stubCheck{id: "disk.smart", platforms: []platform.OS{platform.Linux}, admin: true})

	host := platform.Host{OS: platform.Linux, Arch: "amd64"}
	snap := RunAll(context.Background(), r, host, false, time.Second)

	if len(snap.Results) != 1 || snap.Results[0].CheckID != "os.info" {
		t.Fatalf("results = %v, want only os.info", resultIDs(snap.Results))
	}
	if len(snap.SkippedAdmin) != 1 || snap.SkippedAdmin[0] != "disk.smart" {
		t.Fatalf("skipped = %v, want [disk.smart] — a skipped check must be named, not omitted", snap.SkippedAdmin)
	}
	if snap.Schema != SnapshotSchema || snap.Host != host {
		t.Errorf("snapshot metadata = %+v, want schema %d and host %+v", snap, SnapshotSchema, host)
	}
}

func TestRunAllRunsAdminChecksWhenElevated(t *testing.T) {
	r := NewRegistry()
	mustRegister(t, r, stubCheck{id: "disk.smart", platforms: []platform.OS{platform.Linux}, admin: true})

	snap := RunAll(context.Background(), r, platform.Host{OS: platform.Linux}, true, time.Second)

	if len(snap.Results) != 1 {
		t.Fatalf("results = %v, want disk.smart to run when elevated", resultIDs(snap.Results))
	}
	if len(snap.SkippedAdmin) != 0 {
		t.Errorf("skipped = %v, want none", snap.SkippedAdmin)
	}
}

func TestRunAllExcludesOtherPlatforms(t *testing.T) {
	r := NewRegistry()
	mustRegister(t, r, stubCheck{id: "drivers.problem", platforms: []platform.OS{platform.Windows}})

	snap := RunAll(context.Background(), r, platform.Host{OS: platform.Darwin}, false, time.Second)

	if len(snap.Results) != 0 {
		t.Errorf("results = %v, want none on a platform the check does not support", resultIDs(snap.Results))
	}
}

func TestCounts(t *testing.T) {
	snap := Snapshot{Results: []Result{
		{Severity: SeverityOK},
		{Severity: SeverityOK},
		{Severity: SeverityUrgent},
		{Severity: SeverityUnknown},
	}}

	counts := snap.Counts()
	for severity, want := range map[Severity]int{
		SeverityOK:        2,
		SeverityUrgent:    1,
		SeverityUnknown:   1,
		SeverityAttention: 0,
	} {
		if counts[severity] != want {
			t.Errorf("counts[%s] = %d, want %d", severity, counts[severity], want)
		}
	}
}

func TestRunsOn(t *testing.T) {
	c := stubCheck{id: "os.info", platforms: []platform.OS{platform.Windows, platform.Linux}}
	if !RunsOn(c, platform.Linux) {
		t.Error("RunsOn(linux) = false, want true")
	}
	if RunsOn(c, platform.Darwin) {
		t.Error("RunsOn(darwin) = true, want false")
	}
}

func resultIDs(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.CheckID
	}
	return out
}
