// Package restore creates the system-level undo that sits behind a fix.
//
// A fix's own Rollback undoes what that fix did. A restore point is the wider
// net: if something else goes wrong in the same session, the machine can be put
// back. Not every platform has one, and this package says so plainly rather
// than pretending — a user who is told there is no restore point can still
// decide to go ahead, but they decide knowing it.
package restore

import (
	"context"
	"time"
)

// Point is a restore point that was created.
type Point struct {
	// Kind names the mechanism, e.g. "Windows System Restore".
	Kind string `json:"kind"`

	// Reference is the platform's own identifier for the point, where it
	// gives one, so a technician can find it later.
	Reference string `json:"reference,omitempty"`

	Label   string    `json:"label"`
	Created time.Time `json:"created"`
}

// Availability says whether a restore point can be made, and why not when it
// cannot. The reason is shown to the user before they confirm a change.
type Availability struct {
	Available bool `json:"available"`

	// Kind names the mechanism that would be used, or would have been.
	Kind string `json:"kind"`

	// Reason explains an unavailable mechanism in the user's terms: the
	// feature is switched off, or the platform has no equivalent, or it needs
	// administrator rights.
	Reason string `json:"reason,omitempty"`
}

// Maker creates restore points. Each platform has one implementation, and
// tests substitute their own.
type Maker interface {
	// Check reports whether a restore point can be made right now. It must
	// not create anything.
	Check(ctx context.Context) Availability

	// Create makes a restore point labelled for the action about to happen.
	Create(ctx context.Context, label string) (Point, error)
}

// Message keys this package's availability reasons resolve through.
const (
	KeyUnavailableOnPlatform = "restore.unavailable.platform"
	KeyUnavailableDisabled   = "restore.unavailable.disabled"
	KeyUnavailableNeedsAdmin = "restore.unavailable.needs_admin"
	KeyUnavailableUnreadable = "restore.unavailable.unreadable"
)
