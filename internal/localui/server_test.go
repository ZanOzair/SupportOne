package localui

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/platform"
	"github.com/ZanOzair/supportone/internal/redact"
)

func testSnapshot(context.Context) checks.Snapshot {
	return checks.Snapshot{
		Schema:       checks.SnapshotSchema,
		AgentVersion: "test",
		GeneratedAt:  time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC),
		Host:         platform.Host{OS: platform.Linux, Arch: "amd64"},
		Results: []checks.Result{{
			CheckID:  "network.config",
			Severity: checks.SeverityOK,
			Summary:  "check.network.config.ok",
			Args:     []any{"eth0", "192.168.1.1"},
			Detail:   map[string]any{"gateway": "192.168.1.1", "hostname": "alex-laptop"},
		}},
	}
}

func startServer(t *testing.T) *Server {
	t.Helper()

	s, err := New(Config{
		Assets:      testAssets(),
		Snapshot:    testSnapshot,
		Version:     "test",
		Host:        platform.Host{OS: platform.Linux, Arch: "amd64"},
		Identity:    redact.Identity{Hostname: "alex-laptop", Username: "alex", HomeDir: "/home/alex"},
		IdleTimeout: time.Minute,
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

func TestListensOnLoopbackOnly(t *testing.T) {
	s := startServer(t)

	host, port, err := net.SplitHostPort(s.Addr())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Errorf("listening on %q, which is not loopback", host)
	}
	if port == "0" || port == "80" || port == "8080" {
		t.Errorf("listening on a predictable port %q", port)
	}
}

func TestSessionTokenIsRequiredAndUnguessable(t *testing.T) {
	s := startServer(t)

	if len(s.token) < 40 {
		t.Errorf("session token is %d characters; too short to resist guessing", len(s.token))
	}

	// Every API route must refuse an unauthenticated request.
	for _, route := range []string{"/api/session", "/api/snapshot", "/api/report", "/api/messages"} {
		res := do(t, s, http.MethodGet, route, "", nil)
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without a token = %d, want 401", route, res.StatusCode)
		}
		res.Body.Close()
	}

	res := do(t, s, http.MethodGet, "/api/session", s.token+"x", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("a wrong token was accepted with %d", res.StatusCode)
	}
}

func TestRejectsRequestsWithAForeignHostHeader(t *testing.T) {
	s := startServer(t)

	// A rebound DNS name resolves to 127.0.0.1 but cannot forge the Host
	// header the browser sends.
	req, err := http.NewRequest(http.MethodGet, "http://"+s.Addr()+"/api/session?t="+s.token, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, port, _ := net.SplitHostPort(s.Addr())
	req.Host = "attacker.example:" + port

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("a rebound Host header was accepted with %d, want 403", res.StatusCode)
	}
}

func TestRejectsRequestsWithAForeignOrigin(t *testing.T) {
	s := startServer(t)

	res := do(t, s, http.MethodGet, "/api/session", s.token, map[string]string{
		"Origin": "https://attacker.example",
	})
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("a foreign Origin was accepted with %d, want 403", res.StatusCode)
	}
}

func TestSendsAStrictContentSecurityPolicy(t *testing.T) {
	s := startServer(t)

	res := do(t, s, http.MethodGet, "/api/session", s.token, nil)
	defer res.Body.Close()

	policy := res.Header.Get("Content-Security-Policy")
	for _, required := range []string{"default-src 'none'", "script-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(policy, required) {
			t.Errorf("policy %q is missing %q", policy, required)
		}
	}
	if strings.Contains(policy, "unsafe-inline") || strings.Contains(policy, "http") {
		t.Errorf("policy %q allows inline script or a remote origin", policy)
	}
}

func TestPreviewShowsExactlyWhatRedactionLeaves(t *testing.T) {
	s := startServer(t)

	res := do(t, s, http.MethodPost, "/api/preview", s.token, map[string]string{
		"Content-Type": "application/json",
	}, `{"hostnames":true,"usernames":true,"serials":true,"addresses":true}`)
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.Contains(string(body), "alex-laptop") || strings.Contains(string(body), "192.168.1.1") {
		t.Errorf("the preview still contains identifying detail:\n%s", body)
	}
	if !strings.Contains(string(body), redact.Marker) {
		t.Errorf("nothing in the preview is marked as redacted:\n%s", body)
	}
}

