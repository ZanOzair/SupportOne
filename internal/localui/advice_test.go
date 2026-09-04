package localui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/assist"
	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"

	_ "github.com/ZanOzair/SupportOne/internal/fixes/all"
	_ "github.com/ZanOzair/SupportOne/internal/wizard/all"
)

// findingSnapshot has something wrong in it, so the advice is worth asserting
// on.
func findingSnapshot(context.Context) checks.Snapshot {
	return checks.Snapshot{
		Schema:       checks.SnapshotSchema,
		AgentVersion: "test",
		GeneratedAt:  time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC),
		Host:         platform.Host{OS: platform.Linux, Arch: "amd64"},
		Results: []checks.Result{
			{CheckID: "os.info", Severity: checks.SeverityOK, Summary: "check.os.info.ok"},
			{
				CheckID:  "disk.volumes",
				Severity: checks.SeverityAttention,
				Summary:  "check.disk.volumes.low",
				Detail:   map[string]any{"hostname": "alex-laptop"},
			},
			{CheckID: "disk.smart", Severity: checks.SeverityUrgent, Summary: "check.disk.smart.failing"},
		},
	}
}

// adviceServer starts a server that can explain, and optionally can ask an
// endpoint the test controls. The assistant is built from the same audit log
// the server uses, which is how production wires it: one record of everything
// that happened in one run.
func adviceServer(t *testing.T, build func(*consent.Log) *assist.Assistant) (*Server, string) {
	t.Helper()

	logPath := t.TempDir() + "/audit.log"
	log, err := consent.Open(logPath)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	var assistant *assist.Assistant
	if build != nil {
		assistant = build(log)
	}

	s, err := New(Config{
		Assets:      testAssets(),
		Snapshot:    findingSnapshot,
		Audit:       log,
		Version:     "test",
		Host:        platform.Host{OS: platform.Linux, Arch: "amd64"},
		Identity:    redact.Identity{Hostname: "alex-laptop", Username: "alex", HomeDir: "/home/alex"},
		IdleTimeout: time.Minute,
		Explainer:   explain.New(fixes.Default, nil, platform.Linux),
		Assistant:   assistant,
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
	return s, logPath
}

func TestExplanationsAreServedWorstFirst(t *testing.T) {
	s, _ := adviceServer(t, nil)

	var advice []explain.Advice
	call(t, s, http.MethodGet, "/api/explain", &advice)

	if len(advice) != 3 {
		t.Fatalf("explained %d findings, want 3", len(advice))
	}
	if advice[0].CheckID != "disk.smart" {
		t.Errorf("first = %q, want the urgent finding", advice[0].CheckID)
	}
	// The one that is fine is still explained, and it is last.
	if advice[2].CheckID != "os.info" {
		t.Errorf("last = %q, want the OK finding", advice[2].CheckID)
	}
	if advice[0].Cause == "" || len(advice[0].Steps) == 0 {
		t.Errorf("the urgent finding came with no advice: %+v", advice[0])
	}
	// Advice offers only what this build carries.
	for _, a := range advice {
		for _, id := range a.Fixes {
			if _, ok := fixes.Default.Get(id); !ok {
				t.Errorf("%s offers %q, which is not compiled in", a.CheckID, id)
			}
		}
	}
}

func TestABuildWithNoExplainerStillServesTheRoute(t *testing.T) {
	s := startServer(t)

	var advice []explain.Advice
	call(t, s, http.MethodGet, "/api/explain", &advice)
	if len(advice) != 0 {
		t.Errorf("advice = %v, want none", advice)
	}
}

func TestTheSavedReportCarriesTheAdvice(t *testing.T) {
	s, _ := adviceServer(t, nil)

	res := do(t, s, http.MethodGet, "/api/report?format=html&lang=en", s.token, nil)
	defer res.Body.Close()

	body := make([]byte, 200_000)
	n, _ := res.Body.Read(body)
	rendered := string(body[:n])

	if !strings.Contains(rendered, "What this means:") {
		t.Error("the saved report carries no explanation")
	}
}

// TestTheAssistantIsOffUnlessItWasTurnedOn is the first thing the interface
// asks, and the answer decides whether it offers anything at all.
func TestTheAssistantIsOffUnlessItWasTurnedOn(t *testing.T) {
	s, _ := adviceServer(t, nil)

	var state assistState
	call(t, s, http.MethodGet, "/api/assist", &state)
	if state.Enabled {
		t.Error("Enabled = true with no assistant configured")
	}

	for _, path := range []string{"/api/assist/prepare", "/api/assist/ask", "/api/assist/discard"} {
		res := do(t, s, http.MethodPost, path, s.token, nil, `{}`)
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("%s = %d, want 404 when the assistant is off", path, res.StatusCode)
		}
	}
}

func TestAnAssistantThatIsConfiguredButOffStaysOff(t *testing.T) {
	s, _ := adviceServer(t, func(log *consent.Log) *assist.Assistant {
		return assist.New(assist.Config{Endpoint: "https://example.invalid/v1"}, fixes.Default, platform.Linux, log)
	})

	var state assistState
	call(t, s, http.MethodGet, "/api/assist", &state)
	if state.Enabled {
		t.Error("Enabled = true for an assistant that was never switched on")
	}
	if state.Endpoint != "" {
		t.Errorf("Endpoint = %q; a switched-off assistant reveals nothing", state.Endpoint)
	}
}

func TestPreparingShowsThePayloadAndSendsNothing(t *testing.T) {
	var reached int
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	t.Cleanup(endpoint.Close)

	s, logPath := adviceServer(t, func(log *consent.Log) *assist.Assistant {
		return assist.New(
			assist.Config{Enabled: true, Endpoint: endpoint.URL, Model: "stub"},
			fixes.Default, platform.Linux, log,
		)
	})

	var payload assist.Payload
	call(t, s, http.MethodPost, "/api/assist/prepare", &payload, `{"hostnames":true,"usernames":true,"serials":true,"addresses":true}`)

	if reached != 0 {
		t.Errorf("the endpoint was contacted %d times while preparing", reached)
	}
	if payload.Body == "" || payload.Token == "" {
		t.Fatalf("payload = %+v, want the bytes and a token", payload)
	}
	// The user chose full redaction, and what they were shown honours it.
	if strings.Contains(payload.Body, "alex-laptop") {
		t.Error("the payload still carries the hostname after full redaction")
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	// Preparing is not sending, and the log distinguishes them.
	if strings.Contains(string(raw), string(consent.EventDataSent)) {
		t.Errorf("preparing was recorded as a send:\n%s", raw)
	}
	if !strings.Contains(string(raw), string(consent.EventConsentAsked)) {
		t.Errorf("preparing left no record:\n%s", raw)
	}
}

func TestAskingSendsOnlyWhatWasConfirmed(t *testing.T) {
	var sent []byte
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 1<<20)
		n, _ := r.Body.Read(body)
		sent = body[:n]
		_, _ = w.Write([]byte(`{"model":"stub","choices":[{"message":{"content":"{\"notes\":\"Low on space.\",\"fix_ids\":[\"temp.clear\",\"format.disk\"]}"}}]}`))
	}))
	t.Cleanup(endpoint.Close)

	s, logPath := adviceServer(t, func(log *consent.Log) *assist.Assistant {
		return assist.New(
			assist.Config{Enabled: true, Endpoint: endpoint.URL, Model: "stub"},
			fixes.Default, platform.Linux, log,
		)
	})

	var payload assist.Payload
	call(t, s, http.MethodPost, "/api/assist/prepare", &payload, `{"hostnames":true,"usernames":true,"serials":true,"addresses":true}`)

	body, err := json.Marshal(map[string]string{"token": payload.Token})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var answer assist.Answer
	call(t, s, http.MethodPost, "/api/assist/ask", &answer, string(body))

	// What went out is what was shown, byte for byte.
	if string(sent) != payload.Body {
		t.Error("what was sent differs from what the user was shown")
	}
	// The invented ID did not survive the registry.
	if len(answer.Fixes) != 1 || answer.Fixes[0] != "temp.clear" {
		t.Errorf("Fixes = %v, want only the compiled-in repair", answer.Fixes)
	}
	if answer.Discarded != 1 {
		t.Errorf("Discarded = %d, want 1", answer.Discarded)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(raw), string(consent.EventDataSent)) {
		t.Errorf("the send left no DATA_SENT record:\n%s", raw)
	}
}

