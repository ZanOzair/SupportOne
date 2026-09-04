package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

const testToken = "a-token-long-enough-to-be-a-token"

// serve starts a server on loopback and returns its base URL.
func serve(t *testing.T) (*Server, *Store, string, *bytes.Buffer) {
	t.Helper()

	s := store(t)
	logs := new(bytes.Buffer)

	server, err := New(Config{
		Store:  s,
		Token:  testToken,
		Lang:   "en",
		Logger: slog.New(slog.NewTextHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	go func() {
		close(ready)
		if err := server.Serve(ctx, "127.0.0.1:0"); err != nil {
			t.Errorf("Serve: %v", err)
		}
	}()
	<-ready

	// Wait for the listener to exist before handing back its address.
	deadline := time.Now().Add(2 * time.Second)
	for server.Addr() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if server.Addr() == "" {
		t.Fatal("the server never started listening")
	}

	t.Cleanup(func() {
		cancel()
		_ = server.Close()
	})
	return server, s, "http://" + server.Addr(), logs
}

func do(t *testing.T, method, url string, body string, auth func(*http.Request)) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if auth != nil {
		auth(req)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return res
}

func bearer(req *http.Request)     { req.Header.Set("Authorization", "Bearer "+testToken) }
func basic(req *http.Request)      { req.SetBasicAuth("technician", testToken) }
func wrongToken(req *http.Request) { req.Header.Set("Authorization", "Bearer not-the-token") }

func reportJSON(t *testing.T, name string, results ...checks.Result) string {
	t.Helper()

	raw, err := json.Marshal(report(name, results...))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// TestTheServerWillNotServeAFleetWithoutAToken is the first rule: an
// unauthenticated fleet dashboard is a list of other people's machines.
func TestTheServerWillNotServeAFleetWithoutAToken(t *testing.T) {
	s := store(t)

	for _, token := range []string{"", "   ", "short"} {
		if _, err := New(Config{Store: s, Token: token}); err == nil {
			t.Errorf("New accepted the token %q", token)
		}
	}
	if _, err := New(Config{Token: testToken}); err == nil {
		t.Error("New accepted a nil store")
	}
}

func TestSubmittingAReportNeedsTheToken(t *testing.T) {
	_, s, base, _ := serve(t)

	for name, auth := range map[string]func(*http.Request){
		"no credential": nil,
		"a wrong token": wrongToken,
		// Basic auth is for the dashboard; it is not a submission credential.
		"basic auth": basic,
	} {
		t.Run(name, func(t *testing.T) {
			res := do(t, http.MethodPost, base+"/api/report", reportJSON(t, "Reception PC"), auth)
			res.Body.Close()
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", res.StatusCode)
			}
		})
	}

	machines, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(machines) != 0 {
		t.Errorf("an unauthenticated report was stored: %+v", machines)
	}
}

func TestReadingTheDashboardNeedsTheToken(t *testing.T) {
	_, _, base, _ := serve(t)

	for _, path := range []string{"/", "/machine/" + MachineID("Reception PC")} {
		res := do(t, http.MethodGet, base+path, "", nil)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s = %d, want 401", path, res.StatusCode)
		}
		// The browser needs to know how to ask.
		if !strings.Contains(res.Header.Get("WWW-Authenticate"), "Basic") {
			t.Errorf("%s does not prompt for a credential", path)
		}
	}
}

func TestAReportIsStoredAndAppearsOnTheDashboard(t *testing.T) {
	_, _, base, logs := serve(t)

	urgent := checks.Result{CheckID: "disk.smart", Severity: checks.SeverityUrgent, Summary: "check.disk.smart.failing"}

	res := do(t, http.MethodPost, base+"/api/report", reportJSON(t, "Reception PC", urgent), bearer)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(res.Body); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(body.String(), MachineID("Reception PC")) {
		t.Errorf("the response does not name the stored machine: %s", body)
	}

	// The log records the machine's ID, never its name: a fleet server's log
	// should not become a directory of whose computer is whose.
	if strings.Contains(logs.String(), "Reception PC") {
		t.Errorf("the machine's name reached the log:\n%s", logs)
	}
	if !strings.Contains(logs.String(), MachineID("Reception PC")) {
		t.Errorf("the report was not logged:\n%s", logs)
	}

	page := do(t, http.MethodGet, base+"/", "", basic)
	defer page.Body.Close()

	rendered := new(bytes.Buffer)
	if _, err := rendered.ReadFrom(page.Body); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(rendered.String(), "Reception PC") {
		t.Errorf("the dashboard does not list the machine:\n%s", rendered)
	}
	if !strings.Contains(rendered.String(), "Urgent") {
		t.Error("the dashboard does not show the urgent finding")
	}
}

func TestTheMachinePageShowsTheOfflineExplanation(t *testing.T) {
	_, _, base, _ := serve(t)

	failing := checks.Result{CheckID: "disk.smart", Severity: checks.SeverityUrgent, Summary: "check.disk.smart.failing"}
	res := do(t, http.MethodPost, base+"/api/report", reportJSON(t, "Reception PC", failing), bearer)
	res.Body.Close()

	page := do(t, http.MethodGet, base+"/machine/"+MachineID("Reception PC"), "", basic)
	defer page.Body.Close()

	rendered := new(bytes.Buffer)
	if _, err := rendered.ReadFrom(page.Body); err != nil {
		t.Fatalf("read: %v", err)
	}
	body := rendered.String()

	// A technician reads the same explanation the person at the machine read.
	if !strings.Contains(body, "What this means:") {
		t.Error("the machine page carries no explanation")
	}
	if !strings.Contains(body, "Copy anything you would hate to lose") {
		t.Error("the machine page carries no next steps")
	}
	// And is told the blanks are deliberate.
	if !strings.Contains(body, "removed identifying details") {
		t.Error("the page does not say the report was redacted")
	}
}

func TestAMachineThatIsNotThere(t *testing.T) {
	_, _, base, _ := serve(t)

	res := do(t, http.MethodGet, base+"/machine/"+MachineID("never reported"), "", basic)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}

	// A path that is not an identifier at all is refused the same way, not
	// with a server error that says something about the filesystem.
	res = do(t, http.MethodGet, base+"/machine/not-an-id", "", basic)
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestAMalformedOrOversizedReportIsRefused(t *testing.T) {
	_, s, base, _ := serve(t)

	res := do(t, http.MethodPost, base+"/api/report", "not json", bearer)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("malformed body = %d, want 400", res.StatusCode)
	}

	res = do(t, http.MethodPost, base+"/api/report", `{"name":"","snapshot":{}}`, bearer)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("empty report = %d, want 400", res.StatusCode)
	}

	huge := `{"name":"Reception PC","snapshot":{"schema":1,"results":[]},"filler":"` +
		strings.Repeat("x", MaxReportBytes) + `"}`
	res = do(t, http.MethodPost, base+"/api/report", huge, bearer)
	res.Body.Close()
	if res.StatusCode == http.StatusOK {
		t.Error("an oversized report was accepted")
	}

	machines, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(machines) != 0 {
		t.Errorf("a refused report was stored: %+v", machines)
	}
}

