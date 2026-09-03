// Package wizard walks a user through one problem, one step at a time.
//
// A wizard is not a script that runs a list of repairs. It is a sequence of
// read-only questions, each with one thing to try if the answer is wrong, and
// the rule that binds them: after anything is changed, the same question is
// asked again. A step that was tried and did not help is recorded as tried and
// did not help. Nothing is ever reported as fixed on the strength of having
// been attempted.
//
// When the wizard runs out of things it knows how to check, it says so and
// hands over. "I found nothing wrong and this still needs a person" is a
// result, and a more useful one than a confident guess.
package wizard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/remediate"
)

// Outcome is how a wizard session ended.
type Outcome string

// The ways a session can end.
const (
	// OutcomeFixed means something was found wrong, something was changed,
	// and asking again showed it was no longer wrong.
	OutcomeFixed Outcome = "fixed"

	// OutcomeUnresolved means something was found wrong and is still wrong:
	// the fix was declined, was not available, or did not help.
	OutcomeUnresolved Outcome = "unresolved"

	// OutcomeUnverified means a change was made that this tool cannot check
	// the effect of. Only the user, by trying the thing again, can say
	// whether it helped — and saying so is more useful than claiming a
	// repair nothing confirmed.
	OutcomeUnverified Outcome = "unverified"

	// OutcomeNoFault means every question this wizard knows to ask came back
	// clean. The problem is real and this tool cannot see it.
	OutcomeNoFault Outcome = "no_fault"

	// OutcomeStopped means the user ended the session part-way.
	OutcomeStopped Outcome = "stopped"

	// OutcomeRunning means the session has not finished.
	OutcomeRunning Outcome = "running"
)

// Message keys the engine itself resolves through internal/i18n.
const (
	KeyOutcomeFixed      = "wizard.outcome.fixed"
	KeyOutcomeUnresolved = "wizard.outcome.unresolved"
	KeyOutcomeUnverified = "wizard.outcome.unverified"
	KeyOutcomeNoFault    = "wizard.outcome.no_fault"
	KeyOutcomeStopped    = "wizard.outcome.stopped"

	KeyStepChecking = "wizard.step.checking"
	KeyStepClean    = "wizard.step.clean"
	KeyStepTried    = "wizard.step.tried"
	KeyStepApplied  = "wizard.step.applied"
	KeyStepNoHelp   = "wizard.step.no_help"
	KeyStepDeclined = "wizard.step.declined"
	KeyStepBlocked  = "wizard.step.blocked"
)

// Finding is what one question answered.
type Finding struct {
	// OK reports that this particular thing is not the problem.
	OK bool `json:"ok"`

	// Summary is a message key describing what was found, in the user's
	// language, whether or not anything is wrong.
	Summary string `json:"summary"`

	// Args fill the placeholders in Summary.
	Args []any `json:"summary_args,omitempty"`

	// Unknown reports that the question could not be answered. It is never
	// treated as OK: a question that was not answered is not a question that
	// passed.
	Unknown bool `json:"unknown"`
}

// Probe answers one read-only question about the machine. It must not change
// anything.
type Probe func(ctx context.Context) (Finding, error)

// Step is one question, with at most one thing to try when the answer is
// wrong.
type Step struct {
	// ID is stable and dotted, so the audit log and the escalation report
	// join on it.
	ID string

	// Title is a message key naming what this step looks at.
	Title string

	// Ask is the read-only question.
	Ask Probe

	// FixID names the compiled-in fix that addresses this finding, or is
	// empty when there is nothing the agent can safely do. It is resolved
	// through the fix registry, so a step cannot name a fix that was not
	// compiled in.
	FixID string

	// Advice is a message key telling the user what to do themselves. It is
	// what a step offers when there is no fix, and what it adds when there is.
	Advice string

	// Unverifiable says that asking again cannot tell whether this step's fix
	// worked. Some things — a cache that refills itself the moment it is
	// used — look identical after a successful repair and after a useless
	// one. Such a step is recorded as changed, never as fixed: claiming a
	// repair nothing confirmed is the one claim this tool does not make.
	Unverifiable bool
}

// Wizard is an ordered set of steps for one complaint.
type Wizard struct {
	// ID is stable and dotted, e.g. "wizard.printing".
	ID string

	// Title and Complaint are message keys: what this wizard is for, in the
	// words a user would use to describe the problem.
	Title     string
	Complaint string

	// Platforms lists where this wizard's questions can be answered.
	Platforms []platform.OS

	Steps []Step
}

// RunsOn reports whether w is available on os.
func (w *Wizard) RunsOn(os platform.OS) bool {
	for _, p := range w.Platforms {
		if p == os {
			return true
		}
	}
	return false
}

// StepStatus is what happened at one step.
type StepStatus string

