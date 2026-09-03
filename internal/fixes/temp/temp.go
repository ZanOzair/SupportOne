// Package temp clears the temporary files a machine has stopped using.
//
// This is the fix that shows what "reversible" means here. Nothing is deleted:
// every file is moved into a quarantine directory on the same volume, and
// Rollback moves all of it back where it came from, with its permissions
// intact. Reclaiming the space for good is a separate, later decision the user
// makes with the files still in front of them.
package temp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// ID is the stable identifier this fix is offered and audited under.
const ID = "temp.clear"

// Message keys this fix resolves through internal/i18n.
const (
	KeySummary = "fix.temp.clear.summary"
	KeyUndo    = "fix.temp.clear.undo"

	KeyChangeMove = "fix.temp.clear.change.move"
	KeyChangeKeep = "fix.temp.clear.change.keep"
	KeyChangeSkip = "fix.temp.clear.change.skip"

	KeyBlockedNothing    = "fix.temp.clear.blocked.nothing"
	KeyBlockedUnreadable = "fix.temp.clear.blocked.unreadable"

	KeyOutcomeMoved = "fix.temp.clear.outcome.moved"
	KeyOutcomeInUse = "fix.temp.clear.outcome.in_use"
)

// minAge is how long a temporary file must have gone untouched before this fix
// will move it. A file an application wrote a minute ago is a file it is
// probably still using; a week-old one is litter.
const minAge = 7 * 24 * time.Hour

// quarantineName is the directory, inside the temporary directory itself, that
// holds what was moved. Keeping it there guarantees a rename rather than a copy
// — same volume — and makes the holding area obvious to anyone looking.
const quarantineName = "SupportOne-quarantine"

// Fix moves aged temporary files aside.
type Fix struct {
	// Dir is the temporary directory to clear. Empty means the one this OS
	// gives the current user.
	Dir string

	// MinAge overrides how old an entry must be. Zero means the default.
	MinAge time.Duration

	// Now is swappable in tests.
	Now func() time.Time

	// take is the seam that stands in for a file another process has open.
	// That case is ordinary on a real machine and awkward to stage in a test,
	// and it is the one branch where this fix deliberately carries on rather
	// than failing.
	take func(*fixes.Quarantine, string) error

	held *fixes.Quarantine
}

// New returns the fix over this machine's temporary directory.
func New() *Fix { return &Fix{} }

// Ensure the contract is satisfied at compile time rather than at the moment a
// user clicks something.
var _ fixes.Fix = (*Fix)(nil)

// ID returns this fix's stable identifier.
func (f *Fix) ID() string { return ID }

// Describe returns what the user is shown before they confirm anything.
func (f *Fix) Describe() fixes.Explanation {
	return fixes.Explanation{
		Summary: KeySummary,
		Changes: []string{KeyChangeMove, KeyChangeKeep, KeyChangeSkip},
		Undo:    KeyUndo,
	}
}

// Platforms is every desktop OS: each has a temporary directory, and none of
// them promises anything about what is left in it.
func (f *Fix) Platforms() []platform.OS { return platform.All() }

// RequiresAdmin is false: this fix touches the current user's own temporary
// directory and nothing else. Asking for administrator rights to tidy your own
// files would be asking for more than the job needs.
func (f *Fix) RequiresAdmin() bool { return false }

// Reversible is true, and it is true because Rollback moves every file back —
// not because the files are unimportant.
func (f *Fix) Reversible() bool { return true }

func (f *Fix) dir() string {
	if f.Dir != "" {
		return f.Dir
	}
	return os.TempDir()
}

func (f *Fix) minAge() time.Duration {
	if f.MinAge > 0 {
		return f.MinAge
	}
	return minAge
}

func (f *Fix) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

// Preflight reads the temporary directory and refuses when there is nothing
// worth doing. A fix that ran anyway and reported success for moving zero files
// would be inviting the user to confirm a change that is not a change.
func (f *Fix) Preflight(ctx context.Context) error {
	candidates, err := f.candidates(ctx)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return errors.New(KeyBlockedNothing)
	}
	return nil
}

// candidates lists the entries this fix would move: everything directly in the
// temporary directory that has gone untouched for long enough, excluding the
// quarantine itself.
func (f *Fix) candidates(ctx context.Context) ([]string, error) {
	dir := f.dir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", KeyBlockedUnreadable, err)
	}

	cutoff := f.now().Add(-f.minAge())
	out := make([]string, 0, len(entries))

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Never move our own holding area into itself, and never move an
		// earlier run's.
		if strings.HasPrefix(e.Name(), quarantineName) {
			continue
		}

		info, err := e.Info()
		if err != nil {
			// The entry vanished between listing and asking about it, which
			// is ordinary in a temporary directory.
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out, nil
}

// Apply moves the aged entries into quarantine.
//
// An entry that will not move is left exactly where it is and counted. In a
// temporary directory that is the normal case, not a failure: something has the
// file open, and taking it would be worse than leaving it.
func (f *Fix) Apply(ctx context.Context) (fixes.Outcome, error) {
	candidates, err := f.candidates(ctx)
	if err != nil {
		return fixes.Outcome{}, err
	}
	if len(candidates) == 0 {
		return fixes.Outcome{}, errors.New(KeyBlockedNothing)
	}

	held, err := fixes.NewQuarantine(filepath.Join(f.dir(), quarantineName), ID)
	if err != nil {
		return fixes.Outcome{}, err
	}
	f.held = held

	inUse := 0
	for _, path := range candidates {
		if err := ctx.Err(); err != nil {
			return fixes.Outcome{}, err
		}
		if err := f.takeOne(held, path); err != nil {
			inUse++
		}
	}

	detail, args := fixes.PluralKey(KeyOutcomeMoved, held.Count()), []any{held.Count()}
	if inUse > 0 {
		detail, args = fixes.PluralKey(KeyOutcomeInUse, held.Count()), []any{held.Count(), inUse}
	}
	return fixes.Outcome{
		Applied:    true,
		Detail:     detail,
		DetailArgs: args,
	}, nil
}

func (f *Fix) takeOne(q *fixes.Quarantine, path string) error {
	if f.take != nil {
		return f.take(q, path)
	}
	return q.Take(path)
}

// Rollback puts every quarantined file back.
func (f *Fix) Rollback(context.Context) error {
	if f.held == nil {
		return nil
	}
	if err := f.held.Restore(); err != nil {
		return err
	}
	f.held = nil
	return nil
}

// Held reports how many entries are currently in quarantine, so the interface
// can offer to put them back.
func (f *Fix) Held() int {
	if f.held == nil {
		return 0
	}
	return f.held.Count()
}

func init() {
	fixes.MustRegister(New())
}
