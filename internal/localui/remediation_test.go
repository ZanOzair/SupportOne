package localui

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
	"github.com/ZanOzair/SupportOne/internal/remediate"
	"github.com/ZanOzair/SupportOne/internal/restore"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

// stubFix records whether it ran. Nothing in this package's tests touches a
// real machine.
type stubFix struct {
	id      string
	applied int
	broken  bool
}

func (f *stubFix) ID() string { return f.id }
func (f *stubFix) Describe() fixes.Explanation {
	return fixes.Explanation{
		Summary: "fix.stub.summary",
		Changes: []string{"fix.stub.change.one", "fix.stub.change.two"},
		Undo:    "fix.stub.undo",
	}
}
func (f *stubFix) Platforms() []platform.OS        { return platform.All() }
func (f *stubFix) RequiresAdmin() bool             { return false }
func (f *stubFix) Reversible() bool                { return true }
func (f *stubFix) Preflight(context.Context) error { return nil }
func (f *stubFix) Rollback(context.Context) error  { return nil }
func (f *stubFix) Apply(context.Context) (fixes.Outcome, error) {
	f.applied++
	return fixes.Outcome{Applied: true}, nil
}

type stubMaker struct{}

func (stubMaker) Check(context.Context) restore.Availability {
	return restore.Availability{Available: true, Kind: "stub"}
}
func (stubMaker) Create(context.Context, string) (restore.Point, error) {
	return restore.Point{Kind: "stub"}, nil
}

// repairServer starts a server that can change things, with one fix and one
// wizard behind it.
func repairServer(t *testing.T) (*Server, *stubFix) {
	t.Helper()

	fix := &stubFix{id: "stub.fix"}
	registry := fixes.NewRegistry()
	if err := registry.Register(fix); err != nil {
		t.Fatalf("register fix: %v", err)
	}

	log, err := consent.Open(t.TempDir() + "/audit.log")
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	applier := remediate.New(registry, log, stubMaker{}, platform.Linux)
	applier.Elevated = func() (bool, error) { return true, nil }

	wizards := wizard.NewRegistry()
	if err := wizards.Register(&wizard.Wizard{
		ID: "wizard.stub", Title: "wizard.stub.title", Complaint: "wizard.stub.complaint",
		Platforms: platform.All(),
		Steps: []wizard.Step{
			{
				ID:    "stub.broken",
				Title: "wizard.stub.step",
				Ask: func(context.Context) (wizard.Finding, error) {
					if fix.applied > 0 && !fix.broken {
						return wizard.Finding{OK: true, Summary: "wizard.stub.ok"}, nil
					}
					return wizard.Finding{Summary: "wizard.stub.broken"}, nil
				},
				FixID:  "stub.fix",
				Advice: "wizard.stub.advice",
			},
		},
	}); err != nil {
		t.Fatalf("register wizard: %v", err)
	}

	s, err := New(Config{
		Assets:      testAssets(),
		Snapshot:    testSnapshot,
		Audit:       log,
		Version:     "test",
		Host:        platform.Host{OS: platform.Linux, Arch: "amd64"},
		Identity:    redact.Identity{Hostname: "alex-laptop"},
		IdleTimeout: time.Minute,
		Fixes:       registry,
		Applier:     applier,
		Wizards:     wizards,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := s.Serve(ctx); err != nil {
			t.Errorf("Serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		s.Close()
	})
	return s, fix
}

// call makes one API request and decodes the JSON it returns. Building the
// request and closing its body in one place keeps both obvious.
func call(t *testing.T, s *Server, method, path string, into any, body ...string) {
	t.Helper()

	res := do(t, s, method, path, s.token, nil, body...)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s %s = %d, want 200", method, path, res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(into); err != nil {
		t.Fatalf("decode %s %s: %v", method, path, err)
	}
}

func TestTheRepairRoutesNeedTheSessionToken(t *testing.T) {
	s, fix := repairServer(t)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/fixes"},
		{http.MethodPost, "/api/fixes/plan"},
		{http.MethodPost, "/api/fixes/apply"},
		{http.MethodPost, "/api/fixes/rollback"},
		{http.MethodGet, "/api/wizards"},
		{http.MethodPost, "/api/wizards/start"},
		{http.MethodPost, "/api/wizards/next"},
	} {
		res := do(t, s, route.method, route.path, "", nil, `{}`)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", route.method, route.path, res.StatusCode)
		}
	}
	if fix.applied != 0 {
		t.Error("a fix ran for an unauthenticated request")
	}
}