// The states a step passes through.
const (
	StatusClean    StepStatus = "clean"    // asked, nothing wrong
	StatusFound    StepStatus = "found"    // asked, something wrong, awaiting a decision
	StatusFixed    StepStatus = "fixed"    // changed, and asking again came back clean
	StatusApplied  StepStatus = "applied"  // changed, and asking again cannot tell whether it helped
	StatusNoHelp   StepStatus = "no_help"  // changed, and asking again found the same thing
	StatusDeclined StepStatus = "declined" // the user chose not to
	StatusBlocked  StepStatus = "blocked"  // there was nothing the agent could offer
	StatusUnknown  StepStatus = "unknown"  // the question could not be answered
)

// AllStatuses returns every state a step can end in. The interface renders a
// label for each, so this is what a guard test checks the catalogs against.
func AllStatuses() []StepStatus {
	return []StepStatus{
		StatusClean, StatusFound, StatusFixed, StatusApplied,
		StatusNoHelp, StatusDeclined, StatusBlocked, StatusUnknown,
	}
}

// Record is the history of one step, for the user and for a technician.
type Record struct {
	StepID  string     `json:"step_id"`
	Title   string     `json:"title"`
	Status  StepStatus `json:"status"`
	Finding Finding    `json:"finding"`

	// FixID is what was applied, when something was.
	FixID string `json:"fix_id,omitempty"`

	// Err records why a question could not be answered.
	Err string `json:"error,omitempty"`
}

// Progress is what the interface shows after each move.
type Progress struct {
	WizardID string `json:"wizard_id"`

	// Step is the step awaiting the user, when one is.
	Step *Record `json:"step,omitempty"`

	// Offer is the plan the user must confirm before the wizard changes
	// anything. It is nil when the step has nothing to offer.
	Offer *remediate.Plan `json:"offer,omitempty"`

	// Advice is a message key for what the user can do themselves.
	Advice string `json:"advice,omitempty"`

	// Outcome is OutcomeRunning until the session ends.
	Outcome Outcome `json:"outcome"`

	// Done lists every step already behind the session.
	Done []Record `json:"done"`
}

// Session is one run of one wizard.
type Session struct {
	wizard  *Wizard
	applier *remediate.Applier
	timeout time.Duration

	at      int
	done    []Record
	pending *Record
	outcome Outcome
}

// DefaultTimeout bounds a single question.
const DefaultTimeout = 30 * time.Second

// Start begins a session. The applier is the only way this session can change
// anything, and it is the same consent gate everything else goes through.
func Start(w *Wizard, applier *remediate.Applier, timeout time.Duration) *Session {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Session{wizard: w, applier: applier, timeout: timeout, outcome: OutcomeRunning}
}

// ErrFinished is returned by a move on a session that has already ended.
var ErrFinished = errors.New("wizard: this session has finished")

// ErrNothingOffered is returned when Confirm is called on a step that offered
// nothing to confirm.
var ErrNothingOffered = errors.New("wizard: this step offered nothing to confirm")

// Next asks questions until one of them finds something, or until there are no
// questions left.
//
// A step that comes back clean is recorded and passed over: the user is shown
// what needs their attention, not a list of things that were fine.
func (s *Session) Next(ctx context.Context) (Progress, error) {
	if s.outcome != OutcomeRunning {
		return s.progress(), ErrFinished
	}

	for s.at < len(s.wizard.Steps) {
		step := s.wizard.Steps[s.at]
		record := s.ask(ctx, step)

		if record.Status == StatusClean {
			s.done = append(s.done, record)
			s.at++
			continue
		}

		s.pending = &record
		return s.offer(ctx, step, record)
	}

	s.finish()
	return s.progress(), nil
}

// ask runs one step's question and turns it into a record.
func (s *Session) ask(ctx context.Context, step Step) Record {
	record := Record{StepID: step.ID, Title: step.Title}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	finding, err := askGuarded(ctx, step)
	switch {
	case err != nil:
		// A question that could not be answered is never treated as one that
		// passed.
		record.Status = StatusUnknown
		record.Err = err.Error()
		record.Finding = Finding{Unknown: true}
	case finding.Unknown:
		record.Status = StatusUnknown
		record.Finding = finding
	case finding.OK:
		record.Status = StatusClean
		record.Finding = finding
	default:
		record.Status = StatusFound
		record.Finding = finding
	}
	return record
}

// askGuarded turns a panicking probe into an error, so one bad step cannot end
// a session the user is relying on.
func askGuarded(ctx context.Context, step Step) (f Finding, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("wizard: step %s panicked: %v", step.ID, r)
		}
	}()
	return step.Ask(ctx)
}

// offer builds the progress for a step that found something: the plan for its
// fix, when it has one the agent can apply, and the advice either way.
func (s *Session) offer(ctx context.Context, step Step, record Record) (Progress, error) {
	out := s.progress()
	out.Step = &record
	out.Advice = step.Advice

	if step.FixID == "" || s.applier == nil {
		return out, nil
	}

	// The fix is resolved through the registry here, not named directly: a
	// step cannot reach a fix that was never compiled in.
	plan, err := s.applier.Plan(ctx, step.FixID)
	if err != nil {
		// The step knows of a fix that does not run here. That is worth
		// saying plainly rather than pretending the step had nothing.
		out.Step.Err = err.Error()
		return out, nil
	}
	if !plan.Applicable() {
		out.Offer = &plan
		return out, nil
	}

	out.Offer = &plan
	return out, nil
}