func TestReportDownloadsWithTheChosenRedaction(t *testing.T) {
	s := startServer(t)

	res := do(t, s, http.MethodGet, "/api/report?format=json&hostnames=1&addresses=1&t="+s.token, s.token, nil)
	defer res.Body.Close()

	if disposition := res.Header.Get("Content-Disposition"); !strings.HasPrefix(disposition, "attachment;") {
		t.Errorf("Content-Disposition = %q, want an attachment", disposition)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if strings.Contains(string(body), "alex-laptop") {
		t.Errorf("the saved report still names the machine:\n%s", body)
	}

	var snap checks.Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("saved report is not valid JSON: %v", err)
	}
	if len(snap.Results) != 1 {
		t.Errorf("saved report has %d results, want 1", len(snap.Results))
	}
}

func TestStaticInterfaceNeedsNoToken(t *testing.T) {
	s := startServer(t)

	// The page itself carries no data — the snapshot only ever arrives through
	// the API — so it is served without a token, and the page then uses the
	// token from the URL the agent opened.
	res := do(t, s, http.MethodGet, "/", "", nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("GET / = %d, want 200", res.StatusCode)
	}
}

func TestURLCarriesTheToken(t *testing.T) {
	s := startServer(t)

	if !strings.HasPrefix(s.URL(), "http://127.0.0.1:") {
		t.Errorf("URL = %q, want a loopback address", s.URL())
	}
	if !strings.Contains(s.URL(), s.token) {
		t.Error("the opened URL does not carry the session token")
	}
}

func TestShutsDownWhenIdle(t *testing.T) {
	s, err := New(Config{
		Assets:      testAssets(),
		Snapshot:    testSnapshot,
		IdleTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("the server was still running long after it went idle")
		s.Close()
	}
}

func do(t *testing.T, s *Server, method, path, token string, headers map[string]string, body ...string) *http.Response {
	t.Helper()

	var reader io.Reader
	if len(body) > 0 {
		reader = strings.NewReader(body[0])
	}
	req, err := http.NewRequest(method, "http://"+s.Addr()+path, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do %s %s: %v", method, path, err)
	}
	return res
}

func TestMessagesServeTheCatalogTheInterfaceRenders(t *testing.T) {
	s := startServer(t)

	res := do(t, s, http.MethodGet, "/api/messages?lang=ms", s.token, nil)
	defer res.Body.Close()

	var catalog struct {
		Lang     string            `json:"lang"`
		Messages map[string]string `json:"messages"`
	}
	if err := json.NewDecoder(res.Body).Decode(&catalog); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if catalog.Lang != "ms" {
		t.Errorf("lang = %q, want ms", catalog.Lang)
	}
	if catalog.Messages["severity.urgent"] != "Mendesak" {
		t.Errorf("the catalog did not come back translated: %q", catalog.Messages["severity.urgent"])
	}
	// English fills any gap, so the interface never renders a bare key.
	if catalog.Messages["check.os.info.ok"] == "" {
		t.Error("a key present in English is missing from the served catalog")
	}
}

func TestRefreshRunsTheChecksAgainAndRecordsIt(t *testing.T) {
	s := startServer(t)

	var runs int
	s.cfg.Snapshot = func(ctx context.Context) checks.Snapshot {
		runs++
		return testSnapshot(ctx)
	}

	res := do(t, s, http.MethodGet, "/api/snapshot", s.token, nil)
	res.Body.Close()
	res = do(t, s, http.MethodGet, "/api/snapshot", s.token, nil)
	res.Body.Close()
	if runs != 1 {
		t.Errorf("checks ran %d times for two reads; the session should show one snapshot", runs)
	}

	res = do(t, s, http.MethodPost, "/api/snapshot", s.token, nil)
	res.Body.Close()
	if runs != 2 {
		t.Errorf("checks ran %d times, want a re-run when the user asks", runs)
	}
}

func TestPreviewRejectsAnOversizedPolicy(t *testing.T) {
	s := startServer(t)

	res := do(t, s, http.MethodPost, "/api/preview", s.token,
		map[string]string{"Content-Type": "application/json"},
		`{"hostnames":true,"padding":"`+strings.Repeat("x", 8192)+`"}`)
	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		t.Error("an oversized request body was accepted")
	}
}

func TestReportRefusesAnUnknownFormat(t *testing.T) {
	s := startServer(t)

	res := do(t, s, http.MethodGet, "/api/report?format=pdf", s.token, nil)
	defer res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want a refusal for a format the agent cannot write", res.StatusCode)
	}
}

func TestCloseStopsTheServer(t *testing.T) {
	s, err := New(Config{Assets: testAssets(), Snapshot: testSnapshot, IdleTimeout: time.Minute})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background()) }()

	res := do(t, s, http.MethodPost, "/api/close", s.token, nil)
	res.Body.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("the server kept running after the user closed the session")
		s.Close()
	}
}