func TestFixesAreListedWithWhatTheyWouldChange(t *testing.T) {
	s, _ := repairServer(t)

	var listed []fixSummary
	call(t, s, http.MethodGet, "/api/fixes", &listed)

	if len(listed) != 1 || listed[0].ID != "stub.fix" {
		t.Fatalf("listed %+v, want the one registered fix", listed)
	}
	if len(listed[0].Explanation.Changes) == 0 {
		t.Error("a fix was listed without the changes it makes")
	}
	if !listed[0].Reversible {
		t.Error("Reversible was not carried into the listing")
	}
}

func TestPlanningChangesNothing(t *testing.T) {
	s, fix := repairServer(t)

	var plan remediate.Plan
	call(t, s, http.MethodPost, "/api/fixes/plan", &plan, `{"fix_id":"stub.fix"}`)

	if fix.applied != 0 {
		t.Error("planning applied the fix")
	}
	if plan.Token == "" {
		t.Error("the plan carries no token")
	}
	if len(plan.Explanation.Changes) == 0 {
		t.Error("the plan does not list what would change")
	}
}

func TestPlanningAnIDThatIsNotCompiledInIsRefused(t *testing.T) {
	s, _ := repairServer(t)

	res := do(t, s, http.MethodPost, "/api/fixes/plan", s.token, nil, `{"fix_id":"rm.everything"}`)
	res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an ID that is not in the registry", res.StatusCode)
	}
}

func TestApplyingNeedsTheConfirmationTheGateAsksFor(t *testing.T) {
	s, fix := repairServer(t)

	var plan remediate.Plan
	call(t, s, http.MethodPost, "/api/fixes/plan", &plan, `{"fix_id":"stub.fix"}`)

	// An acknowledgement that does not repeat what the plan showed.
	body := `{"token":"` + plan.Token + `","acknowledged":["something else"]}`
	res := do(t, s, http.MethodPost, "/api/fixes/apply", s.token, nil, body)
	res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a confirmation that does not match", res.StatusCode)
	}
	if fix.applied != 0 {
		t.Error("the fix ran on a confirmation that did not match what was shown")
	}
}

