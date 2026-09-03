// Package spooler clears a stuck Windows print queue.
//
// A jammed queue is the classic "it just won't print" problem, and the usual
// advice online is to delete everything in the spool directory. This fix does
// the same job without the deletion: the spool files are moved into quarantine
// on the same volume, so a rollback puts the queue back exactly as it was —
// including the document that was jamming it, in case it mattered.
//
// The service has to be stopped for either direction, and it is always started
// again, including when the work in between fails.
package spooler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// ID is the stable identifier this fix is offered and audited under.
const ID = "print.clear-spooler"

// Message keys this fix resolves through internal/i18n.
const (
	KeySummary = "fix.print.clear_spooler.summary"
	KeyUndo    = "fix.print.clear_spooler.undo"

	KeyChangeStop  = "fix.print.clear_spooler.change.stop"
	KeyChangeMove  = "fix.print.clear_spooler.change.move"
	KeyChangeStart = "fix.print.clear_spooler.change.start"

	KeyBlockedEmpty      = "fix.print.clear_spooler.blocked.empty"
	KeyBlockedUnreadable = "fix.print.clear_spooler.blocked.unreadable"

	KeyOutcomeCleared = "fix.print.clear_spooler.outcome.cleared"
)

// service is the Windows print spooler, named once.
const service = "spooler"

// defaultSpoolDir is where Windows keeps queued print jobs.
const defaultSpoolDir = `C:\Windows\System32\spool\PRINTERS`

// quarantineName is the holding directory, placed beside the queue so moving a
// file into it is a rename on the same volume rather than a copy.
const quarantineName = "SupportOne-quarantine"

// Fix clears the print queue, reversibly.
type Fix struct {
	// SpoolDir is the queue directory. Empty means the Windows default.
	SpoolDir string

	// run performs a service command. Tests substitute their own.
	run platform.Runner

	// take is the seam that stands in for a job file that will not move —
	// the case this fix has to leave the queue whole after.
	take func(*fixes.Quarantine, string) error

	held *fixes.Quarantine
}

// New returns the fix over this machine's print queue.
func New() *Fix { return &Fix{} }

var _ fixes.Fix = (*Fix)(nil)

// ID returns this fix's stable identifier.
func (f *Fix) ID() string { return ID }

// Describe returns what the user is shown before they confirm anything.
func (f *Fix) Describe() fixes.Explanation {
	return fixes.Explanation{
		Summary: KeySummary,
		Changes: []string{KeyChangeStop, KeyChangeMove, KeyChangeStart},
		Undo:    KeyUndo,
	}
}

// Platforms is Windows only. macOS and Linux both queue through CUPS, whose
// spool directory mixes job data with the control files CUPS needs to stay
// consistent; clearing it the way this fix clears the Windows queue is not the
// same operation, and pretending it is would be worse than not offering it.
func (f *Fix) Platforms() []platform.OS { return []platform.OS{platform.Windows} }

// RequiresAdmin is true: stopping a system service and touching its spool
// directory both need administrator rights.
func (f *Fix) RequiresAdmin() bool { return true }

// Reversible is true: every queued job is moved, not deleted, and Rollback puts
// all of it back.
func (f *Fix) Reversible() bool { return true }

func (f *Fix) dir() string {
	if f.SpoolDir != "" {
		return f.SpoolDir
	}
	return defaultSpoolDir
}

func (f *Fix) runner() platform.Runner {
	if f.run != nil {
		return f.run
	}
	return platform.RunAction
}

// queued lists the job files in the queue, excluding the holding directory.
func (f *Fix) queued() ([]string, error) {
	entries, err := os.ReadDir(f.dir())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", KeyBlockedUnreadable, err)
	}

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), quarantineName) {
			continue
		}
		out = append(out, filepath.Join(f.dir(), e.Name()))
	}
	return out, nil
}

// Preflight refuses an empty queue. Stopping and starting the print service to
// move nothing is a change with no benefit, and offering it would teach the
// user that confirming a change is a formality.
func (f *Fix) Preflight(context.Context) error {
	queued, err := f.queued()
	if err != nil {
		return err
	}
	if len(queued) == 0 {
		return errors.New(KeyBlockedEmpty)
	}
	return nil
}

// Apply stops the spooler, moves the queued jobs aside, and starts it again.
func (f *Fix) Apply(ctx context.Context) (fixes.Outcome, error) {
	queued, err := f.queued()
	if err != nil {
		return fixes.Outcome{}, err
	}
	if len(queued) == 0 {
		return fixes.Outcome{}, errors.New(KeyBlockedEmpty)
	}

	if err := f.stop(ctx); err != nil {
		return fixes.Outcome{}, err
	}
	// The service goes back on whatever happens next. A machine left unable to
	// print at all would be a worse outcome than the jam this fix was called
	// about.
	defer func() { _ = f.start(ctx) }()

	held, err := fixes.NewQuarantine(filepath.Join(f.dir(), quarantineName), ID)
	if err != nil {
		return fixes.Outcome{}, err
	}
	f.held = held

	for _, path := range queued {
		if err := f.takeOne(held, path); err != nil {
			// Put back what was already taken, so a half-cleared queue is
			// never left behind.
			if restoreErr := held.Restore(); restoreErr != nil {
				// Both failures matter, and the second is the one the user has
				// to act on: jobs are sitting in the holding folder.
				return fixes.Outcome{}, fmt.Errorf("spooler: a queued job could not be moved (%w), and the jobs already moved could not be put back: %w", err, restoreErr)
			}
			f.held = nil
			return fixes.Outcome{}, err
		}
	}

	return fixes.Outcome{
		Applied:    true,
		Detail:     fixes.PluralKey(KeyOutcomeCleared, held.Count()),
		DetailArgs: []any{held.Count()},
	}, nil
}

// Rollback puts the queued jobs back and restarts the service so it picks them
// up again.
func (f *Fix) Rollback(ctx context.Context) error {
	if f.held == nil {
		return nil
	}

	if err := f.stop(ctx); err != nil {
		return err
	}
	defer func() { _ = f.start(ctx) }()

	if err := f.held.Restore(); err != nil {
		return err
	}
	f.held = nil
	return nil
}

func (f *Fix) takeOne(q *fixes.Quarantine, path string) error {
	if f.take != nil {
		return f.take(q, path)
	}
	return q.Take(path)
}

func (f *Fix) stop(ctx context.Context) error {
	if _, err := f.runner()(ctx, "net", "stop", service); err != nil {
		return fmt.Errorf("spooler: stop the print service: %w", err)
	}
	return nil
}

func (f *Fix) start(ctx context.Context) error {
	if _, err := f.runner()(ctx, "net", "start", service); err != nil {
		return fmt.Errorf("spooler: start the print service: %w", err)
	}
	return nil
}

// Held reports how many queued jobs are currently set aside.
func (f *Fix) Held() int {
	if f.held == nil {
		return 0
	}
	return f.held.Count()
}

func init() {
	fixes.MustRegister(New())
}
