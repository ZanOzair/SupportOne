package remediate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/restore"
)

// stubFix is a fix that records what was asked of it and nothing else. No test
// in this package touches a real machine.
type stubFix struct {
	id            string
	changes       []string
	requiresAdmin bool
	reversible    bool
	preflightErr  error
	applyErr      error
	rollbackErr   error

	applied    int
	rolledBack int
}

func (f *stubFix) ID() string { return f.id }

func (f *stubFix) Describe() fixes.Explanation {
	changes := f.changes
	if changes == nil {
		changes = []string{"fix.stub.change.one", "fix.stub.change.two"}
	}
	return fixes.Explanation{Summary: "fix.stub.summary", Changes: changes, Undo: "fix.stub.undo"}
}

func (f *stubFix) Platforms() []platform.OS { return platform.All() }
func (f *stubFix) RequiresAdmin() bool      { return f.requiresAdmin }
func (f *stubFix) Reversible() bool         { return f.reversible }

func (f *stubFix) Preflight(context.Context) error { return f.preflightErr }

func (f *stubFix) Apply(context.Context) (fixes.Outcome, error) {
	f.applied++
	if f.applyErr != nil {
		return fixes.Outcome{Applied: false, Detail: "fix.stub.failed"}, f.applyErr
	}
	return fixes.Outcome{Applied: true, Detail: "fix.stub.done"}, nil
}

func (f *stubFix) Rollback(context.Context) error {
	if f.rollbackErr != nil {
		return f.rollbackErr
	}
	f.rolledBack++
	return nil
}

// stubMaker stands in for the platform's restore mechanism.
type stubMaker struct {
	availability restore.Availability
	err          error
	created      int
	labels       []string
}

func (m *stubMaker) Check(context.Context) restore.Availability { return m.availability }

func (m *stubMaker) Create(_ context.Context, label string) (restore.Point, error) {
	if m.err != nil {
		return restore.Point{}, m.err
	}
	m.created++
	m.labels = append(m.labels, label)
	return restore.Point{Kind: "stub", Reference: "42", Label: label, Created: time.Unix(0, 0).UTC()}, nil
}

var available = restore.Availability{Available: true, Kind: "stub"}

func newApplier(t *testing.T, fix fixes.Fix, maker restore.Maker) (*Applier, string) {
	t.Helper()

	logPath := filepath.Join(t.TempDir(), "audit.log")
	log, err := consent.Open(logPath)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	registry := fixes.NewRegistry()
	if fix != nil {
		if err := registry.Register(fix); err != nil {
			t.Fatalf("register fix: %v", err)
		}
	}

	a := New(registry, log, maker, platform.Current())
	a.Elevated = func() (bool, error) { return true, nil }
	return a, logPath
}

// confirm builds the confirmation a caller that actually displayed the plan
// would be able to produce.
func confirm(p Plan) Confirmation {
	return Confirmation{Token: p.Token, Acknowledged: append([]string(nil), p.Explanation.Changes...)}
}

func TestPlanDescribesTheFixWithoutApplyingIt(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if fix.applied != 0 {
		t.Error("Plan applied the fix; planning must change nothing")
	}
	if plan.FixID != "stub.fix" {
		t.Errorf("FixID = %q, want %q", plan.FixID, "stub.fix")
	}
	if len(plan.Explanation.Changes) == 0 {
		t.Error("plan carries no list of changes for the user to confirm against")
	}
	if plan.Token == "" {
		t.Error("plan carries no token, so nothing proves it was shown")
	}
	if !plan.Restore.Available {
		t.Error("Restore.Available = false, want the maker's answer")
	}
	if !plan.Applicable() {
		t.Error("Applicable = false for a fix with nothing blocking it")
	}
}

func TestPlanRefusesAnIDThatWasNeverCompiledIn(t *testing.T) {
	a, _ := newApplier(t, &stubFix{id: "stub.fix", reversible: true}, &stubMaker{availability: available})

	// This is the whitelist: an ID from anywhere — a URL, a model response —
	// resolves to nothing unless it was built into this binary.
	if _, err := a.Plan(context.Background(), "rm.everything"); err == nil {
		t.Fatal("Plan accepted an ID that is not in the registry")
	}
}

func TestPlanTokensAreNotGuessable(t *testing.T) {
	a, _ := newApplier(t, &stubFix{id: "stub.fix", reversible: true}, &stubMaker{availability: available})

	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		plan, err := a.Plan(context.Background(), "stub.fix")
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if seen[plan.Token] {
			t.Fatalf("plan token %q was issued twice", plan.Token)
		}
		seen[plan.Token] = true
	}
}

