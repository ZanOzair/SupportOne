package wizard

import (
	"context"
	"errors"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/remediate"
	"github.com/ZanOzair/SupportOne/internal/restore"
)

// answers turns a list of findings into a probe that returns them in order,
// repeating the last one. That is how a step is asked again after a fix.
func answers(findings ...Finding) (Probe, *int) {
	calls := 0
	return func(context.Context) (Finding, error) {
		f := findings[min(calls, len(findings)-1)]
		calls++
		return f, nil
	}, &calls
}

func clean() Finding  { return Finding{OK: true, Summary: "step.clean"} }
func broken() Finding { return Finding{Summary: "step.broken"} }

// stubFix is a fix that does nothing but succeed.
type stubFix struct{ id string }

func (f stubFix) ID() string { return f.id }
func (f stubFix) Describe() fixes.Explanation {
	return fixes.Explanation{Summary: "fix.stub.summary", Changes: []string{"fix.stub.change"}, Undo: "fix.stub.undo"}
}
func (f stubFix) Platforms() []platform.OS        { return platform.All() }
func (f stubFix) RequiresAdmin() bool             { return false }
func (f stubFix) Reversible() bool                { return true }
func (f stubFix) Preflight(context.Context) error { return nil }
func (f stubFix) Rollback(context.Context) error  { return nil }
func (f stubFix) Apply(context.Context) (fixes.Outcome, error) {
	return fixes.Outcome{Applied: true}, nil
}

// alwaysMaker is a restore mechanism that always works.
type alwaysMaker struct{}

func (alwaysMaker) Check(context.Context) restore.Availability {
	return restore.Availability{Available: true, Kind: "stub"}
}
func (alwaysMaker) Create(context.Context, string) (restore.Point, error) {
	return restore.Point{Kind: "stub"}, nil
}

func newApplier(t *testing.T, ids ...string) *remediate.Applier {
	t.Helper()

	registry := fixes.NewRegistry()
	for _, id := range ids {
		if err := registry.Register(stubFix{id: id}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	log, err := consent.Open(t.TempDir() + "/audit.log")
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	a := remediate.New(registry, log, alwaysMaker{}, platform.Current())
	a.Elevated = func() (bool, error) { return true, nil }
	return a
}

func confirm(p *remediate.Plan) remediate.Confirmation {
	return remediate.Confirmation{Token: p.Token, Acknowledged: append([]string(nil), p.Explanation.Changes...)}
}

func TestASessionThatFindsNothingSaysSoRatherThanClaimingSuccess(t *testing.T) {
	askA, _ := answers(clean())
	askB, _ := answers(clean())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{
			{ID: "stub.one", Title: "one", Ask: askA, Advice: "a"},
			{ID: "stub.two", Title: "two", Ask: askB, Advice: "a"},
		},
	}

	s := Start(w, nil, 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	// "I looked at everything I know how to look at and found nothing" is a
	// result. Calling it a repair would not be.
	if got.Outcome != OutcomeNoFault {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeNoFault)
	}
	if len(got.Done) != 2 {
		t.Errorf("recorded %d steps, want 2", len(got.Done))
	}
	if got.Step != nil {
		t.Error("a step is awaiting the user though nothing was found")
	}
}

func TestAStepThatFindsSomethingStopsAndOffersTheFix(t *testing.T) {
	askA, _ := answers(clean())
	askB, _ := answers(broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{
			{ID: "stub.one", Title: "one", Ask: askA, Advice: "a"},
			{ID: "stub.two", Title: "two", Ask: askB, FixID: "stub.fix", Advice: "advice.key"},
		},
	}

	s := Start(w, newApplier(t, "stub.fix"), 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	if got.Outcome != OutcomeRunning {
		t.Errorf("Outcome = %q, want the session still running", got.Outcome)
	}
	if got.Step == nil || got.Step.StepID != "stub.two" {
		t.Fatalf("Step = %+v, want the step that found something", got.Step)
	}
	if got.Step.Status != StatusFound {
		t.Errorf("Status = %q, want %q", got.Step.Status, StatusFound)
	}
	if got.Offer == nil {
		t.Fatal("no fix was offered for a step that names one")
	}
	if got.Offer.FixID != "stub.fix" {
		t.Errorf("offered %q, want %q", got.Offer.FixID, "stub.fix")
	}
	if got.Advice != "advice.key" {
		t.Errorf("Advice = %q, want the step's advice alongside the offer", got.Advice)
	}
	// The clean step is behind us and is not shown as needing attention.
	if len(got.Done) != 1 || got.Done[0].Status != StatusClean {
		t.Errorf("Done = %+v, want the one clean step", got.Done)
	}
}

func TestAFixIsOnlyCalledFixedWhenAskingAgainAgrees(t *testing.T) {
	ask, calls := answers(broken(), clean())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{ID: "stub.one", Title: "one", Ask: ask, FixID: "stub.fix", Advice: "a"}},
	}

	s := Start(w, newApplier(t, "stub.fix"), 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	got, err = s.Confirm(context.Background(), confirm(got.Offer))
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	if *calls != 2 {
		t.Errorf("the step was asked %d times, want it asked again after the fix", *calls)
	}
	if len(got.Done) != 1 || got.Done[0].Status != StatusFixed {
		t.Fatalf("Done = %+v, want one step recorded as fixed", got.Done)
	}
	if got.Done[0].FixID != "stub.fix" {
		t.Errorf("the record does not say what was applied: %+v", got.Done[0])
	}
	if got.Outcome != OutcomeFixed {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeFixed)
	}
}

