package checks

import (
	"context"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// SnapshotSchema is the version of the JSON snapshot format. It changes only
// when the shape changes, so a technician's tooling can rely on it.
const SnapshotSchema = 1

// Snapshot is the result of running the checks available on this machine.
type Snapshot struct {
	Schema       int           `json:"schema"`
	AgentVersion string        `json:"agent_version"`
	GeneratedAt  time.Time     `json:"generated_at"`
	Host         platform.Host `json:"host"`

	// Results holds one entry per check that ran, including the ones that
	// could not answer.
	Results []Result `json:"results"`

	// SkippedAdmin lists checks that were not run because they need
	// administrator rights this process does not hold. They are named rather
	// than omitted: a check that did not run is not a check that passed.
	SkippedAdmin []string `json:"skipped_needs_admin,omitempty"`
}

// Counts returns how many results fall into each severity.
func (s Snapshot) Counts() map[Severity]int {
	out := make(map[Severity]int, 4)
	for _, r := range s.Results {
		out[r.Severity]++
	}
	return out
}

// RunAll runs every check registered for host.OS and returns a snapshot.
//
// Checks needing rights the process does not hold are listed in SkippedAdmin
// rather than run and reported as failures.
func RunAll(ctx context.Context, r *Registry, host platform.Host, elevated bool, timeout time.Duration) Snapshot {
	snap := Snapshot{
		Schema:      SnapshotSchema,
		GeneratedAt: time.Now().UTC(),
		Host:        host,
		Results:     []Result{},
	}

	for _, c := range r.ForPlatform(host.OS) {
		if c.RequiresAdmin() && !elevated {
			snap.SkippedAdmin = append(snap.SkippedAdmin, c.ID())
			continue
		}
		snap.Results = append(snap.Results, Run(ctx, c, timeout))
	}
	return snap
}
