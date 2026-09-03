// Package fixes defines the remediation plugin contract and the registry of
// compiled-in fixes.
//
// Fixes are the only code in SupportOne that changes a machine. Three rules
// hold for every one of them: the user is told exactly what will change before
// it runs, the change is made only after they confirm that specific action,
// and the change can be undone.
//
// A fix is reachable only by its registered ID. Nothing — not user input, not
// a model response — can name a fix that was not compiled in.
package fixes

import (
	"context"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Explanation is shown to the user before a fix runs, in plain language.
type Explanation struct {
	// Summary is one sentence: what this fix does and why. A message key
	// resolved through internal/i18n.
	Summary string `json:"summary"`

	// Changes lists every system change the fix makes, one line each. The
	// user confirms against this list, so it is exhaustive: a fix that does
	// something not listed here is a bug.
	Changes []string `json:"changes"`

	// Undo describes how the change is reversed, so the user knows the exit
	// before they agree to the entrance.
	Undo string `json:"undo"`
}

// Outcome records what a fix actually did.
type Outcome struct {
	FixID   string `json:"fix_id"`
	Applied bool   `json:"applied"`

	// DryRun is true when the fix was asked what it would change and
	// deliberately changed nothing.
	DryRun bool `json:"dry_run"`

	// Detail says what happened, as a message key resolved through
	// internal/i18n rather than English prose, so the record of a change
	// reads in the same language as the rest of the interface.
	Detail string `json:"detail,omitempty"`

	// DetailArgs fill the placeholders in Detail.
	DetailArgs []any `json:"detail_args,omitempty"`

	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration_ns"`
}

// Fix is a reversible, whitelisted remediation module.
type Fix interface {
	// ID is stable and dotted, e.g. "net.flush-dns".
	ID() string

	// Describe returns what the fix will do, before it does it.
	Describe() Explanation

	Platforms() []platform.OS
	RequiresAdmin() bool

	// Reversible reports whether Rollback restores the prior state. A fix
	// that cannot honestly claim this returns false and the UI says so
	// rather than implying an undo that does not exist.
	Reversible() bool

	// Preflight inspects the machine and returns an error if applying now
	// would be unsafe or pointless. It must not change anything.
	Preflight(ctx context.Context) error

	// Apply performs the change. It is called only after Preflight succeeds
	// and the user confirms this specific action.
	Apply(ctx context.Context) (Outcome, error)

	// Rollback undoes Apply.
	Rollback(ctx context.Context) error
}

// RunsOn reports whether f is available on os.
func RunsOn(f Fix, os platform.OS) bool {
	for _, p := range f.Platforms() {
		if p == os {
			return true
		}
	}
	return false
}
