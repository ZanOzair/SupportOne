// Package profile compares a machine against what a technician says it should
// look like.
//
// Provisioning in most tools means "make this machine match" — a script that
// runs and changes things until the shapes line up. This does the read-only
// half, deliberately, and stops there. It says what differs and what could be
// done about it; anything that would actually change the machine goes through
// the same consent gate as every other change, one action at a time, described
// and confirmed.
//
// A profile is a file a technician writes. It names check IDs and the worst
// verdict each is allowed to reach, and may name fixes to offer where an
// expectation is not met. Those fixes are resolved through the registry, so a
// profile can never name an action this binary was not built with — a profile
// is data, not a program, and cannot become one.
package profile

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Schema is the version of the profile format.
const Schema = 1

// MaxExpectations bounds how many rules one profile may carry. A profile is a
// standard a technician wrote, not a generated policy dump.
const MaxExpectations = 200

// Expectation is one rule: a check, and the worst verdict it may reach.
type Expectation struct {
	// Check is the ID of a compiled-in check. One that is not compiled in is
	// reported as unknown rather than silently passing.
	Check string `json:"check"`

	// Worst is the worst severity this check may report and still count as
	// met. "ok" means it must be clean; "attention" tolerates a warning.
	Worst checks.Severity `json:"worst"`

	// Why is the technician's own note about the rule, shown with the result
	// so a reader knows the reason and not only the rule.
	Why string `json:"why,omitempty"`

	// Offer names fixes to suggest where this is not met. They are resolved
	// through the registry before anything is shown.
	Offer []string `json:"offer,omitempty"`
}

// Profile is what a technician says a machine should look like.
type Profile struct {
	Schema int    `json:"schema"`
	Name   string `json:"name"`

	// Expectations are the rules, in the order they should be read.
	Expectations []Expectation `json:"expectations"`
}

// Load reads a profile and refuses one it cannot act on honestly.
func Load(r io.Reader) (Profile, error) {
	var p Profile

	decoder := json.NewDecoder(io.LimitReader(r, 1<<20))
	// A misspelled field is a rule that would silently not apply, which in a
	// compliance document is worse than a parse error.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return Profile{}, fmt.Errorf("profile: read the profile: %w", err)
	}

	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Validate reports whether this profile can be applied honestly.
func (p Profile) Validate() error {
	switch {
	case p.Schema != Schema:
		return fmt.Errorf("profile: this file says schema %d and this build understands %d", p.Schema, Schema)
	case strings.TrimSpace(p.Name) == "":
		return fmt.Errorf("profile: a profile needs a name, so a report can say which standard it was measured against")
	case len(p.Expectations) == 0:
		return fmt.Errorf("profile: a profile with no expectations would report everything as met")
	case len(p.Expectations) > MaxExpectations:
		return fmt.Errorf("profile: %d expectations is more than the %d this reads", len(p.Expectations), MaxExpectations)
	}

	seen := make(map[string]bool, len(p.Expectations))
	for _, e := range p.Expectations {
		id := strings.TrimSpace(e.Check)
		switch {
		case id == "":
			return fmt.Errorf("profile: an expectation names no check")
		case seen[id]:
			// Two rules for one check would let a profile contradict itself
			// and report both answers.
			return fmt.Errorf("profile: %q has two expectations", id)
		case !e.Worst.Valid():
			return fmt.Errorf("profile: %q allows the severity %q, which is not one", id, e.Worst)
		}
		seen[id] = true
	}
	return nil
}

// State is what happened to one expectation.
type State string

// The states an expectation can end in.
const (
	// StateMet means the check ran and its verdict is within what the
	// profile allows.
	StateMet State = "met"

	// StateUnmet means the check ran and its verdict is worse than allowed.
	StateUnmet State = "unmet"

	// StateUnknown means the check could not answer. It is never counted as
	// met: a check that did not run is not a check that passed.
	StateUnknown State = "unknown"

	// StateMissing means the profile names a check this build does not
	// carry, or that did not run on this platform.
	StateMissing State = "missing"
)

