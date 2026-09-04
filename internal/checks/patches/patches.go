// Package patches reports what this machine has actually applied, and when.
//
// The updates.os check answers "is this machine behind?". This one answers the
// question a technician has to put in writing at the end of a month: which
// patches went on, and on what dates. Both read records the machine already
// holds. Neither contacts an update server — asking Windows Update, Apple or a
// distribution mirror what is new is an outbound connection, and this agent
// makes none the user did not ask for.
//
// The honest limit is that every platform records this differently and none
// records it completely. Windows keeps hotfixes but not driver or Store
// updates; macOS keeps its own installs but not App Store apps; a Linux
// package log is rotated away on a schedule the distribution chose. So the
// count is always "what this machine still has a record of", and the report
// says which record it read.
package patches

import (
	"context"
	"sort"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// patch is one applied update, as the machine recorded it.
type patch struct {
	// ID is the platform's own identifier: a KB number, a package name, a
	// macOS update label.
	ID string `json:"id"`

	// Title is a human description where the platform keeps one.
	Title string `json:"title,omitempty"`

	// Applied is when it went on. Zero means the record has no date, which
	// is reported as unknown rather than guessed.
	Applied time.Time `json:"applied,omitempty"`
}

// patchFacts is what every platform's collector produces.
type patchFacts struct {
	// Source names the record that was read, so the report can be specific
	// about what it is and is not covering.
	Source string

	// Applied is what the machine still has a record of, newest first.
	Applied []patch

	// Horizon is how far back the record itself goes, where the platform
	// makes that knowable. Zero means it does not.
	Horizon time.Time
}

// Message keys for this package's results.
const (
	keyPatchesRecent     = "check.updates.installed.recent"
	keyPatchesRecentOne  = "check.updates.installed.recent.one"
	keyPatchesOld        = "check.updates.installed.old"
	keyPatchesNone       = "check.updates.installed.none"
	keyPatchesUnreadable = "check.updates.installed.unreadable"
)

// recentWindow is how far back a patch still counts as recent activity. It
// matches the reporting period a monthly statement covers.
const recentWindow = 30 * 24 * time.Hour

// maxListed bounds how many patches are carried in the evidence. A machine
// with years of history does not need to put all of it in every report, and
// the newest are the ones a reader is asking about.
const maxListed = 50

type installedCheck struct {
	run platform.Runner

	// now is swappable in tests.
	now func() time.Time
}

func (installedCheck) ID() string               { return "updates.installed" }
func (installedCheck) Platforms() []platform.OS { return platform.All() }
func (installedCheck) RequiresAdmin() bool      { return false }

func (c installedCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectPatches(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	return patchesVerdict(facts, c.at()), nil
}

func (c installedCheck) at() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// patchesVerdict decides from the facts alone.
//
// Nothing here is urgent. A patch record is inventory, not a fault: whether
// this machine is dangerously behind is updates.os's question, and answering
// it twice in two different ways would let the two disagree.
func patchesVerdict(facts patchFacts, now time.Time) checks.Result {
	applied := newestFirst(facts.Applied)

	detail := map[string]any{"source": facts.Source, "recorded": len(applied)}
	if !facts.Horizon.IsZero() {
		detail["record_starts"] = facts.Horizon.UTC().Format(time.RFC3339)
	}
	if listed := listable(applied); len(listed) > 0 {
		detail["patches"] = listed
	}

	if facts.Source == "" {
		return checks.Unknown(keyPatchesUnreadable).With(detail)
	}
	if len(applied) == 0 {
		// A record that exists and is empty is a real answer: this machine
		// has applied nothing the record covers.
		return checks.Attention(keyPatchesNone, facts.Source).With(detail)
	}

	recent := countSince(applied, now.Add(-recentWindow))
	newest := applied[0].Applied

	if recent == 0 {
		if newest.IsZero() {
			// Dated records exist for none of them, so "how recently" cannot
			// be answered from this record at all.
			return checks.Unknown(keyPatchesUnreadable).With(detail)
		}
		return checks.Attention(keyPatchesOld, checks.HumanDuration(now.Sub(newest)), facts.Source).With(detail)
	}

	summary := keyPatchesRecent
	if recent == 1 {
		summary = keyPatchesRecentOne
	}
	return checks.OK(summary, recent, facts.Source).With(detail)
}

// newestFirst sorts by applied date, newest first, with undated entries last:
// a reader scanning the list wants the recent ones at the top, and an entry
// with no date belongs after every entry that has one.
func newestFirst(applied []patch) []patch {
	out := append([]patch(nil), applied...)
	sort.SliceStable(out, func(i, j int) bool {
		switch {
		case out[i].Applied.IsZero():
			return false
		case out[j].Applied.IsZero():
			return true
		default:
			return out[i].Applied.After(out[j].Applied)
		}
	})
	return out
}

// countSince counts entries applied at or after cutoff. Undated entries are
// not counted: an unknown date is not evidence of recent activity.
func countSince(applied []patch, cutoff time.Time) int {
	count := 0
	for _, p := range applied {
		if !p.Applied.IsZero() && !p.Applied.Before(cutoff) {
			count++
		}
	}
	return count
}

// listable renders the newest entries for the evidence, capped.
func listable(applied []patch) []map[string]any {
	if len(applied) > maxListed {
		applied = applied[:maxListed]
	}

	out := make([]map[string]any, 0, len(applied))
	for _, p := range applied {
		entry := map[string]any{"id": p.ID}
		if p.Title != "" {
			entry["title"] = p.Title
		}
		if !p.Applied.IsZero() {
			entry["applied"] = p.Applied.UTC().Format("2006-01-02")
		}
		out = append(out, entry)
	}
	return out
}

func init() {
	checks.MustRegister(installedCheck{run: platform.RunRead})
}