// TestAFixThatDidNotHelpIsRecordedAsNotHavingHelped is the rule the whole
// engine exists for.
func TestAFixThatDidNotHelpIsRecordedAsNotHavingHelped(t *testing.T) {
	ask, calls := answers(broken(), broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{ID: "stub.one", Title: "one", Ask: ask, FixID: "stub.fix", Advice: "a"}},
	}

	s := Start(w, newApplier(t, "stub.fix"), 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	got, err = s.Confirm(context.Background(), confirm(got.Offer))
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	if *calls != 2 {
		t.Errorf("the step was asked %d times, want it asked again after the fix", *calls)
	}
	if len(got.Done) != 1 || got.Done[0].Status != StatusNoHelp {
		t.Fatalf("Done = %+v, want the step recorded as no help", got.Done)
	}
	if got.Outcome != OutcomeUnresolved {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeUnresolved)
	}
}

// TestAnUnverifiableFixIsNeverReportedAsAConfirmedRepair covers the case where
// asking again cannot tell the difference — a cache that refills itself.
func TestAnUnverifiableFixIsNeverReportedAsAConfirmedRepair(t *testing.T) {
	ask, calls := answers(broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{
			ID: "stub.one", Title: "one", Ask: ask,
			FixID: "stub.fix", Advice: "a", Unverifiable: true,
		}},
	}

	s := Start(w, newApplier(t, "stub.fix"), 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	got, err = s.Confirm(context.Background(), confirm(got.Offer))
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	// Asking again would prove nothing, so it is not asked.
	if *calls != 1 {
		t.Errorf("the step was asked %d times, want it not asked again", *calls)
	}
	if len(got.Done) != 1 || got.Done[0].Status != StatusApplied {
		t.Fatalf("Done = %+v, want the step recorded as applied, not fixed", got.Done)
	}
	if got.Outcome != OutcomeUnverified {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeUnverified)
	}
}

func TestAConfirmedFixThatWillNotApplyIsRecordedAsBlocked(t *testing.T) {
	ask, _ := answers(broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{ID: "stub.one", Title: "one", Ask: ask, FixID: "stub.fix", Advice: "a"}},
	}

	s := Start(w, newApplier(t, "stub.fix"), 0)
	if _, err := s.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}

	// An acknowledgement that does not match what was shown: the consent gate
	// refuses, and the wizard says so rather than moving on quietly.
	got, err := s.Confirm(context.Background(), remediate.Confirmation{Token: "wrong"})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if len(got.Done) != 1 || got.Done[0].Status != StatusBlocked {
		t.Fatalf("Done = %+v, want the step recorded as blocked", got.Done)
	}
	if got.Done[0].Err == "" {
		t.Error("the record does not say why the change did not happen")
	}
	if got.Outcome != OutcomeUnresolved {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeUnresolved)
	}
}

func TestSkipRecordsThatTheUserChoseNotTo(t *testing.T) {
	ask, calls := answers(broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{ID: "stub.one", Title: "one", Ask: ask, FixID: "stub.fix", Advice: "a"}},
	}

	s := Start(w, newApplier(t, "stub.fix"), 0)
	if _, err := s.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}

	got, err := s.Skip(context.Background())
	if err != nil {
		t.Fatalf("Skip: %v", err)
	}
	if *calls != 1 {
		t.Errorf("the step was asked %d times; skipping must change nothing", *calls)
	}
	if len(got.Done) != 1 || got.Done[0].Status != StatusDeclined {
		t.Fatalf("Done = %+v, want the step recorded as declined", got.Done)
	}
	if got.Outcome != OutcomeUnresolved {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeUnresolved)
	}
}

func TestAQuestionThatCouldNotBeAnsweredIsNeverTreatedAsClean(t *testing.T) {
	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{
			ID:    "stub.one",
			Title: "one",
			Ask: func(context.Context) (Finding, error) {
				return Finding{}, errors.New("the tool is not installed")
			},
			Advice: "a",
		}},
	}

	s := Start(w, nil, 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.Step == nil || got.Step.Status != StatusUnknown {
		t.Fatalf("Step = %+v, want it recorded as unanswered", got.Step)
	}
	if got.Step.Finding.OK {
		t.Error("an unanswered question was recorded as OK")
	}
	if got.Step.Err == "" {
		t.Error("the record does not say why the question could not be answered")
	}
}

