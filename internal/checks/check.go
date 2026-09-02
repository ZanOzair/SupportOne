// Package checks defines the diagnostic plugin contract and the registry that
// holds every compiled-in check.
//
// A Check is strictly read-only: it observes the machine and reports what it
// found. Anything that changes the machine is a Fix and lives in
// internal/fixes, behind explicit per-action consent.
package checks

import (
	"context"
	"time"

	"github.com/ZanOzair/supportone/internal/platform"
)

// Severity is the traffic-light verdict of a single check.
//
// A check reports Unknown when it could not determine an answer — because it
// needed rights it did not have, or the OS did not expose the data. It never
// reports OK in that case, and it never invents a problem to look useful.
type Severity string

// The severities a check can report.
const (
	SeverityOK        Severity = "ok"
	SeverityAttention Severity = "attention"
	SeverityUrgent    Severity = "urgent"
	SeverityUnknown   Severity = "unknown"
)

// Valid reports whether s is a defined severity.
func (s Severity) Valid() bool {
	switch s {
	case SeverityOK, SeverityAttention, SeverityUrgent, SeverityUnknown:
		return true
	default:
		return false
	}
}

// Result is what a check found.
type Result struct {
	CheckID  string   `json:"check_id"`
	Severity Severity `json:"severity"`

	// Summary is one sentence a non-technical reader can act on. It is a
	// message key resolved through internal/i18n, not English prose, so the
	// same result renders in every supported language.
	Summary string `json:"summary"`

	// Detail carries the structured evidence behind Summary. It is what the
	// report renders and what the user reviews and redacts before any send.
	Detail map[string]any `json:"detail,omitempty"`

	// Err records why a check could not complete, when Severity is Unknown.
	Err string `json:"error,omitempty"`

	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration_ns"`
}

// Check is a read-only diagnostic module.
//
// Implementations MUST NOT modify system state, MUST return promptly when ctx
// is cancelled, and MUST report Unknown rather than guessing when the OS does
// not give them an answer.
type Check interface {
	// ID is stable and dotted, e.g. "disk.smart". It is the key reports,
	// the Tier-1 explainer and the audit log all join on, so it never changes
	// once released.
	ID() string

	// Platforms lists the operating systems this check runs on. A check is
	// only offered where it can give an honest answer; elsewhere the report
	// says the check is unavailable on this platform rather than showing a
	// fabricated result.
	Platforms() []platform.OS

	// RequiresAdmin reports whether the check needs elevated rights to read
	// what it reads. Read-only and elevated are not a contradiction: some
	// data (SMART attributes on Windows, for one) is simply not readable
	// without it.
	RequiresAdmin() bool

	Run(ctx context.Context) (Result, error)
}

// RunsOn reports whether c is available on os.
func RunsOn(c Check, os platform.OS) bool {
	for _, p := range c.Platforms() {
		if p == os {
			return true
		}
	}
	return false
}
