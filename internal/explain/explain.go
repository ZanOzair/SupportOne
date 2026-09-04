// Package explain turns a check's verdict into plain language and a short list
// of things to try, using nothing but a table compiled into the binary.
//
// This is Tier 1, and it is the tier that has to make the product useful on
// its own. It works with the network unplugged, needs no API key, costs
// nothing to run, and gives the same answer every time for the same finding.
// Whatever an optional model is later asked to add, this is what the user gets
// when there is no model, no key and no connection — which is most of the time
// and, for many people, all of the time.
//
// Two rules keep it honest. Every step is a message key, so the advice reads in
// the user's language rather than in English pasted into a template. And every
// fix or walkthrough an explanation points at is resolved through the same
// registries the rest of the agent uses, so an explanation can never name
// something this binary was not built with.
package explain

import (
	"sort"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

// Advice is what SupportOne says about one finding, offline.
type Advice struct {
	CheckID  string          `json:"check_id"`
	Severity checks.Severity `json:"severity"`

	// Cause is a message key: what this finding usually means, in terms the
	// person holding the machine can act on.
	Cause string `json:"cause"`

	// Steps are message keys, in the order they are worth trying. The order
	// is part of the advice: "back up first" before "replace the drive" is
	// not the same advice reversed.
	Steps []string `json:"steps,omitempty"`

	// Fixes are the repairs this build can actually offer for this finding.
	// They are resolved against the registry and this platform, so an entry
	// here is always something the user can press.
	Fixes []string `json:"fixes,omitempty"`

	// Wizards are the walkthroughs worth starting, resolved the same way.
	Wizards []string `json:"wizards,omitempty"`

	// Escalate says this is past what someone should work through alone. It
	// is set for findings that risk data or that no safe local action fixes.
	Escalate bool `json:"escalate"`
}

// KeyEscalate is the closing line on any advice marked Escalate.
const KeyEscalate = "explain.escalate"

// Explainer answers for one platform, against one set of registries.
type Explainer struct {
	fixes   *fixes.Registry
	wizards *wizard.Registry
	os      platform.OS
}

// New returns an explainer. Nil registries are allowed: a build with no fixes
// still explains every finding, it just has nothing to offer pressing.
func New(f *fixes.Registry, w *wizard.Registry, os platform.OS) *Explainer {
	return &Explainer{fixes: f, wizards: w, os: os}
}

// For explains one result. The second return is false only for a verdict with
// no rule, which a guard test makes impossible to ship.
func (e *Explainer) For(res checks.Result) (Advice, bool) {
	r, ok := rules[Base(res.Summary)]
	if !ok {
		return Advice{}, false
	}

	advice := Advice{
		CheckID:  res.CheckID,
		Severity: res.Severity,
		Cause:    CauseKey(Base(res.Summary)),
		Steps:    append([]string(nil), r.Steps...),
		Escalate: r.Escalate,
	}
	if r.Escalate {
		// The closing line is part of the advice, not a footnote the
		// interface has to remember to add.
		advice.Steps = append(advice.Steps, KeyEscalate)
	}

	advice.Fixes = e.knownFixes(r.Fixes)
	advice.Wizards = e.knownWizards(r.Wizards)
	return advice, true
}

// ForSnapshot explains every result in a snapshot, in the snapshot's order.
//
// Every result is explained, including the ones that are fine: "this was
// checked and here is what that means" is worth as much to a worried person as
// a warning is, and a report that only speaks up about problems teaches people
// that silence means it did not look.
func (e *Explainer) ForSnapshot(snap checks.Snapshot) []Advice {
	out := make([]Advice, 0, len(snap.Results))
	for _, res := range snap.Results {
		if advice, ok := e.For(res); ok {
			out = append(out, advice)
		}
	}
	return out
}

// Ordered returns advice worst-first, so what matters is read first. Results
// of equal severity keep their snapshot order, which is stable.
func Ordered(advice []Advice) []Advice {
	rank := map[checks.Severity]int{
		checks.SeverityUrgent:    0,
		checks.SeverityAttention: 1,
		checks.SeverityUnknown:   2,
		checks.SeverityOK:        3,
	}

	out := append([]Advice(nil), advice...)
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}

// knownFixes keeps only the fixes this build has and this platform runs. An ID
// the table names but the binary does not carry is dropped here rather than
// shown to the user as a button that cannot work.
func (e *Explainer) knownFixes(candidates []string) []string {
	if e.fixes == nil || len(candidates) == 0 {
		return nil
	}

	known, _ := e.fixes.Resolve(candidates, e.os)
	out := make([]string, 0, len(known))
	for _, f := range known {
		out = append(out, f.ID())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// knownWizards does the same for walkthroughs.
func (e *Explainer) knownWizards(candidates []string) []string {
	if e.wizards == nil || len(candidates) == 0 {
		return nil
	}

	var out []string
	for _, id := range candidates {
		w, ok := e.wizards.Get(id)
		if !ok || !w.RunsOn(e.os) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// Base strips the plural variant from a verdict key, so "…found.one" and
// "…found" are the same finding and share one rule.
func Base(summary string) string {
	return strings.TrimSuffix(summary, ".one")
}

// CauseKey derives the explanation key from the verdict key it explains:
// "check.disk.smart.failing" is explained by "explain.disk.smart.failing".
//
// Deriving it rather than writing it twice means the two cannot drift apart,
// and a rule for a verdict that has no explanation fails the guard test.
func CauseKey(verdict string) string {
	return "explain." + strings.TrimPrefix(verdict, "check.")
}

// sortStrings is sort.Strings, named here so rules.go does not import sort for
// one call.
func sortStrings(s []string) { sort.Strings(s) }