// Finding is one expectation measured against this machine.
type Finding struct {
	Check string          `json:"check"`
	State State           `json:"state"`
	Worst checks.Severity `json:"worst_allowed"`

	// Actual is what the check reported, empty when it did not run.
	Actual checks.Severity `json:"actual,omitempty"`

	// Summary is the check's own message key, so a report renders the
	// finding in the reader's language.
	Summary string `json:"summary,omitempty"`
	Args    []any  `json:"summary_args,omitempty"`

	Why string `json:"why,omitempty"`

	// Offer is the fixes this build can actually run for it.
	Offer []string `json:"offer,omitempty"`
}

// Report is a machine measured against a profile.
type Report struct {
	Profile  string    `json:"profile"`
	Findings []Finding `json:"findings"`

	Met     int `json:"met"`
	Unmet   int `json:"unmet"`
	Unknown int `json:"unknown"`
	Missing int `json:"missing"`
}

// Conforms reports whether every expectation was met.
//
// Unknown and missing both count against it. A standard that treated "could
// not check" as compliance would certify machines nobody looked at.
func (r Report) Conforms() bool {
	return r.Unmet == 0 && r.Unknown == 0 && r.Missing == 0
}

// severityRank orders verdicts from best to worst, so "worse than allowed" is
// a comparison rather than a table of special cases.
var severityRank = map[checks.Severity]int{
	checks.SeverityOK:        0,
	checks.SeverityAttention: 1,
	checks.SeverityUrgent:    2,
}

// Measure compares a snapshot against the profile.
//
// The registry and platform decide what may be offered, so a profile written
// for a Windows fleet does not offer Windows-only repairs on a Mac.
func (p Profile) Measure(snap checks.Snapshot, registry *fixes.Registry, os platform.OS) Report {
	results := make(map[string]checks.Result, len(snap.Results))
	for _, res := range snap.Results {
		results[res.CheckID] = res
	}

	skipped := make(map[string]bool, len(snap.SkippedAdmin))
	for _, id := range snap.SkippedAdmin {
		skipped[id] = true
	}

	report := Report{Profile: p.Name}
	for _, e := range p.Expectations {
		report.Findings = append(report.Findings, measureOne(e, results, skipped, registry, os))
	}

	for _, f := range report.Findings {
		switch f.State {
		case StateMet:
			report.Met++
		case StateUnmet:
			report.Unmet++
		case StateUnknown:
			report.Unknown++
		case StateMissing:
			report.Missing++
		}
	}
	return report
}

func measureOne(
	e Expectation,
	results map[string]checks.Result,
	skipped map[string]bool,
	registry *fixes.Registry,
	os platform.OS,
) Finding {
	finding := Finding{Check: e.Check, Worst: e.Worst, Why: e.Why}

	res, ran := results[e.Check]
	switch {
	case !ran && skipped[e.Check]:
		// The check exists and was skipped for want of rights. That is a
		// question we did not answer, not one we answered well.
		finding.State = StateUnknown
	case !ran:
		finding.State = StateMissing
		return finding
	default:
		finding.Actual = res.Severity
		finding.Summary = res.Summary
		finding.Args = res.Args

		switch {
		case res.Severity == checks.SeverityUnknown:
			finding.State = StateUnknown
		case severityRank[res.Severity] <= severityRank[e.Worst]:
			finding.State = StateMet
		default:
			finding.State = StateUnmet
		}
	}

	if finding.State != StateMet {
		finding.Offer = offerable(e.Offer, registry, os)
	}
	return finding
}

// offerable keeps only the fixes this build carries and this platform runs.
// A profile is data: it can name anything, and only what the registry knows
// survives to be shown.
func offerable(candidates []string, registry *fixes.Registry, os platform.OS) []string {
	if registry == nil || len(candidates) == 0 {
		return nil
	}

	known, _ := registry.Resolve(candidates, os)
	out := make([]string, 0, len(known))
	for _, f := range known {
		out = append(out, f.ID())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Ordered returns findings worst-first, so what fails a standard is read
// before what passes it.
func Ordered(findings []Finding) []Finding {
	rank := map[State]int{StateUnmet: 0, StateUnknown: 1, StateMissing: 2, StateMet: 3}

	out := append([]Finding(nil), findings...)
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].State] < rank[out[j].State] })
	return out
}
