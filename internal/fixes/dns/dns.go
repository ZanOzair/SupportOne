// Package dns clears the machine's cached record of which names point where.
//
// This is the one fix in SupportOne that does not restore its prior state on
// rollback, and it says so rather than pretending otherwise. The DNS cache is
// not state the user owns: it is a copy of answers the machine already has a
// way to fetch again, and it refills itself the next time anything looks a name
// up. What is lost by clearing it is a few milliseconds, not information — but
// "lost nothing" is still not "put back", so Reversible reports false and the
// explanation says why.
package dns

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// ID is the stable identifier this fix is offered and audited under.
const ID = "net.flush-dns"

// Message keys this fix resolves through internal/i18n.
const (
	KeySummary = "fix.net.flush_dns.summary"
	KeyUndo    = "fix.net.flush_dns.undo"

	KeyChangeDiscard = "fix.net.flush_dns.change.discard"
	KeyChangeRefill  = "fix.net.flush_dns.change.refill"

	KeyBlockedNoTool  = "fix.net.flush_dns.blocked.no_tool"
	KeyBlockedNoCache = "fix.net.flush_dns.blocked.no_cache"
	KeyBlockedElse    = "fix.net.flush_dns.blocked.platform"

	KeyOutcomeCleared = "fix.net.flush_dns.outcome.cleared"
)

// command is one compiled-in invocation. Nothing here is ever assembled from
// user input or model output: the tables below are the whole vocabulary.
type command struct {
	name string
	args []string
}

// procedure is how one platform clears its cache.
type procedure struct {
	// probe is a read-only command proving the mechanism is there to use. It
	// runs during Preflight, which must change nothing.
	probe *command

	// steps run in order once the user has confirmed.
	steps []command
}

// procedures is the complete set of commands this fix can ever run.
var procedures = map[platform.OS]procedure{
	platform.Windows: {
		steps: []command{{name: "ipconfig", args: []string{"/flushdns"}}},
	},
	platform.Darwin: {
		// Two steps because macOS splits the job: the directory service holds
		// one cache and mDNSResponder holds the other.
		steps: []command{
			{name: "dscacheutil", args: []string{"-flushcache"}},
			{name: "killall", args: []string{"-HUP", "mDNSResponder"}},
		},
	},
	platform.Linux: {
		// Linux has a cache only if something is caching. systemd-resolved is
		// the common case; where it is absent there is nothing to clear, and
		// Preflight says so instead of inventing work.
		probe: &command{name: "resolvectl", args: []string{"status"}},
		steps: []command{{name: "resolvectl", args: []string{"flush-caches"}}},
	},
}

// Fix clears the DNS cache.
type Fix struct {
	// OS is the platform to act as. Empty means the running one.
	OS platform.OS

	// run performs a step. Tests substitute their own; production runs the
	// compiled-in command.
	run platform.Runner

	// lookPath reports whether a tool is installed. Substituted in tests.
	lookPath func(string) (string, error)
}

// New returns the fix for this machine.
func New() *Fix { return &Fix{} }

var _ fixes.Fix = (*Fix)(nil)

// ID returns this fix's stable identifier.
func (f *Fix) ID() string { return ID }

// Describe returns what the user is shown before they confirm anything.
func (f *Fix) Describe() fixes.Explanation {
	return fixes.Explanation{
		Summary: KeySummary,
		Changes: []string{KeyChangeDiscard, KeyChangeRefill},
		Undo:    KeyUndo,
	}
}

// Platforms is every desktop OS: each one caches name lookups, and each one
// has its own way of throwing that cache away.
func (f *Fix) Platforms() []platform.OS { return platform.All() }

// RequiresAdmin is true on every platform: the cache belongs to the system
// resolver, not to the user's session.
func (f *Fix) RequiresAdmin() bool { return true }

// Reversible is false, and the explanation's Undo says what that means here:
// there is nothing to put back, because nothing was taken that the machine
// cannot fetch again.
func (f *Fix) Reversible() bool { return false }

func (f *Fix) os() platform.OS {
	if f.OS != "" {
		return f.OS
	}
	return platform.Current()
}

func (f *Fix) runner() platform.Runner {
	if f.run != nil {
		return f.run
	}
	return platform.RunAction
}

func (f *Fix) installed(name string) error {
	look := f.lookPath
	if look == nil {
		look = exec.LookPath
	}
	if _, err := look(name); err != nil {
		return fmt.Errorf("%s: %s", KeyBlockedNoTool, name)
	}
	return nil
}

// Preflight proves the tools are there and, where the platform needs it, that
// there is a cache at all. It runs nothing that changes anything.
func (f *Fix) Preflight(ctx context.Context) error {
	how, ok := procedures[f.os()]
	if !ok {
		return errors.New(KeyBlockedElse)
	}

	for _, step := range how.steps {
		if err := f.installed(step.name); err != nil {
			return err
		}
	}

	if how.probe != nil {
		if err := f.installed(how.probe.name); err != nil {
			return err
		}
		// The probe is read-only: it asks the resolver to describe itself.
		if _, err := platform.RunRead(ctx, how.probe.name, how.probe.args...); err != nil {
			return fmt.Errorf("%s: %w", KeyBlockedNoCache, err)
		}
	}
	return nil
}

// Apply clears the cache.
func (f *Fix) Apply(ctx context.Context) (fixes.Outcome, error) {
	how, ok := procedures[f.os()]
	if !ok {
		return fixes.Outcome{}, errors.New(KeyBlockedElse)
	}

	run := f.runner()
	for _, step := range how.steps {
		if err := ctx.Err(); err != nil {
			return fixes.Outcome{}, err
		}
		if _, err := run(ctx, step.name, step.args...); err != nil {
			return fixes.Outcome{Applied: false}, fmt.Errorf("dns: %s: %w", step.name, err)
		}
	}

	return fixes.Outcome{Applied: true, Detail: KeyOutcomeCleared}, nil
}

// Rollback does nothing, and does nothing deliberately.
//
// Refilling the cache with the entries that were in it is not something any of
// these platforms offers, and faking it — by looking the same names up again —
// would be a different action wearing an undo's name. The machine repopulates
// the cache itself, on demand, which is the actual answer.
func (f *Fix) Rollback(context.Context) error { return nil }

func init() {
	fixes.MustRegister(New())
}
