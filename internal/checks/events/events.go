// Package events reports the errors the operating system logged about itself.
//
// Every machine logs some errors; a raw count is noise, and presenting one as a
// problem count is the oldest trick in the PC-cleaner playbook. This check looks
// for the thing that actually indicates a fault: the same component failing
// over and over.
package events

import (
	"context"
	"sort"
	"time"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/platform"
)

// logEvent is one error the system recorded about itself.
type logEvent struct {
	Time    time.Time `json:"time"`
	Source  string    `json:"source"`
	ID      string    `json:"id,omitempty"`
	Level   string    `json:"level,omitempty"`
	Message string    `json:"message,omitempty"`
}

// repetition is one component failing repeatedly.
type repetition struct {
	Source string `json:"source"`
	ID     string `json:"id,omitempty"`
	Count  int    `json:"count"`
}

// repeatThreshold is how many times the same source and event must appear
// before the check calls it a pattern rather than noise. It is stated here and
// in docs/CHECKS.md.
const repeatThreshold = 10

// maxEvents bounds how much log the check reads, so a machine with a noisy log
// cannot stall a snapshot.
const maxEvents = 200

// Message keys for this package's results.
const (
	keyEventsNone     = "check.eventlog.errors.none"
	keyEventsQuiet    = "check.eventlog.errors.quiet"
	keyEventsRepeated = "check.eventlog.errors.repeated"
	keyEventsCritical = "check.eventlog.errors.critical"
)

type errorsCheck struct{ run platform.Runner }

func (errorsCheck) ID() string               { return "eventlog.errors" }
func (errorsCheck) Platforms() []platform.OS { return platform.All() }

// RequiresAdmin is false: this check reads only the system log, which every
// platform exposes to ordinary users. It never reads the security log, which
// would need elevation and is not its business.
func (errorsCheck) RequiresAdmin() bool { return false }

func (c errorsCheck) Run(ctx context.Context) (checks.Result, error) {
	events, window, err := collectEvents(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}

	days := int(window.Hours() / 24)
	if len(events) == 0 {
		return checks.OK(keyEventsNone, days), nil
	}

	repeats := findRepetitions(events)
	detail := map[string]any{
		"window_days": days,
		"count":       len(events),
		"events":      events,
	}
	if len(repeats) > 0 {
		detail["repeated"] = repeats
	}

	if critical := countCritical(events); critical > 0 {
		return checks.Attention(checks.PluralKey(keyEventsCritical, critical), critical, days).With(detail), nil
	}
	if len(repeats) > 0 {
		worst := repeats[0]
		return checks.Attention(keyEventsRepeated, worst.Source, worst.Count, days).With(detail), nil
	}

	// Errors happened, but nothing is failing repeatedly. That is a normal
	// machine, and saying so plainly is the honest answer.
	return checks.OK(checks.PluralKey(keyEventsQuiet, len(events)), len(events), days).With(detail), nil
}

// findRepetitions groups events by source and ID and returns the groups that
// cross the threshold, worst first.
func findRepetitions(events []logEvent) []repetition {
	type key struct{ source, id string }
	counts := make(map[key]int)
	for _, e := range events {
		counts[key{e.Source, e.ID}]++
	}

	var out []repetition
	for k, count := range counts {
		if count >= repeatThreshold {
			out = append(out, repetition{Source: k.source, ID: k.id, Count: count})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Source < out[j].Source
	})
	return out
}

// countCritical counts events the platform itself marked as critical — a
// bugcheck, a disk controller failure, something that stopped the machine.
func countCritical(events []logEvent) int {
	count := 0
	for _, e := range events {
		if e.Level == levelCritical {
			count++
		}
	}
	return count
}

// Levels are normalised across platforms so the verdict does not depend on
// which OS produced the log.
const (
	levelCritical = "critical"
	levelError    = "error"
)

func init() {
	checks.MustRegister(errorsCheck{run: platform.RunRead})
}
