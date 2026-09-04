package enrol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
)

func snapshot() checks.Snapshot {
	return checks.Snapshot{
		Schema:       checks.SnapshotSchema,
		AgentVersion: "test",
		GeneratedAt:  time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC),
		Host:         platform.Host{OS: platform.Linux, Arch: "amd64"},
		Results: []checks.Result{{
			CheckID:  "disk.volumes",
			Severity: checks.SeverityAttention,
			Summary:  "check.disk.volumes.low",
			Detail:   map[string]any{"hostname": "alex-laptop", "serial": "SN-12345"},
		}},
	}
}

var identity = redact.Identity{Hostname: "alex-laptop", Username: "alex", HomeDir: "/home/alex"}

func server(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
}

func accepted(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"stored","machine":"1f418b38d486e206002b154d773140ca"}`))
}

func newEnroller(t *testing.T, url, name string) (*Enroller, string) {
	t.Helper()

	logPath := filepath.Join(t.TempDir(), "audit.log")
	log, err := consent.Open(logPath)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	e := New(Config{Enabled: true, Server: url, Name: name}, log, "test")
	e.key = func() string { return "a-real-token-long-enough-to-use" }
	return e, logPath
}

// TestOffByDefault is the first property: a zero Config sends nothing and
// prepares nothing.
func TestOffByDefault(t *testing.T) {
	e := New(Config{}, nil, "test")

	if e.Enabled() {
		t.Error("Enabled = true for a zero configuration")
	}
	if _, err := e.Prepare(snapshot(), redact.Everything(), identity); !errors.Is(err, ErrDisabled) {
		t.Errorf("Prepare err = %v, want ErrDisabled", err)
	}
	if _, err := e.Send(context.Background(), "anything"); !errors.Is(err, ErrDisabled) {
		t.Errorf("Send err = %v, want ErrDisabled", err)
	}
}

func TestPreparingSendsNothing(t *testing.T) {
	var reached int
	s := server(t, func(w http.ResponseWriter, r *http.Request) {
		reached++
		accepted(w, r)
	})

	e, _ := newEnroller(t, s.URL, "Reception PC")
	payload, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if reached != 0 {
		t.Errorf("the server was contacted %d times while preparing", reached)
	}
	if payload.Body == "" || payload.Token == "" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Host == "" {
		t.Error("the payload does not name the host it would go to")
	}
}

func TestThePayloadIsTheRedactedSnapshot(t *testing.T) {
	e, _ := newEnroller(t, "https://fleet.example.com", "Reception PC")

	payload, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	for _, secret := range []string{"alex-laptop", "SN-12345", "/home/alex"} {
		if strings.Contains(payload.Body, secret) {
			t.Errorf("the payload still carries %q after full redaction", secret)
		}
	}
	if !payload.Redacted {
		t.Error("Redacted = false though a policy was applied")
	}
	// The name the user chose is what identifies the machine, and it is in
	// the payload they were shown.
	if !strings.Contains(payload.Body, "Reception PC") {
		t.Error("the payload does not carry the name the machine reports under")
	}
}

// TestAMachineNeedsANameItWasGiven: what a machine is called in someone
// else's dashboard is a decision, not something to harvest from the hostname.
func TestAMachineNeedsANameItWasGiven(t *testing.T) {
	for _, name := range []string{"", "   "} {
		e, _ := newEnroller(t, "https://fleet.example.com", name)
		if _, err := e.Prepare(snapshot(), redact.Everything(), identity); !errors.Is(err, ErrNoName) {
			t.Errorf("Prepare with name %q: err = %v, want ErrNoName", name, err)
		}
	}
}

func TestAnEndpointThatWouldSendInTheClearIsRefused(t *testing.T) {
	for _, url := range []string{"http://fleet.example.com", "ftp://example.com", "", "not a url"} {
		e, _ := newEnroller(t, url, "Reception PC")
		if _, err := e.Prepare(snapshot(), redact.Everything(), identity); err == nil {
			t.Errorf("Prepare accepted the server %q", url)
		}
	}

	// A fleet server being tried out on the same machine is fine: the
	// traffic never leaves it.
	e, _ := newEnroller(t, "http://127.0.0.1:8080", "Reception PC")
	if _, err := e.Prepare(snapshot(), redact.Everything(), identity); err != nil {
		t.Errorf("Prepare refused a loopback server: %v", err)
	}
}

// TestNoCredentialIsCaughtBeforeTheUserIsAsked: better to say so now than to
// build a payload, show it, take a confirmation, and only then discover there
// is nothing to send with.
func TestNoCredentialIsCaughtBeforeTheUserIsAsked(t *testing.T) {
	e, _ := newEnroller(t, "https://fleet.example.com", "Reception PC")
	e.key = func() string { return "" }

	_, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("err = %v, want ErrNoCredential", err)
	}
	if !strings.Contains(err.Error(), TokenEnv) {
		t.Errorf("the error does not say what to set: %v", err)
	}
}

func TestSendRefusesWithoutTheTokenFromTheShownPayload(t *testing.T) {
	var reached int
	s := server(t, func(w http.ResponseWriter, r *http.Request) {
		reached++
		accepted(w, r)
	})

	e, _ := newEnroller(t, s.URL, "Reception PC")
	if _, err := e.Send(context.Background(), "invented"); !errors.Is(err, ErrNotConfirmed) {
		t.Errorf("err = %v, want ErrNotConfirmed", err)
	}
	if reached != 0 {
		t.Error("something was sent without a confirmed payload")
	}
}

func TestATokenIsGoodForOneSend(t *testing.T) {
	var reached int
	s := server(t, func(w http.ResponseWriter, r *http.Request) {
		reached++
		accepted(w, r)
	})

	e, _ := newEnroller(t, s.URL, "Reception PC")
	payload, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if _, err := e.Send(context.Background(), payload.Token); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := e.Send(context.Background(), payload.Token); !errors.Is(err, ErrNotConfirmed) {
		t.Errorf("replayed Send: err = %v, want ErrNotConfirmed", err)
	}
	if reached != 1 {
		t.Errorf("the server was contacted %d times, want 1", reached)
	}
}

func TestWhatIsSentIsWhatWasShown(t *testing.T) {
	var sent string
	var auth string
	s := server(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 1<<20)
		n, _ := r.Body.Read(body)
		sent = string(body[:n])
		auth = r.Header.Get("Authorization")
		accepted(w, r)
	})

	e, _ := newEnroller(t, s.URL, "Reception PC")
	payload, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	result, err := e.Send(context.Background(), payload.Token)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sent != payload.Body {
		t.Error("what was sent differs from what the user was shown")
	}
	if auth != "Bearer a-real-token-long-enough-to-use" {
		t.Errorf("Authorization = %q", auth)
	}
	if result.Machine == "" {
		t.Error("the server's identifier was not carried back")
	}
}

func TestDiscardDropsAPreparedPayload(t *testing.T) {
	e, _ := newEnroller(t, "https://fleet.example.com", "Reception PC")

	payload, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if e.Pending() != 1 {
		t.Errorf("Pending = %d, want 1", e.Pending())
	}

	e.Discard(payload.Token)
	if e.Pending() != 0 {
		t.Errorf("Pending = %d after Discard, want 0", e.Pending())
	}
	if _, err := e.Send(context.Background(), payload.Token); !errors.Is(err, ErrNotConfirmed) {
		t.Errorf("err = %v after the payload was discarded", err)
	}
}

func TestTheCredentialNeverReachesTheAuditLog(t *testing.T) {
	s := server(t, accepted)

	e, logPath := newEnroller(t, s.URL+"/?key=supersecret", "Reception PC")
	payload, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, err := e.Send(context.Background(), payload.Token); err != nil {
		t.Fatalf("Send: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	log := string(raw)

	for _, secret := range []string{"a-real-token-long-enough-to-use", "supersecret"} {
		if strings.Contains(log, secret) {
			t.Errorf("a credential reached the audit log:\n%s", log)
		}
	}
	if !strings.Contains(log, string(consent.EventDataSent)) {
		t.Errorf("the send left no DATA_SENT record:\n%s", log)
	}
	for _, want := range []string{"purpose=fleet report", "redacted=true", "delivered=true"} {
		if !strings.Contains(log, want) {
			t.Errorf("the entry does not record %q:\n%s", want, log)
		}
	}
}

func TestAFailedSendIsStillRecorded(t *testing.T) {
	s := server(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	})

	e, logPath := newEnroller(t, s.URL, "Reception PC")
	payload, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, err := e.Send(context.Background(), payload.Token); err == nil {
		t.Fatal("a refusal was reported as success")
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	// Bytes left the machine whether or not the server took them.
	if !strings.Contains(string(raw), "delivered=false") {
		t.Errorf("the failed send was not recorded:\n%s", raw)
	}
}

func TestAnUnreachableServerDoesNotQuoteTheURL(t *testing.T) {
	e, _ := newEnroller(t, "https://127.0.0.1:1/?key=supersecret", "Reception PC")
	e.client = &http.Client{Timeout: 100 * time.Millisecond}

	payload, err := e.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	_, err = e.Send(context.Background(), payload.Token)
	if err == nil {
		t.Fatal("an unreachable server was reported as success")
	}
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("the error quotes the URL's query: %v", err)
	}
}

func TestAReportTooLargeToSendIsRefusedBeforeItTravels(t *testing.T) {
	e, _ := newEnroller(t, "https://fleet.example.com", "Reception PC")

	huge := snapshot()
	filler := strings.Repeat("x", 8192)
	for i := 0; i < MaxRequestBytes/4096; i++ {
		huge.Results = append(huge.Results, checks.Result{
			CheckID: "filler", Summary: "check.os.info.ok",
			Detail: map[string]any{"data": filler},
		})
	}

	if _, err := e.Prepare(huge, redact.Policy{}, identity); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestASnapshotWithNoResultsIsNotWorthSending(t *testing.T) {
	e, _ := newEnroller(t, "https://fleet.example.com", "Reception PC")

	empty := checks.Snapshot{Schema: checks.SnapshotSchema, Host: platform.Host{OS: platform.Linux}}
	if _, err := e.Prepare(empty, redact.Everything(), identity); err == nil {
		t.Error("Prepare accepted a snapshot with nothing in it")
	}
}
