package assist

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
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"

	_ "github.com/ZanOzair/SupportOne/internal/fixes/all"
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

// server stands in for a model endpoint. No test here reaches a network.
func server(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
}

func answered(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// reply wraps model content in the response shape the endpoint speaks.
func reply(content string) string {
	escaped := strings.ReplaceAll(content, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	return `{"model":"stub-model","choices":[{"message":{"content":"` + escaped + `"}}]}`
}

func newAssistant(t *testing.T, endpoint string) (*Assistant, string) {
	t.Helper()

	logPath := filepath.Join(t.TempDir(), "audit.log")
	log, err := consent.Open(logPath)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	a := New(Config{Enabled: true, Endpoint: endpoint, Model: "stub-model"}, fixes.Default, platform.Linux, log)
	a.key = func() string { return "" }
	return a, logPath
}

// TestOffByDefault is the first property: a zero Config sends nothing and
// prepares nothing.
func TestOffByDefault(t *testing.T) {
	a := New(Config{}, fixes.Default, platform.Linux, nil)

	if a.Enabled() {
		t.Error("Enabled = true for a zero configuration")
	}
	if _, err := a.Prepare(snapshot(), redact.Everything(), identity); !errors.Is(err, ErrDisabled) {
		t.Errorf("Prepare err = %v, want ErrDisabled", err)
	}
	if _, err := a.Ask(context.Background(), "anything"); !errors.Is(err, ErrDisabled) {
		t.Errorf("Ask err = %v, want ErrDisabled", err)
	}
}

// TestPrepareSendsNothing is the second: what the user confirms is the bytes,
// and building them contacts nobody.
func TestPrepareSendsNothing(t *testing.T) {
	var reached int
	s := server(t, func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(reply(`{"notes":"ok","fix_ids":[]}`)))
	})

	a, _ := newAssistant(t, s.URL)
	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if reached != 0 {
		t.Errorf("the endpoint was contacted %d times while preparing", reached)
	}
	if payload.Bytes == 0 || payload.Body == "" {
		t.Error("the payload is empty")
	}
	if payload.Token == "" {
		t.Error("the payload carries no token, so nothing proves it was shown")
	}
	if payload.Host == "" {
		t.Error("the payload does not name the host it would go to")
	}
}

// TestThePayloadIsTheRedactedSnapshot is the third: what is shown is what
// leaves, redaction included.
func TestThePayloadIsTheRedactedSnapshot(t *testing.T) {
	a, _ := newAssistant(t, "https://example.invalid/v1/chat/completions")

	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
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

	// And with nothing redacted, the same content is there to be seen — the
	// user is choosing, not being protected from their own choice.
	open, err := a.Prepare(snapshot(), redact.Policy{}, identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if !strings.Contains(open.Body, "alex-laptop") {
		t.Error("an unredacted payload does not carry what it would send")
	}
	if open.Redacted {
		t.Error("Redacted = true for a policy that removes nothing")
	}
}

func TestAskRefusesWithoutTheTokenFromTheShownPayload(t *testing.T) {
	var reached int
	s := server(t, func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(reply(`{"notes":"ok","fix_ids":[]}`)))
	})

	a, _ := newAssistant(t, s.URL)
	if _, err := a.Ask(context.Background(), "invented"); !errors.Is(err, ErrNotConfirmed) {
		t.Errorf("err = %v, want ErrNotConfirmed", err)
	}
	if reached != 0 {
		t.Error("something was sent without a confirmed payload")
	}
}

func TestATokenIsGoodForOneSend(t *testing.T) {
	var reached int
	s := server(t, func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(reply(`{"notes":"ok","fix_ids":[]}`)))
	})

	a, _ := newAssistant(t, s.URL)
	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if _, err := a.Ask(context.Background(), payload.Token); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if _, err := a.Ask(context.Background(), payload.Token); !errors.Is(err, ErrNotConfirmed) {
		t.Errorf("replayed Ask err = %v, want ErrNotConfirmed", err)
	}
	if reached != 1 {
		t.Errorf("the endpoint was contacted %d times, want 1", reached)
	}
}