func TestAskingWithoutAConfirmedPayloadIsRefused(t *testing.T) {
	var reached int
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	t.Cleanup(endpoint.Close)

	s, _ := adviceServer(t, func(log *consent.Log) *assist.Assistant {
		return assist.New(
			assist.Config{Enabled: true, Endpoint: endpoint.URL, Model: "stub"},
			fixes.Default, platform.Linux, log,
		)
	})

	res := do(t, s, http.MethodPost, "/api/assist/ask", s.token, nil, `{"token":"invented"}`)
	res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
	if reached != 0 {
		t.Error("something was sent without a confirmed payload")
	}
}

func TestDiscardingAPayloadLeavesNothingToConfirm(t *testing.T) {
	s, logPath := adviceServer(t, func(log *consent.Log) *assist.Assistant {
		return assist.New(
			assist.Config{Enabled: true, Endpoint: "https://example.invalid/v1", Model: "stub"},
			fixes.Default, platform.Linux, log,
		)
	})

	var payload assist.Payload
	call(t, s, http.MethodPost, "/api/assist/prepare", &payload, `{}`)

	body, err := json.Marshal(map[string]string{"token": payload.Token})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var discarded map[string]string
	call(t, s, http.MethodPost, "/api/assist/discard", &discarded, string(body))

	res := do(t, s, http.MethodPost, "/api/assist/ask", s.token, nil, string(body))
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 after the payload was discarded", res.StatusCode)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(raw), string(consent.EventConsentDenied)) {
		t.Errorf("declining to send left no record:\n%s", raw)
	}
}

func TestTheAdviceRoutesNeedTheSessionToken(t *testing.T) {
	s, _ := adviceServer(t, nil)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/explain"},
		{http.MethodGet, "/api/assist"},
		{http.MethodPost, "/api/assist/prepare"},
		{http.MethodPost, "/api/assist/ask"},
		{http.MethodPost, "/api/assist/discard"},
	} {
		res := do(t, s, route.method, route.path, "", nil, `{}`)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", route.method, route.path, res.StatusCode)
		}
	}
}