func TestApplyRefusesWithoutAPlan(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	_, err := a.Apply(context.Background(), Confirmation{Token: "made-up", Acknowledged: []string{"fix.stub.change.one"}})
	if !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("Apply with an invented token: err = %v, want ErrNotConfirmed", err)
	}
	if fix.applied != 0 {
		t.Error("the fix ran without a plan")
	}
}

func TestApplyRefusesAnAcknowledgementThatDoesNotMatch(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	cases := map[string][]string{
		"nothing acknowledged":  nil,
		"empty list":            {},
		"only part of the list": {"fix.stub.change.one"},
		"a change never listed": {"fix.stub.change.one", "fix.stub.change.three"},
		"the wrong order":       {"fix.stub.change.two", "fix.stub.change.one"},
		"an extra change":       {"fix.stub.change.one", "fix.stub.change.two", "fix.stub.change.three"},
	}
	for name, acknowledged := range cases {
		t.Run(name, func(t *testing.T) {
			// A fresh plan each time: a rejected confirmation still spends its
			// token, which is the next test.
			plan, err = a.Plan(context.Background(), "stub.fix")
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			_, err := a.Apply(context.Background(), Confirmation{Token: plan.Token, Acknowledged: acknowledged})
			if !errors.Is(err, ErrNotConfirmed) {
				t.Fatalf("err = %v, want ErrNotConfirmed", err)
			}
			if fix.applied != 0 {
				t.Fatal("the fix ran against an acknowledgement that did not match what was shown")
			}
		})
	}
}

func TestApplyAcceptsAMatchingConfirmation(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	maker := &stubMaker{availability: available}
	a, _ := newApplier(t, fix, maker)

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	result, err := a.Apply(context.Background(), confirm(plan))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if fix.applied != 1 {
		t.Errorf("fix applied %d times, want 1", fix.applied)
	}
	if !result.Outcome.Applied {
		t.Error("Outcome.Applied = false after a successful apply")
	}
	if result.Outcome.FixID != "stub.fix" {
		t.Errorf("Outcome.FixID = %q, want %q", result.Outcome.FixID, "stub.fix")
	}
	if result.RestorePoint == nil {
		t.Fatal("no restore point was recorded, though one was available")
	}
	if maker.created != 1 {
		t.Errorf("restore points created = %d, want 1", maker.created)
	}
	if !strings.Contains(maker.labels[0], "stub.fix") {
		t.Errorf("restore point label %q does not name the fix it was made for", maker.labels[0])
	}
	if !result.Reversible {
		t.Error("Reversible = false for a fix that declares it is")
	}
}

func TestATokenIsGoodForOneDecision(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := a.Apply(context.Background(), confirm(plan)); err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	// Replaying the confirmation must not run the fix a second time: one
	// agreement covers one change.
	if _, err := a.Apply(context.Background(), confirm(plan)); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("replayed Apply: err = %v, want ErrNotConfirmed", err)
	}
	if fix.applied != 1 {
		t.Errorf("fix applied %d times, want 1", fix.applied)
	}
}

func TestARejectedConfirmationSpendsItsToken(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if _, err := a.Apply(context.Background(), Confirmation{Token: plan.Token}); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("err = %v, want ErrNotConfirmed", err)
	}
	// The second attempt must fail too: a token cannot be retried until an
	// acknowledgement happens to fit.
	if _, err := a.Apply(context.Background(), confirm(plan)); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("retry after a rejected confirmation: err = %v, want ErrNotConfirmed", err)
	}
	if fix.applied != 0 {
		t.Error("the fix ran on a retried token")
	}
}

func TestApplyRefusesWhenPreflightBlockedTheFix(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true, preflightErr: errors.New("the service is not running")}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Blocked == "" {
		t.Fatal("a refused preflight was not carried into the plan")
	}
	if plan.Applicable() {
		t.Error("Applicable = true for a blocked plan")
	}

	if _, err := a.Apply(context.Background(), confirm(plan)); err == nil {
		t.Fatal("Apply ran a fix its own preflight refused")
	}
	if fix.applied != 0 {
		t.Error("the fix ran despite a blocked preflight")
	}
}