func TestDiscardDropsAPreparedPayload(t *testing.T) {
	a, _ := newAssistant(t, "https://example.invalid/v1/chat/completions")

	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if a.Pending() != 1 {
		t.Errorf("Pending = %d, want 1", a.Pending())
	}

	a.Discard(payload.Token)
	if a.Pending() != 0 {
		t.Errorf("Pending = %d after Discard, want 0", a.Pending())
	}
	if _, err := a.Ask(context.Background(), payload.Token); !errors.Is(err, ErrNotConfirmed) {
		t.Errorf("err = %v, want ErrNotConfirmed after the payload was discarded", err)
	}
}

// TestSuggestionsPassThroughTheRegistry is the containment rule: the model can
// name anything it likes, and only compiled-in IDs survive.
func TestSuggestionsPassThroughTheRegistry(t *testing.T) {
	content := `{"notes":"Your disk is filling up.","fix_ids":["temp.clear","rm -rf /","format.disk","net.flush-dns","print.clear-spooler"]}`
	s := server(t, answered(reply(content)))

	a, _ := newAssistant(t, s.URL)
	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	got, err := a.Ask(context.Background(), payload.Token)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}

	// temp.clear and net.flush-dns run on Linux; the shell string and the
	// invented ID resolve to nothing, and print.clear-spooler is Windows-only.
	want := map[string]bool{"temp.clear": true, "net.flush-dns": true}
	if len(got.Fixes) != len(want) {
		t.Fatalf("Fixes = %v, want %v", got.Fixes, want)
	}
	for _, id := range got.Fixes {
		if !want[id] {
			t.Errorf("Fixes carries %q, which should not have survived", id)
		}
	}
	if got.Discarded != 3 {
		t.Errorf("Discarded = %d, want 3", got.Discarded)
	}
	if got.Notes != "Your disk is filling up." {
		t.Errorf("Notes = %q", got.Notes)
	}
}

func TestABuildWithNoFixesDiscardsEverySuggestion(t *testing.T) {
	s := server(t, answered(reply(`{"notes":"hello","fix_ids":["temp.clear"]}`)))

	a, _ := newAssistant(t, s.URL)
	a.fixes = nil

	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	got, err := a.Ask(context.Background(), payload.Token)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(got.Fixes) != 0 || got.Discarded != 1 {
		t.Errorf("got %+v, want everything discarded", got)
	}
}

func TestTooManySuggestionsAreCappedAndCounted(t *testing.T) {
	ids := make([]string, 0, MaxFixIDs+5)
	for i := 0; i < MaxFixIDs+5; i++ {
		ids = append(ids, `"temp.clear"`)
	}
	content := `{"notes":"","fix_ids":[` + strings.Join(ids, ",") + `]}`
	s := server(t, answered(reply(content)))

	a, _ := newAssistant(t, s.URL)
	payload, _ := a.Prepare(snapshot(), redact.Everything(), identity)

	got, err := a.Ask(context.Background(), payload.Token)
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if got.Discarded < 5 {
		t.Errorf("Discarded = %d, want at least the 5 over the cap", got.Discarded)
	}
}

func TestAnEndpointThatRefusesIsReportedWithoutItsURL(t *testing.T) {
	s := server(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusUnauthorized)
	})

	a, _ := newAssistant(t, s.URL+"/v1/chat?key=supersecret")
	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	_, err = a.Ask(context.Background(), payload.Token)
	if err == nil {
		t.Fatal("a refusal was reported as success")
	}
	// A URL can carry a credential in its query; the error names the host.
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("the error quotes the URL's query: %v", err)
	}
}

func TestAnUnreachableEndpointDoesNotQuoteTheURL(t *testing.T) {
	a, _ := newAssistant(t, "https://127.0.0.1:1/v1/chat?key=supersecret")
	a.client = &http.Client{Timeout: 100 * time.Millisecond}

	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	_, err = a.Ask(context.Background(), payload.Token)
	if err == nil {
		t.Fatal("an unreachable endpoint was reported as success")
	}
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("the error quotes the URL's query: %v", err)
	}
}

func TestTheKeyIsSentAsAHeaderAndNeverLogged(t *testing.T) {
	var seen string
	s := server(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(reply(`{"notes":"ok","fix_ids":[]}`)))
	})

	a, logPath := newAssistant(t, s.URL)
	a.key = func() string { return "sk-a-real-looking-key" }

	payload, _ := a.Prepare(snapshot(), redact.Everything(), identity)
	if _, err := a.Ask(context.Background(), payload.Token); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	if seen != "Bearer sk-a-real-looking-key" {
		t.Errorf("Authorization = %q", seen)
	}
	// The payload the user was shown must not contain it either.
	if strings.Contains(payload.Body, "sk-a-real-looking-key") {
		t.Error("the credential is in the payload body")
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if strings.Contains(string(raw), "sk-a-real-looking-key") {
		t.Errorf("the credential reached the audit log:\n%s", raw)
	}
}

