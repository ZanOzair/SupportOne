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
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
	"github.com/ZanOzair/SupportOne/internal/remote"
)

// remoteServer starts a server with a real consent wrapper over this machine.
//
// Nothing here fakes an installed remote-help program: these tests drive the
// routes with no tool named, so the whole exchange runs and no process is ever
// launched. What the wrapper does once a tool is found is internal/remote's to
// test, and it does.
func remoteServer(t *testing.T) *Server {
	t.Helper()

	log, err := consent.Open(t.TempDir() + "/audit.log")
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	s, err := New(Config{
		Assets:      testAssets(),
		Snapshot:    testSnapshot,
		Audit:       log,
		Version:     "test",
		Host:        platform.Host{OS: platform.Linux, Arch: "amd64"},
		Identity:    redact.Identity{Hostname: "alex-laptop"},
		IdleTimeout: time.Minute,
		Remote:      remote.New(log, platform.Linux),
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
	return s
}

// readAuditLog returns everything this server has recorded.
func readAuditLog(t *testing.T, s *Server) string {
	t.Helper()

	data, err := os.ReadFile(s.cfg.Audit.Path()) // #nosec G304 -- a path this test just created
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	return string(data)
}

func TestTheRemoteRoutesNeedTheSessionToken(t *testing.T) {
	s := remoteServer(t)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/remote"},
		{http.MethodPost, "/api/remote/plan"},
		{http.MethodPost, "/api/remote/start"},
		{http.MethodPost, "/api/remote/decline"},
		{http.MethodPost, "/api/remote/end"},
	} {
		res := do(t, s, route.method, route.path, "", nil, `{}`)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", route.method, route.path, res.StatusCode)
		}
	}
}

func TestTheRemoteStateCarriesTheConsequencesBeforeAnyoneIsNamed(t *testing.T) {
	s := remoteServer(t)

	var state remoteState
	call(t, s, http.MethodGet, "/api/remote", &state)

	if !state.Available {
		t.Fatal("the server has a wrapper but reports remote help as unavailable")
	}
	if len(state.Consequences) != len(remote.Consequences()) {
		t.Errorf("the state carries %d consequences, want all %d", len(state.Consequences), len(remote.Consequences()))
	}
	if state.Session != nil {
		t.Errorf("a session is reported before anyone agreed to one: %+v", state.Session)
	}

	// The list is whatever is actually on this machine, which on a build
	// runner is usually nothing. What matters is that each entry says which
	// it is and never claims a path it did not find.
	if len(state.Tools) == 0 {
		t.Error("no tools were listed at all, so the panel has nothing to show or rule out")
	}
	for _, tool := range state.Tools {
		if !tool.Installed && tool.Path != "" {
			t.Errorf("%s is not installed but reports path %q", tool.ID, tool.Path)
		}
	}
}

func TestABuildWithoutRemoteHelpSaysSoRatherThanFailing(t *testing.T) {
	s, _ := repairServer(t) // no Remote in its config

	var state remoteState
	call(t, s, http.MethodGet, "/api/remote", &state)

	if state.Available {
		t.Error("a build with no wrapper reports remote help as available")
	}
	if len(state.Consequences) == 0 {
		t.Error("the consequences are not shown, so the panel cannot explain why the button is absent")
	}

	for _, path := range []string{"/api/remote/plan", "/api/remote/start", "/api/remote/decline", "/api/remote/end"} {
		res := do(t, s, http.MethodPost, path, s.token, nil, `{}`)
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", path, res.StatusCode)
		}
	}
}

func TestAPlanIsNotASessionAndStartsNothing(t *testing.T) {
	s := remoteServer(t)

	var plan remote.Plan
	call(t, s, http.MethodPost, "/api/remote/plan", &plan, `{"technician":"Aisyah","tool_id":""}`)

	if plan.Token == "" {
		t.Error("the plan carries no token")
	}
	if len(plan.Consequences) != len(remote.Consequences()) {
		t.Errorf("the plan shows %d consequences, want all %d", len(plan.Consequences), len(remote.Consequences()))
	}

	var state remoteState
	call(t, s, http.MethodGet, "/api/remote", &state)
	if state.Session != nil {
		t.Error("a plan created a session")
	}
}