func TestApplyRefusesWhenItDoesNotHoldTheRightsTheFixNeeds(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true, requiresAdmin: true}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})
	a.Elevated = func() (bool, error) { return false, nil }

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Elevated {
		t.Error("Elevated = true when the agent holds no administrator rights")
	}
	if plan.Applicable() {
		t.Error("Applicable = true for a fix whose rights the agent does not hold")
	}

	if _, err := a.Apply(context.Background(), confirm(plan)); !errors.Is(err, ErrNeedsAdmin) {
		t.Fatalf("err = %v, want ErrNeedsAdmin", err)
	}
	if fix.applied != 0 {
		t.Error("the fix ran without the rights it declared it needs")
	}
}

func TestAnUnanswerableElevationQuestionCountsAsNo(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true, requiresAdmin: true}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})
	a.Elevated = func() (bool, error) { return true, errors.New("cannot tell") }

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Elevated {
		t.Error("Elevated = true though the question could not be answered")
	}
}

func TestApplyRefusesWithoutARestorePointUnlessThatIsAccepted(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	maker := &stubMaker{availability: restore.Availability{Kind: "stub", Reason: restore.KeyUnavailableOnPlatform}}
	a, _ := newApplier(t, fix, maker)

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Restore.Reason == "" {
		t.Error("the plan does not say why no restore point is available")
	}

	if _, err := a.Apply(context.Background(), confirm(plan)); !errors.Is(err, ErrNoRestorePoint) {
		t.Fatalf("err = %v, want ErrNoRestorePoint", err)
	}
	if fix.applied != 0 {
		t.Error("the fix ran with no restore point and no acceptance")
	}

	// Said out loud, it proceeds — and no restore point is invented.
	plan, err = a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	c := confirm(plan)
	c.AcceptWithoutRestorePoint = true

	result, err := a.Apply(context.Background(), c)
	if err != nil {
		t.Fatalf("Apply after accepting: %v", err)
	}
	if result.RestorePoint != nil {
		t.Error("a restore point was reported though none could be made")
	}
	if maker.created != 0 {
		t.Error("a restore point was created though the mechanism said it could not be")
	}
	if fix.applied != 1 {
		t.Errorf("fix applied %d times, want 1", fix.applied)
	}
}

func TestApplyStopsWhenThePromisedRestorePointCannotBeMade(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	maker := &stubMaker{availability: available, err: errors.New("the volume is full")}
	a, _ := newApplier(t, fix, maker)

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// The user agreed to a change that came with a restore point. Going ahead
	// without one would not be the change they agreed to.
	if _, err := a.Apply(context.Background(), confirm(plan)); err == nil {
		t.Fatal("Apply proceeded after the restore point it promised failed")
	}
	if fix.applied != 0 {
		t.Error("the fix ran after its restore point failed")
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	maker := &stubMaker{availability: available}
	a, _ := newApplier(t, fix, maker)
	a.DryRun = true

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !plan.DryRun {
		t.Error("the plan does not say this is a dry run")
	}

	result, err := a.Apply(context.Background(), confirm(plan))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if fix.applied != 0 {
		t.Error("a dry run ran the fix")
	}
	if maker.created != 0 {
		t.Error("a dry run created a restore point for a change it never made")
	}
	if !result.Outcome.DryRun || result.Outcome.Applied {
		t.Errorf("Outcome = %+v, want a dry run that applied nothing", result.Outcome)
	}
	if len(a.Applied()) != 0 {
		t.Error("a dry run was recorded as something to roll back")
	}
}

func TestRollbackUndoesWhatThisSessionApplied(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	// Nothing has been applied, so there is nothing to undo.
	if err := a.Rollback(context.Background(), "stub.fix"); !errors.Is(err, ErrNotApplied) {
		t.Fatalf("Rollback before Apply: err = %v, want ErrNotApplied", err)
	}

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := a.Apply(context.Background(), confirm(plan)); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got := a.Applied(); len(got) != 1 || got[0] != "stub.fix" {
		t.Fatalf("Applied = %v, want [stub.fix]", got)
	}

	if err := a.Rollback(context.Background(), "stub.fix"); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if fix.rolledBack != 1 {
		t.Errorf("fix rolled back %d times, want 1", fix.rolledBack)
	}
	if len(a.Applied()) != 0 {
		t.Errorf("Applied = %v after a rollback, want empty", a.Applied())
	}
}

func TestAFailedRollbackStaysOnTheListOfThingsToUndo(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true, rollbackErr: errors.New("the file is in use")}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := a.Apply(context.Background(), confirm(plan)); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if err := a.Rollback(context.Background(), "stub.fix"); err == nil {
		t.Fatal("Rollback reported success though the fix could not undo itself")
	}
	// Forgetting it would leave a change on the machine that nothing records
	// as still applied.
	if got := a.Applied(); len(got) != 1 {
		t.Errorf("Applied = %v after a failed rollback, want the fix still listed", got)
	}
}