func TestHealthNeedsNoCredentialAndCarriesNoData(t *testing.T) {
	_, _, base, _ := serve(t)

	res := do(t, http.MethodGet, base+"/healthz", "", nil)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(res.Body); err != nil {
		t.Fatalf("read: %v", err)
	}
	// A load balancer should not have to be handed the fleet's credential,
	// and should not learn anything from asking.
	if strings.TrimSpace(body.String()) != "ok" {
		t.Errorf("body = %q, want just ok", body)
	}
}

func TestEveryResponseCarriesTheHardeningHeaders(t *testing.T) {
	_, _, base, _ := serve(t)

	res := do(t, http.MethodGet, base+"/", "", basic)
	res.Body.Close()

	if got := res.Header.Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'none'") {
		t.Errorf("Content-Security-Policy = %q", got)
	}
	// The dashboard is rendered on the server, so there is no script at all.
	if strings.Contains(res.Header.Get("Content-Security-Policy"), "script-src") {
		t.Error("the policy allows script on a page that has none")
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	} {
		if got := res.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestTheDashboardSaysSoWhenThereIsNoFleetYet(t *testing.T) {
	_, _, base, _ := serve(t)

	res := do(t, http.MethodGet, base+"/", "", basic)
	defer res.Body.Close()

	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(res.Body); err != nil {
		t.Fatalf("read: %v", err)
	}
	// An empty dashboard explains why it is empty rather than looking broken.
	if !strings.Contains(body.String(), "No machine has sent a report yet") {
		t.Errorf("the empty dashboard says nothing:\n%s", body)
	}
	if !strings.Contains(body.String(), "Nothing appears on its own") {
		t.Error("the empty dashboard does not explain how a report arrives")
	}
}

// TestTheServerCannotReachAMachine states the shape of the thing: there is no
// route that asks a machine for anything.
func TestTheServerCannotReachAMachine(t *testing.T) {
	_, _, base, _ := serve(t)

	for _, path := range []string{"/api/collect", "/api/run", "/api/fix", "/api/command", "/api/agents"} {
		res := do(t, http.MethodPost, base+path, "{}", bearer)
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d; this server has no way to act on a machine", path, res.StatusCode)
		}
	}
}

func TestAMachineNameIsEscapedInTheDashboard(t *testing.T) {
	_, _, base, _ := serve(t)

	res := do(t, http.MethodPost, base+"/api/report", reportJSON(t, `<img src=x onerror=alert(1)>`), bearer)
	res.Body.Close()

	page := do(t, http.MethodGet, base+"/", "", basic)
	defer page.Body.Close()

	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(page.Body); err != nil {
		t.Fatalf("read: %v", err)
	}
	// A machine name comes from whoever sent the report, which is not
	// necessarily the person reading the dashboard.
	if strings.Contains(body.String(), "<img src=x") {
		t.Error("a machine name was rendered as markup rather than escaped")
	}
}