func TestAPanickingStepDoesNotEndTheSession(t *testing.T) {
	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{
			ID:     "stub.one",
			Title:  "one",
			Ask:    func(context.Context) (Finding, error) { panic("a collector went wrong") },
			Advice: "a",
		}},
	}

	s := Start(w, nil, 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.Step == nil || got.Step.Status != StatusUnknown {
		t.Fatalf("Step = %+v, want a panicking step recorded as unanswered", got.Step)
	}
}

func TestAStepWithNoFixOffersAdviceInstead(t *testing.T) {
	ask, _ := answers(broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{ID: "stub.one", Title: "one", Ask: ask, Advice: "advice.key"}},
	}

	s := Start(w, newApplier(t), 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.Offer != nil {
		t.Error("a fix was offered by a step that names none")
	}
	if got.Advice != "advice.key" {
		t.Errorf("Advice = %q, want the step's advice", got.Advice)
	}

	// Confirming what was never offered is refused rather than guessed at.
	if _, err := s.Confirm(context.Background(), remediate.Confirmation{}); !errors.Is(err, ErrNothingOffered) {
		t.Errorf("Confirm err = %v, want ErrNothingOffered", err)
	}
}

func TestAStepNamingAFixThatDoesNotRunHereSaysSo(t *testing.T) {
	ask, _ := answers(broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{{ID: "stub.one", Title: "one", Ask: ask, FixID: "never.compiled-in", Advice: "a"}},
	}

	s := Start(w, newApplier(t, "stub.fix"), 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.Offer != nil {
		t.Error("a fix that is not in the registry was offered")
	}
	if got.Step == nil || got.Step.Err == "" {
		t.Errorf("Step = %+v, want it to say the named fix is not available", got.Step)
	}
}

func TestStopEndsTheSessionWhereItIs(t *testing.T) {
	ask, _ := answers(broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "c", Platforms: platform.All(),
		Steps: []Step{
			{ID: "stub.one", Title: "one", Ask: ask, FixID: "stub.fix", Advice: "a"},
			{ID: "stub.two", Title: "two", Ask: ask, Advice: "a"},
		},
	}

	s := Start(w, newApplier(t, "stub.fix"), 0)
	if _, err := s.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}

	got := s.Stop()
	if got.Outcome != OutcomeStopped {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeStopped)
	}

	// Nothing further happens on a stopped session.
	for name, move := range map[string]func() (Progress, error){
		"Next":    func() (Progress, error) { return s.Next(context.Background()) },
		"Skip":    func() (Progress, error) { return s.Skip(context.Background()) },
		"Confirm": func() (Progress, error) { return s.Confirm(context.Background(), remediate.Confirmation{}) },
	} {
		if _, err := move(); !errors.Is(err, ErrFinished) {
			t.Errorf("%s after Stop: err = %v, want ErrFinished", name, err)
		}
	}
}

func TestEscalateHandsOverWhatWasAskedAndWhatCameOfIt(t *testing.T) {
	askA, _ := answers(clean())
	askB, _ := answers(broken())

	w := &Wizard{
		ID: "wizard.stub", Title: "t", Complaint: "complaint.key", Platforms: platform.All(),
		Steps: []Step{
			{ID: "stub.one", Title: "one", Ask: askA, Advice: "a"},
			{ID: "stub.two", Title: "two", Ask: askB, Advice: "a"},
		},
	}

	s := Start(w, nil, 0)
	if _, err := s.Next(context.Background()); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if _, err := s.Skip(context.Background()); err != nil {
		t.Fatalf("Skip: %v", err)
	}

	got := s.Escalate()
	if got.WizardID != "wizard.stub" || got.Complaint != "complaint.key" {
		t.Errorf("Escalation = %+v, want it to name the wizard and the complaint", got)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("Escalation lists %d steps, want every question asked", len(got.Steps))
	}
	if got.Outcome != OutcomeUnresolved {
		t.Errorf("Outcome = %q, want %q", got.Outcome, OutcomeUnresolved)
	}
}

func TestRunsOn(t *testing.T) {
	w := &Wizard{Platforms: []platform.OS{platform.Windows}}
	if !w.RunsOn(platform.Windows) {
		t.Error("RunsOn(Windows) = false for a Windows wizard")
	}
	if w.RunsOn(platform.Linux) {
		t.Error("RunsOn(Linux) = true for a Windows wizard")
	}
}

func TestOutcomeKey(t *testing.T) {
	for outcome, want := range map[Outcome]string{
		OutcomeFixed:      KeyOutcomeFixed,
		OutcomeUnresolved: KeyOutcomeUnresolved,
		OutcomeUnverified: KeyOutcomeUnverified,
		OutcomeNoFault:    KeyOutcomeNoFault,
		OutcomeStopped:    KeyOutcomeStopped,
		OutcomeRunning:    "",
	} {
		if got := OutcomeKey(outcome); got != want {
			t.Errorf("OutcomeKey(%q) = %q, want %q", outcome, got, want)
		}
	}
}