func TestStartingWithoutRepeatingThePlanIsRefused(t *testing.T) {
	s := remoteServer(t)

	var plan remote.Plan
	call(t, s, http.MethodPost, "/api/remote/plan", &plan, `{"technician":"Aisyah","tool_id":""}`)

	body, err := json.Marshal(remote.Confirmation{
		Token: plan.Token,
		// One line short of what was shown.
		Acknowledged: plan.Consequences[:len(plan.Consequences)-1],
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	res := do(t, s, http.MethodPost, "/api/remote/start", s.token, nil, string(body))
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("start on a mangled acknowledgement = %d, want 403", res.StatusCode)
	}
}

func TestAConfirmedSessionIsRecordedAndEndsWhenTheUserSaysSo(t *testing.T) {
	s := remoteServer(t)

	var plan remote.Plan
	call(t, s, http.MethodPost, "/api/remote/plan", &plan, `{"technician":"Aisyah","tool_id":""}`)

	body, err := json.Marshal(remote.Confirmation{Token: plan.Token, Acknowledged: plan.Consequences})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var session remote.Session
	call(t, s, http.MethodPost, "/api/remote/start", &session, string(body))
	if session.Technician != "Aisyah" {
		t.Errorf("session.Technician = %q, want the name from the plan", session.Technician)
	}
	if session.Launched {
		t.Error("Launched is true, but no tool was named for SupportOne to start")
	}

	var state remoteState
	call(t, s, http.MethodGet, "/api/remote", &state)
	if state.Session == nil || state.Session.ID != session.ID {
		t.Errorf("state.Session = %+v, want the open session", state.Session)
	}

	var ended remote.Session
	call(t, s, http.MethodPost, "/api/remote/end", &ended, `{}`)
	if ended.Ended.IsZero() {
		t.Error("the ended session has no end time")
	}

	// Decoded into the same object the open session came back in: the state
	// must actively clear it, not merely stop mentioning it.
	call(t, s, http.MethodGet, "/api/remote", &state)
	if state.Session != nil {
		t.Errorf("a session is still reported as open: %+v", state.Session)
	}

}

func TestEndingNothingIsNotFound(t *testing.T) {
	s := remoteServer(t)

	res := do(t, s, http.MethodPost, "/api/remote/end", s.token, nil, `{}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("end with no session = %d, want 404", res.StatusCode)
	}
}

func TestNamingNobodyIsRefusedBeforeAnythingIsRecorded(t *testing.T) {
	s := remoteServer(t)

	res := do(t, s, http.MethodPost, "/api/remote/plan", s.token, nil, `{"technician":"   ","tool_id":""}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("a plan naming nobody = %d, want 400", res.StatusCode)
	}
}

func TestAToolThisBuildDoesNotKnowIsNotFound(t *testing.T) {
	s := remoteServer(t)

	res := do(t, s, http.MethodPost, "/api/remote/plan", s.token, nil, `{"technician":"Aisyah","tool_id":"something-else"}`)
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown tool = %d, want 404", res.StatusCode)
	}
}

func TestTheAuditLogRecordsTheWholeSession(t *testing.T) {
	s := remoteServer(t)

	var plan remote.Plan
	call(t, s, http.MethodPost, "/api/remote/plan", &plan, `{"technician":"Aisyah","tool_id":""}`)

	body, err := json.Marshal(remote.Confirmation{Token: plan.Token, Acknowledged: plan.Consequences})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var session remote.Session
	call(t, s, http.MethodPost, "/api/remote/start", &session, string(body))
	call(t, s, http.MethodPost, "/api/remote/end", &session, `{}`)

	log := readAuditLog(t, s)
	for _, want := range []string{
		string(consent.EventConsentAsked),
		string(consent.EventConsentGiven),
		string(consent.EventRemoteStarted),
		string(consent.EventRemoteEnded),
		"Aisyah",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the audit log does not record %q:\n%s", want, log)
		}
	}
}

func TestDecliningAPlanIsRecordedAndDiscardsIt(t *testing.T) {
	s := remoteServer(t)

	var plan remote.Plan
	call(t, s, http.MethodPost, "/api/remote/plan", &plan, `{"technician":"Aisyah","tool_id":""}`)

	var result map[string]string
	call(t, s, http.MethodPost, "/api/remote/decline", &result, `{}`)
	if result["status"] != "declined" {
		t.Errorf("decline returned %v", result)
	}

	if log := readAuditLog(t, s); !strings.Contains(log, string(consent.EventConsentDenied)) {
		t.Errorf("the refusal was not recorded:\n%s", log)
	}

	body, err := json.Marshal(remote.Confirmation{Token: plan.Token, Acknowledged: plan.Consequences})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := do(t, s, http.MethodPost, "/api/remote/start", s.token, nil, string(body))
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("start on a declined plan = %d, want 403", res.StatusCode)
	}
}