func TestAFailedApplyIsNotRecordedAsApplied(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true, applyErr: errors.New("the service refused to stop")}
	a, _ := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := a.Apply(context.Background(), confirm(plan)); err == nil {
		t.Fatal("Apply reported success though the fix failed")
	}
	if len(a.Applied()) != 0 {
		t.Errorf("Applied = %v after a failed apply, want empty", a.Applied())
	}
}

func TestEveryDecisionReachesTheAuditLog(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	a, logPath := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := a.Apply(context.Background(), confirm(plan)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := a.Rollback(context.Background(), "stub.fix"); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	log := string(raw)

	for _, want := range []consent.EventType{
		consent.EventFixPreflight,
		consent.EventConsentAsked,
		consent.EventConsentGiven,
		consent.EventFixApplied,
		consent.EventFixRolledBack,
	} {
		if !strings.Contains(log, string(want)) {
			t.Errorf("audit log has no %s entry:\n%s", want, log)
		}
	}
}

func TestARefusalReachesTheAuditLog(t *testing.T) {
	fix := &stubFix{id: "stub.fix", reversible: true}
	a, logPath := newApplier(t, fix, &stubMaker{availability: available})

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := a.Apply(context.Background(), Confirmation{Token: plan.Token, Acknowledged: []string{"something else"}}); err == nil {
		t.Fatal("Apply accepted an acknowledgement that did not match")
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(raw), string(consent.EventConsentDenied)) {
		t.Errorf("a refused change left no CONSENT_DENIED entry:\n%s", raw)
	}
}

func TestAnAuditLogIsOptional(t *testing.T) {
	// A missing log must not stop a user from repairing their machine.
	registry := fixes.NewRegistry()
	fix := &stubFix{id: "stub.fix", reversible: true}
	if err := registry.Register(fix); err != nil {
		t.Fatalf("register: %v", err)
	}

	a := New(registry, nil, &stubMaker{availability: available}, platform.Current())
	a.Elevated = func() (bool, error) { return true, nil }

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := a.Apply(context.Background(), confirm(plan)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func TestPlanWithoutARestoreMechanismSaysSo(t *testing.T) {
	registry := fixes.NewRegistry()
	if err := registry.Register(&stubFix{id: "stub.fix", reversible: true}); err != nil {
		t.Fatalf("register: %v", err)
	}

	a := New(registry, nil, nil, platform.Current())
	a.Elevated = func() (bool, error) { return true, nil }

	plan, err := a.Plan(context.Background(), "stub.fix")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Restore.Available {
		t.Error("Restore.Available = true with no restore mechanism at all")
	}
}

func TestPlanRefusesAFixThatDoesNotRunHere(t *testing.T) {
	registry := fixes.NewRegistry()
	if err := registry.Register(&otherPlatformFix{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	a := New(registry, nil, &stubMaker{availability: available}, platform.Linux)
	a.Elevated = func() (bool, error) { return true, nil }

	if _, err := a.Plan(context.Background(), "windows.only"); err == nil {
		t.Fatal("Plan offered a fix that does not run on this platform")
	}
}

type otherPlatformFix struct{ stubFix }

func (f *otherPlatformFix) ID() string               { return "windows.only" }
func (f *otherPlatformFix) Platforms() []platform.OS { return []platform.OS{platform.Windows} }

func TestAcknowledgesRequiresAnExactRepeat(t *testing.T) {
	described := []string{"one", "two"}

	cases := []struct {
		name         string
		acknowledged []string
		want         bool
	}{
		{"the same list", []string{"one", "two"}, true},
		{"nothing", nil, false},
		{"reordered", []string{"two", "one"}, false},
		{"shorter", []string{"one"}, false},
		{"longer", []string{"one", "two", "three"}, false},
		{"different content", []string{"one", "three"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := acknowledges(described, tc.acknowledged); got != tc.want {
				t.Errorf("acknowledges = %v, want %v", got, tc.want)
			}
		})
	}

	// A fix that described no changes cannot be confirmed at all: there was
	// nothing for the user to agree to.
	if acknowledges(nil, nil) {
		t.Error("acknowledges accepted an empty description")
	}
}