// Confirm applies the offered fix and asks the same question again.
//
// The second question is the point. A fix that ran is not a problem that went
// away, and only the re-check can tell the difference.
func (s *Session) Confirm(ctx context.Context, c remediate.Confirmation) (Progress, error) {
	if s.outcome != OutcomeRunning {
		return s.progress(), ErrFinished
	}
	if s.pending == nil || s.at >= len(s.wizard.Steps) {
		return s.progress(), ErrNothingOffered
	}

	step := s.wizard.Steps[s.at]
	if step.FixID == "" || s.applier == nil {
		return s.progress(), ErrNothingOffered
	}

	record := *s.pending
	record.FixID = step.FixID

	if _, err := s.applier.Apply(ctx, c); err != nil {
		record.Status = StatusBlocked
		record.Err = err.Error()
		s.close(record)
		return s.Next(ctx)
	}

	if step.Unverifiable {
		// Asking again would tell us nothing, so it is not asked, and the
		// step is recorded as changed rather than as fixed.
		record.Status = StatusApplied
		s.close(record)
		return s.Next(ctx)
	}

	// Ask again. This is the only thing that decides whether it worked.
	after := s.ask(ctx, step)
	record.Finding = after.Finding
	switch after.Status {
	case StatusClean:
		record.Status = StatusFixed
	case StatusUnknown:
		record.Status = StatusUnknown
		record.Err = after.Err
	default:
		record.Status = StatusNoHelp
	}

	s.close(record)
	return s.Next(ctx)
}

// Skip moves past the current step without changing anything.
func (s *Session) Skip(ctx context.Context) (Progress, error) {
	if s.outcome != OutcomeRunning {
		return s.progress(), ErrFinished
	}
	if s.pending == nil {
		return s.Next(ctx)
	}

	record := *s.pending
	if record.Status == StatusFound {
		record.Status = StatusDeclined
	}
	s.close(record)
	return s.Next(ctx)
}

// Stop ends the session where it is.
func (s *Session) Stop() Progress {
	if s.outcome == OutcomeRunning {
		if s.pending != nil {
			s.close(*s.pending)
		}
		s.outcome = OutcomeStopped
	}
	return s.progress()
}

// close files the current step and moves on.
func (s *Session) close(record Record) {
	s.done = append(s.done, record)
	s.pending = nil
	s.at++
}

// finish decides how the session ended, from what actually happened rather
// than from what was attempted.
func (s *Session) finish() {
	var found, applied, outstanding bool
	for _, r := range s.done {
		switch r.Status {
		case StatusClean:
		case StatusFixed:
			found = true
		case StatusApplied:
			found, applied = true, true
		case StatusFound, StatusNoHelp, StatusDeclined, StatusBlocked, StatusUnknown:
			found, outstanding = true, true
		}
	}

	switch {
	case !found:
		// Every question came back clean. The complaint is still real, and
		// this tool cannot see why.
		s.outcome = OutcomeNoFault
	case outstanding:
		s.outcome = OutcomeUnresolved
	case applied:
		// At least one change was made that nothing can confirm the effect
		// of. Even alongside a repair that was confirmed, that is not a
		// session we can call finished.
		s.outcome = OutcomeUnverified
	default:
		s.outcome = OutcomeFixed
	}
}

func (s *Session) progress() Progress {
	return Progress{
		WizardID: s.wizard.ID,
		Outcome:  s.outcome,
		Done:     append([]Record(nil), s.done...),
	}
}

// Outcome returns how the session ended, or OutcomeRunning.
func (s *Session) Outcome() Outcome { return s.outcome }

// Escalation is the handover a technician reads: the complaint, every question
// asked, and what came of it.
type Escalation struct {
	WizardID  string   `json:"wizard_id"`
	Complaint string   `json:"complaint"`
	Outcome   Outcome  `json:"outcome"`
	Steps     []Record `json:"steps"`
}

// Escalate returns the session's history. It carries message keys and step
// IDs, not machine identifiers: what to send, and whether to send it, stays
// the user's decision.
func (s *Session) Escalate() Escalation {
	return Escalation{
		WizardID:  s.wizard.ID,
		Complaint: s.wizard.Complaint,
		Outcome:   s.outcome,
		Steps:     append([]Record(nil), s.done...),
	}
}

// OutcomeKey maps an outcome to the message key that explains it.
func OutcomeKey(o Outcome) string {
	switch o {
	case OutcomeFixed:
		return KeyOutcomeFixed
	case OutcomeUnresolved:
		return KeyOutcomeUnresolved
	case OutcomeUnverified:
		return KeyOutcomeUnverified
	case OutcomeNoFault:
		return KeyOutcomeNoFault
	case OutcomeStopped:
		return KeyOutcomeStopped
	default:
		return ""
	}
}
