// Package updates reports when the operating system last installed updates,
// and what the machine already knows is waiting.
//
// It never contacts an update server. Asking Windows Update, Apple or a
// distribution mirror "what is new?" is an outbound connection, and this agent
// makes none the user did not ask for. Everything here is read from records the
// machine already has, which is why pending counts reflect the last time the
// machine itself checked.
package updates

import (
	"context"
	"time"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/platform"
)

// updateFacts is what every platform's collector produces.
type updateFacts struct {
	// LastInstalled is zero when the machine keeps no record of it.
	LastInstalled time.Time

	// Pending is -1 when the platform cannot say without going online.
	Pending int

	// Source names where the answer came from, so the report can be specific
	// about what it is and is not telling the user.
	Source string
}

// Thresholds, documented in docs/CHECKS.md.
const (
	staleAfter     = 60 * 24 * time.Hour
	veryStaleAfter = 180 * 24 * time.Hour
)

// Message keys for this package's results.
const (
	keyUpdatesOK        = "check.updates.os.ok"
	keyUpdatesPending   = "check.updates.os.pending"
	keyUpdatesStale     = "check.updates.os.stale"
	keyUpdatesVeryStale = "check.updates.os.very_stale"
	keyUpdatesUnknown   = "check.updates.os.unknown"
)

type osUpdatesCheck struct {
	run platform.Runner

	// now is swappable in tests; production always uses time.Now.
	now func() time.Time
}

func (osUpdatesCheck) ID() string               { return "updates.os" }
func (osUpdatesCheck) Platforms() []platform.OS { return platform.All() }
func (osUpdatesCheck) RequiresAdmin() bool      { return false }

func (c osUpdatesCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectUpdates(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	return c.verdict(facts), nil
}

func (c osUpdatesCheck) verdict(facts updateFacts) checks.Result {
	now := time.Now
	if c.now != nil {
		now = c.now
	}

	detail := map[string]any{"source": facts.Source}
	if !facts.LastInstalled.IsZero() {
		detail["last_installed"] = facts.LastInstalled.Format(time.RFC3339)
	}
	if facts.Pending >= 0 {
		detail["pending"] = facts.Pending
	}

	if facts.LastInstalled.IsZero() {
		if facts.Pending > 0 {
			return checks.Attention(checks.PluralKey(keyUpdatesPending, facts.Pending), facts.Pending).With(detail)
		}
		return checks.Unknown(keyUpdatesUnknown).With(detail)
	}

	age := now().Sub(facts.LastInstalled)
	days := int(age.Hours() / 24)
	detail["days_since_update"] = days

	switch {
	case age > veryStaleAfter:
		return checks.Urgent(keyUpdatesVeryStale, days).With(detail)
	case age > staleAfter:
		return checks.Attention(keyUpdatesStale, days).With(detail)
	case facts.Pending > 0:
		return checks.Attention(checks.PluralKey(keyUpdatesPending, facts.Pending), facts.Pending).With(detail)
	default:
		return checks.OK(keyUpdatesOK, days).With(detail)
	}
}

func init() {
	checks.MustRegister(osUpdatesCheck{run: platform.RunRead})
}