func TestApplyingAndRollingBackAFixOverTheAPI(t *testing.T) {
	s, fix := repairServer(t)

	var plan remediate.Plan
	call(t, s, http.MethodPost, "/api/fixes/plan", &plan, `{"fix_id":"stub.fix"}`)

	confirmation, err := json.Marshal(remediate.Confirmation{
		Token:        plan.Token,
		Acknowledged: plan.Explanation.Changes,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var result remediate.Result
	call(t, s, http.MethodPost, "/api/fixes/apply", &result, string(confirmation))

	if fix.applied != 1 {
		t.Errorf("the fix ran %d times, want 1", fix.applied)
	}
	if !result.Outcome.Applied {
		t.Error("the result does not report the change as applied")
	}

	res := do(t, s, http.MethodPost, "/api/fixes/rollback", s.token, nil, `{"fix_id":"stub.fix"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("rollback status = %d, want 200", res.StatusCode)
	}

	// Rolling back twice is not an error to hide: the second one has nothing
	// to undo and says so.
	res = do(t, s, http.MethodPost, "/api/fixes/rollback", s.token, nil, `{"fix_id":"stub.fix"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("second rollback status = %d, want 404", res.StatusCode)
	}
}

func TestAWizardRunsToAFixAndBack(t *testing.T) {
	s, fix := repairServer(t)

	var listed []wizardSummary
	call(t, s, http.MethodGet, "/api/wizards", &listed)
	if len(listed) != 1 || listed[0].ID != "wizard.stub" {
		t.Fatalf("listed %+v, want the one registered wizard", listed)
	}

	var started wizardProgress
	call(t, s, http.MethodPost, "/api/wizards/start", &started, `{"wizard_id":"wizard.stub"}`)

	if started.SessionID == "" {
		t.Fatal("the session has no ID to continue it with")
	}
	if started.Progress.Offer == nil {
		t.Fatal("the step that found something offered nothing")
	}

	move, err := json.Marshal(wizardMove{
		SessionID: started.SessionID,
		Confirmation: &remediate.Confirmation{
			Token:        started.Progress.Offer.Token,
			Acknowledged: started.Progress.Offer.Explanation.Changes,
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var after wizardProgress
	call(t, s, http.MethodPost, "/api/wizards/confirm", &after, string(move))

	if fix.applied != 1 {
		t.Errorf("the fix ran %d times, want 1", fix.applied)
	}
	// The step was asked again, came back clean, and only then is it fixed.
	if after.Progress.Outcome != wizard.OutcomeFixed {
		t.Errorf("Outcome = %q, want %q", after.Progress.Outcome, wizard.OutcomeFixed)
	}

	// The handover survives the session ending.
	var escalation wizard.Escalation
	call(t, s, http.MethodGet, "/api/wizards/escalation?session="+started.SessionID, &escalation)
	if escalation.WizardID != "wizard.stub" || len(escalation.Steps) != 1 {
		t.Errorf("escalation = %+v, want the wizard's history", escalation)
	}
}

func TestSkippingAWizardStepChangesNothing(t *testing.T) {
	s, fix := repairServer(t)

	var started wizardProgress
	call(t, s, http.MethodPost, "/api/wizards/start", &started, `{"wizard_id":"wizard.stub"}`)

	var after wizardProgress
	body := `{"session_id":"` + started.SessionID + `"}`
	call(t, s, http.MethodPost, "/api/wizards/skip", &after, body)

	if fix.applied != 0 {
		t.Error("skipping a step applied its fix")
	}
	if after.Progress.Outcome != wizard.OutcomeUnresolved {
		t.Errorf("Outcome = %q, want %q", after.Progress.Outcome, wizard.OutcomeUnresolved)
	}
}

func TestAConfirmWithNoConfirmationIsRefused(t *testing.T) {
	s, fix := repairServer(t)

	var started wizardProgress
	call(t, s, http.MethodPost, "/api/wizards/start", &started, `{"wizard_id":"wizard.stub"}`)

	body := `{"session_id":"` + started.SessionID + `"}`
	res := do(t, s, http.MethodPost, "/api/wizards/confirm", s.token, nil, body)
	res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
	if fix.applied != 0 {
		t.Error("a fix ran without a confirmation")
	}
}

func TestAnUnknownWizardOrSessionIsRefused(t *testing.T) {
	s, _ := repairServer(t)

	res := do(t, s, http.MethodPost, "/api/wizards/start", s.token, nil, `{"wizard_id":"wizard.nope"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown wizard = %d, want 404", res.StatusCode)
	}

	res = do(t, s, http.MethodPost, "/api/wizards/next", s.token, nil, `{"session_id":"nope"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown session = %d, want 404", res.StatusCode)
	}

	res = do(t, s, http.MethodGet, "/api/wizards/escalation?session=nope", s.token, nil)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown escalation = %d, want 404", res.StatusCode)
	}
}

func TestOnlySoManyWizardSessionsAtOnce(t *testing.T) {
	s, _ := repairServer(t)

	for i := 0; i < maxSessions; i++ {
		res := do(t, s, http.MethodPost, "/api/wizards/start", s.token, nil, `{"wizard_id":"wizard.stub"}`)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("session %d = %d, want 200", i, res.StatusCode)
		}
	}

	res := do(t, s, http.MethodPost, "/api/wizards/start", s.token, nil, `{"wizard_id":"wizard.stub"}`)
	res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429 once the limit is reached", res.StatusCode)
	}
}

func TestAReadOnlyBuildSaysSoRatherThanFailingObscurely(t *testing.T) {
	// A build with no fixes compiled in still serves the interface; the
	// repair routes report they have nothing rather than erroring.
	s := startServer(t)

	var listed []fixSummary
	call(t, s, http.MethodGet, "/api/fixes", &listed)
	if len(listed) != 0 {
		t.Errorf("listed %+v, want nothing", listed)
	}

	for _, path := range []string{"/api/fixes/plan", "/api/fixes/apply", "/api/fixes/rollback", "/api/wizards/start"} {
		res := do(t, s, http.MethodPost, path, s.token, nil, `{}`)
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s = %d, want 404 on a read-only build", path, res.StatusCode)
		}
	}
}

func TestAWizardSessionIsRecordedWhenItEnds(t *testing.T) {
	s, _ := repairServer(t)

	var started wizardProgress
	call(t, s, http.MethodPost, "/api/wizards/start", &started, `{"wizard_id":"wizard.stub"}`)

	body := `{"session_id":"` + started.SessionID + `"}`
	res := do(t, s, http.MethodPost, "/api/wizards/stop", s.token, nil, body)
	res.Body.Close()

	raw, err := readFile(s.cfg.Audit.Path())
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !contains(raw, string(consent.EventWizardEnded)) {
		t.Errorf("the audit log has no record of the session ending:\n%s", raw)
	}
}

func readFile(path string) (string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- a path this test just created
	return string(raw), err
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