func TestNoKeyMeansNoAuthorizationHeader(t *testing.T) {
	var present bool
	s := server(t, func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["Authorization"]
		_, _ = w.Write([]byte(reply(`{"notes":"ok","fix_ids":[]}`)))
	})

	a, _ := newAssistant(t, s.URL)
	payload, _ := a.Prepare(snapshot(), redact.Everything(), identity)
	if _, err := a.Ask(context.Background(), payload.Token); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	// A local model server needs no key, and sending an empty bearer token
	// would make some of them refuse.
	if present {
		t.Error("an Authorization header was sent with no key configured")
	}
}

func TestEverySendReachesTheAuditLog(t *testing.T) {
	s := server(t, answered(reply(`{"notes":"ok","fix_ids":[]}`)))

	a, logPath := newAssistant(t, s.URL)
	payload, _ := a.Prepare(snapshot(), redact.Everything(), identity)
	if _, err := a.Ask(context.Background(), payload.Token); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	log := string(raw)

	if !strings.Contains(log, string(consent.EventDataSent)) {
		t.Errorf("no DATA_SENT entry:\n%s", log)
	}
	for _, want := range []string{"purpose=assistant", "redacted=true", "answered=true"} {
		if !strings.Contains(log, want) {
			t.Errorf("the entry does not record %q:\n%s", want, log)
		}
	}
}

func TestAFailedSendIsStillRecorded(t *testing.T) {
	s := server(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	})

	a, logPath := newAssistant(t, s.URL)
	payload, _ := a.Prepare(snapshot(), redact.Everything(), identity)
	if _, err := a.Ask(context.Background(), payload.Token); err == nil {
		t.Fatal("a failing endpoint was reported as success")
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	// Bytes left the machine whether or not the answer was any good.
	if !strings.Contains(string(raw), "answered=false") {
		t.Errorf("the failed send was not recorded:\n%s", raw)
	}
}

func TestAHugeResponseIsNotReadWhole(t *testing.T) {
	s := server(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"`))
		for i := 0; i < MaxResponseBytes/16+64; i++ {
			_, _ = w.Write([]byte("aaaaaaaaaaaaaaaa"))
		}
		_, _ = w.Write([]byte(`"}}]}`))
	})

	a, _ := newAssistant(t, s.URL)
	payload, _ := a.Prepare(snapshot(), redact.Everything(), identity)

	// Truncated at the cap, so it no longer parses. Refusing is the right
	// outcome; exhausting memory is not.
	if _, err := a.Ask(context.Background(), payload.Token); err == nil {
		t.Error("an oversized answer was accepted")
	}
}

func TestARequestLargerThanTheCapIsRefusedBeforeSending(t *testing.T) {
	a, _ := newAssistant(t, "https://example.invalid/v1/chat/completions")

	huge := snapshot()
	filler := strings.Repeat("x", 4096)
	for i := 0; i < MaxRequestBytes/2048; i++ {
		huge.Results = append(huge.Results, checks.Result{
			CheckID: "filler", Summary: "check.os.info.ok",
			Detail: map[string]any{"data": filler},
		})
	}

	if _, err := a.Prepare(huge, redact.Policy{}, identity); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestTheRequestNamesOnlyTheFixesThisBuildCarries(t *testing.T) {
	a, _ := newAssistant(t, "https://example.invalid/v1/chat/completions")

	payload, err := a.Prepare(snapshot(), redact.Everything(), identity)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if !strings.Contains(payload.Body, "available_fixes") {
		t.Error("the request does not tell the model which repairs exist")
	}
	if !strings.Contains(payload.Body, "temp.clear") {
		t.Error("the request omits a repair that runs here")
	}
	// print.clear-spooler is Windows-only and this assistant is on Linux.
	if strings.Contains(payload.Body, "print.clear-spooler") {
		t.Error("the request offers a repair that does not run on this platform")
	}
}
