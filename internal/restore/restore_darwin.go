package restore

import (
	"context"
	"fmt"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

const tmutilExe = "tmutil"

// darwinMaker takes an APFS local snapshot through Time Machine. A local
// snapshot is not a full backup — it lives on the same disk — but it is what
// macOS offers as a point to return to, and Migration Assistant and the
// recovery environment can both restore from one.
type darwinMaker struct{ run platform.Runner }

// New returns the restore point maker for this platform.
func New() Maker { return darwinMaker{run: platform.RunRead} }

func (m darwinMaker) Check(ctx context.Context) Availability {
	out := Availability{Kind: "APFS local snapshot"}

	// Listing snapshots needs no elevation and proves the mechanism works on
	// this volume; taking one does need administrator rights.
	if _, err := m.run(ctx, tmutilExe, "listlocalsnapshots", "/"); err != nil {
		out.Reason = KeyUnavailableUnreadable
		return out
	}

	elevated, err := platform.IsElevated()
	if err == nil && !elevated {
		out.Reason = KeyUnavailableNeedsAdmin
		return out
	}

	out.Available = true
	return out
}

func (m darwinMaker) Create(ctx context.Context, label string) (Point, error) {
	// tmutil names snapshots by timestamp; the label lives in our own audit
	// log and report rather than in the snapshot's name.
	raw, err := m.run(ctx, tmutilExe, "localsnapshot")
	if err != nil {
		return Point{}, fmt.Errorf("restore: create local snapshot: %w", err)
	}

	return Point{
		Kind:      "APFS local snapshot",
		Reference: snapshotName(raw),
		Label:     label,
		Created:   time.Now().UTC(),
	}, nil
}
