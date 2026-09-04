package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/profile"
	"github.com/ZanOzair/SupportOne/internal/redact"
)

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--version", "--lang", "en"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "supportone-agent") {
		t.Errorf("stdout = %q, want build information", out)
	}
	if !strings.Contains(out, runtime.Version()) {
		t.Errorf("stdout = %q, want the Go toolchain that built it", out)
	}

	// The number is not the point. A version a program prints about itself is
	// not evidence, and the output has to say so rather than leaving a reader
	// to assume otherwise.
	if !strings.Contains(out, "not proof") {
		t.Errorf("the version output presents itself as evidence:\n%s", out)
	}
	if !strings.Contains(out, "SHA256SUMS") {
		t.Errorf("the version output does not point at a check worth something:\n%s", out)
	}
}

func TestAnUnpublishedBuildSaysSoInItsReport(t *testing.T) {
	// These tests run against a build nobody released, which is the case the
	// report must be honest about: a saved report is read later, often by
	// someone else.
	var stdout, stderr bytes.Buffer
	args := []string{"--text", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "development build") && !strings.Contains(out, "uncommitted changes") {
		t.Errorf("the report does not say this build was never published:\n%s", out[:min(600, len(out))])
	}
}

func TestConflictingOutputFlagsAreRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--json", "--text"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("run succeeded, want an error when two outputs are requested")
	}
}

func TestUnknownArgumentIsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"snapshot"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("run succeeded, want error for unexpected argument")
	}
}

func TestJSONSnapshotIsWellFormed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--json", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	var snap checks.Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snap); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v\n%s", err, stdout.String())
	}
	if snap.Schema != checks.SnapshotSchema {
		t.Errorf("schema = %d, want %d", snap.Schema, checks.SnapshotSchema)
	}
	if snap.GeneratedAt.IsZero() {
		t.Error("generated_at is not set")
	}
}

func TestSnapshotWritesAuditTrail(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.log")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--text", "--audit-log", auditPath, "--lang", "en"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries := readFile(t, auditPath)
	for _, want := range []string{"AGENT_START", "AGENT_STOP"} {
		if !strings.Contains(entries, want) {
			t.Errorf("audit log is missing %s:\n%s", want, entries)
		}
	}
}

func TestDryRunSaysNothingWillChange(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--text", "--dry-run", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "nothing on this computer will be changed") {
		t.Errorf("dry run output does not state that nothing changes:\n%s", stdout.String())
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// The tests below exercise the terminal repair flow against the real
// compiled-in registries. They use temp.clear, whose preflight refuses when
// there is nothing to clear, so nothing on the machine running the tests is
// changed by them.

func TestListFixesNamesWhatEachOneChanges(t *testing.T) {
	var stdout, stderr bytes.Buffer

	args := []string{"--list-fixes", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "temp.clear") {
		t.Errorf("the listing does not name temp.clear:\n%s", out)
	}
	// A listing that gives IDs and no explanation is a listing nobody can act
	// on.
	if !strings.Contains(out, "temporary files") {
		t.Errorf("the listing does not say what the repair does:\n%s", out)
	}
}

func TestListWizardsNamesTheProblemsTheyAreFor(t *testing.T) {
	var stdout, stderr bytes.Buffer

	args := []string{"--list-wizards", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "wizard.connection") {
		t.Errorf("the listing does not name the connection walkthrough:\n%s", stdout.String())
	}
}

func TestAFixIsDescribedBeforeAnythingIsAsked(t *testing.T) {
	stale := staleTempDir(t)

	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "temp.clear", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	// Empty input: the prompt gets nothing, and silence is not consent.
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"This will change:", "To undo:", "Nothing was changed."} {
		if !strings.Contains(out, want) {
			t.Errorf("the output is missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("the file was moved without a confirmation: %v", err)
	}
}

func TestAFixIsNotAppliedOnAnythingButItsOwnID(t *testing.T) {
	stale := staleTempDir(t)

	for _, answer := range []string{"y\n", "yes\n", "\n", "temp clear\n", "TEMP.CLEARED\n"} {
		var stdout, stderr bytes.Buffer
		args := []string{"--fix", "temp.clear", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

		if err := run(args, strings.NewReader(answer), &stdout, &stderr); err != nil {
			t.Fatalf("run with %q: %v", answer, err)
		}
		if !strings.Contains(stdout.String(), "Nothing was changed.") {
			t.Errorf("answering %q did not abort:\n%s", answer, stdout.String())
		}
		if _, err := os.Stat(stale); err != nil {
			t.Fatalf("answering %q moved the file: %v", answer, err)
		}
	}
}

func TestADryRunDescribesTheChangeAndMakesNone(t *testing.T) {
	stale := staleTempDir(t)

	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "temp.clear", "--dry-run", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	// Even typing the ID cannot make a dry run change anything: it is never
	// asked for.
	if err := run(args, strings.NewReader("temp.clear\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "nothing on this computer will be changed") {
		t.Errorf("the output does not say it is a dry run:\n%s", stdout.String())
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("a dry run moved the file: %v", err)
	}
}

func TestConfirmingByIDAppliesTheFixAndUndoPutsItBack(t *testing.T) {
	// This is the only test here that actually changes anything, so it runs
	// only where the restore mechanism provably creates nothing. On Windows
	// or macOS an available restore point would mean a real checkpoint or
	// snapshot on the machine running the tests, which no test should make.
	if runtime.GOOS != "linux" {
		t.Skip("this test applies a fix; it runs only where no restore point would be created")
	}

	stale := staleTempDir(t)

	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "temp.clear", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	// Linux has no restore point, so the second question is asked too, and
	// then the offer to undo.
	input := strings.NewReader("temp.clear\nyes\nundo\n")
	if err := run(args, input, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "There is no restore point") {
		t.Errorf("the user was not told there would be no restore point:\n%s", out)
	}
	if !strings.Contains(out, "The change was undone.") {
		t.Errorf("the undo was not reported:\n%s", out)
	}

	// The file is back, with what was in it.
	raw, err := os.ReadFile(stale)
	if err != nil {
		t.Fatalf("the file was not restored: %v", err)
	}
	if string(raw) != "litter" {
		t.Errorf("the restored file holds %q, want %q", raw, "litter")
	}
}

func TestAFixWithNothingToDoSaysSoRatherThanRunning(t *testing.T) {
	emptyTempDir(t)

	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "temp.clear", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	if err := run(args, strings.NewReader("temp.clear\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "will not run right now") {
		t.Errorf("the output does not say the repair was refused:\n%s", stdout.String())
	}
}

func TestAnIDThatIsNotCompiledInIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--fix", "rm.everything", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("run accepted a fix ID that is not in the registry")
	}
}

func TestFixAndWizardAreNotAskedForTogether(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--fix", "temp.clear", "--wizard", "wizard.connection"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("run accepted --fix and --wizard together")
	}
}

func TestAWizardRunsToTheEndAndHandsOver(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--wizard", "wizard.connection", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	// Every prompt gets an empty line, so nothing is changed whatever the
	// machine running the tests looks like.
	if err := run(args, strings.NewReader("\n\n\n\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "What was checked:") && !strings.Contains(out, "came back clean") {
		t.Errorf("the walkthrough did not hand over what it found:\n%s", out)
	}
}

func TestAWizardThatDoesNotRunHereIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--wizard", "wizard.nope", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}

	if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("run accepted a walkthrough ID that is not in the registry")
	}
}

// staleTempDir points this process's temporary directory at a fresh one
// holding a single file old enough for temp.clear to move, and returns that
// file's path. All three variables are set because each platform reads a
// different one.
func staleTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, dir)
	}

	path := filepath.Join(dir, "old.tmp")
	if err := os.WriteFile(path, []byte("litter"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return path
}

// emptyTempDir points this process's temporary directory at one with nothing
// in it.
func emptyTempDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, dir)
	}
}

func TestTheTextReportExplainsEveryFinding(t *testing.T) {
	var stdout, stderr bytes.Buffer

	args := []string{"--text", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	// Whatever this machine reports, every verdict it prints carries an
	// explanation beneath it — that is the phase's whole point.
	if !strings.Contains(out, "os.info") {
		t.Fatalf("the report has no findings at all:\n%s", out)
	}
	if !strings.Contains(out, "This is what the computer is running") {
		t.Errorf("the report carries no explanation:\n%s", out)
	}
}

func TestExplanationsCanBeTurnedOff(t *testing.T) {
	var stdout, stderr bytes.Buffer

	args := []string{"--text", "--no-explain", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "os.info") {
		t.Fatalf("the report has no findings at all:\n%s", out)
	}
	if strings.Contains(out, "This is what the computer is running") {
		t.Errorf("--no-explain still printed the explanation:\n%s", out)
	}
}

// TestTheAssistantNeedsAnEndpointAndAcceptsNoDefault is the first line of the
// egress gate: there is no endpoint to fall back to.
func TestTheAssistantNeedsAnEndpointAndAcceptsNoDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if err := run([]string{"--text", "--assist"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("--assist was accepted with no endpoint")
	}
}

func TestTheAssistantRefusesAnEndpointThatWouldSendInTheClear(t *testing.T) {
	refused := []string{
		"http://api.example.com/v1/chat/completions",
		"ftp://example.com/",
		"not a url at all",
	}

	for _, endpoint := range refused {
		var stdout, stderr bytes.Buffer
		args := []string{"--text", "--assist", "--assist-endpoint", endpoint}

		if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
			t.Errorf("--assist-endpoint %q was accepted", endpoint)
		}
	}
}

func TestTheAssistantShowsThePayloadAndSendsNothingWithoutSend(t *testing.T) {
	var reached int
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer endpoint.Close()

	var stdout, stderr bytes.Buffer
	args := []string{
		"--text", "--assist",
		"--assist-endpoint", endpoint.URL,
		"--assist-model", "stub",
		"--lang", "en",
		"--audit-log", filepath.Join(t.TempDir(), "audit.log"),
	}

	// An empty line at the prompt. Silence is not consent here either.
	if err := run(args, strings.NewReader("\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "This is exactly what would be sent:") {
		t.Errorf("the payload was not shown:\n%s", out)
	}
	if !strings.Contains(out, "Nothing was sent.") {
		t.Errorf("the refusal was not reported:\n%s", out)
	}
	if reached != 0 {
		t.Errorf("the endpoint was contacted %d times without a confirmation", reached)
	}
}

func TestTypingSendSendsExactlyWhatWasShown(t *testing.T) {
	var received string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		received = string(body)
		_, _ = w.Write([]byte(`{"model":"stub","choices":[{"message":{"content":"{\"notes\":\"Looks fine to me.\",\"fix_ids\":[\"temp.clear\",\"format.disk\"]}"}}]}`))
	}))
	defer endpoint.Close()

	var stdout, stderr bytes.Buffer
	args := []string{
		"--text", "--assist",
		"--assist-endpoint", endpoint.URL,
		"--assist-model", "stub",
		"--lang", "en",
		"--audit-log", filepath.Join(t.TempDir(), "audit.log"),
	}

	if err := run(args, strings.NewReader("send\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if received == "" {
		t.Fatal("nothing was sent after the send was confirmed")
	}
	// The terminal path always redacts, and what went out proves it: the
	// visible marker is there, and the hostname is not.
	if !strings.Contains(received, redact.Marker) {
		t.Error("nothing in the sent payload was redacted")
	}
	if host := searchableHostname(t); host != "" && strings.Contains(received, host) {
		t.Error("the sent payload carries this machine's hostname")
	}
	// The model's words are marked as the model's words.
	if !strings.Contains(out, "These are its words, not SupportOne's") {
		t.Errorf("the answer was not attributed to the model:\n%s", out)
	}
	if !strings.Contains(out, "Looks fine to me.") {
		t.Errorf("the model's notes were not shown:\n%s", out)
	}
	// The invented ID did not survive the registry, and the count says so.
	if !strings.Contains(out, "--fix temp.clear") {
		t.Errorf("the surviving suggestion was not offered:\n%s", out)
	}
	if strings.Contains(out, "format.disk") {
		t.Errorf("an ID this build does not carry reached the user:\n%s", out)
	}
	if !strings.Contains(out, "named 1 things this build does not carry") {
		t.Errorf("the discarded suggestion was not reported:\n%s", out)
	}
}

func TestNothingReachesTheNetworkWithoutAssist(t *testing.T) {
	var reached int
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(`{}`))
	}))
	defer endpoint.Close()

	// The endpoint is named but --assist is not given, so nothing is offered
	// and nothing is contacted.
	var stdout, stderr bytes.Buffer
	args := []string{
		"--text", "--assist-endpoint", endpoint.URL,
		"--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log"),
	}
	if err := run(args, strings.NewReader("send\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	if reached != 0 {
		t.Errorf("the endpoint was contacted %d times without --assist", reached)
	}
	if strings.Contains(stdout.String(), "This is exactly what would be sent:") {
		t.Error("a send was offered without --assist")
	}
}

// searchableHostname returns this machine's hostname when it is long enough
// to search for meaningfully, and "" when it is not.
//
// A build machine can be called "vm". Searching a payload for a two-character
// string matches package names, base64 and compressed bytes, so an assertion
// built on it proves nothing and fails at random. Where the name is too short,
// the redaction marker is the assertion that carries the weight.
func searchableHostname(t *testing.T) string {
	t.Helper()

	name, err := os.Hostname()
	if err != nil || len(name) < 6 {
		return ""
	}
	return name
}

func TestTheTicketBundleIsWrittenAndSentNowhere(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bundle.zip")

	shot := filepath.Join(dir, "screenshot.png")
	if err := os.WriteFile(shot, []byte("\x89PNG\r\n\x1a\nsome image bytes here"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"--ticket", out,
		"--describe", "The printer stopped working after lunch.",
		"--attach", shot,
		"--lang", "en",
		"--audit-log", filepath.Join(dir, "audit.log"),
	}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(stdout.String(), "sent nowhere") {
		t.Errorf("the output does not say the bundle went nowhere:\n%s", stdout.String())
	}

	archive, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer archive.Close()

	names := make(map[string]bool)
	for _, f := range archive.File {
		names[f.Name] = true
	}
	for _, want := range []string{"ticket.json", "report.html", "attachments/screenshot.png"} {
		if !names[want] {
			t.Errorf("the bundle has no %s", want)
		}
	}
}

func TestTheTicketBundleIsRedacted(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bundle.zip")

	var stdout, stderr bytes.Buffer
	args := []string{"--ticket", out, "--lang", "en", "--audit-log", filepath.Join(dir, "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	archive, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer archive.Close()

	var manifest []byte
	for _, f := range archive.File {
		if f.Name != "ticket.json" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatalf("open ticket.json: %v", err)
		}
		manifest, err = io.ReadAll(io.LimitReader(r, 4<<20))
		_ = r.Close()
		if err != nil {
			t.Fatalf("read ticket.json: %v", err)
		}
	}
	if manifest == nil {
		t.Fatal("the bundle has no ticket.json")
	}

	var decoded struct {
		Redacted bool `json:"redacted"`
	}
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatalf("decode ticket.json: %v", err)
	}
	// A bundle is built to be handed to someone else, so the terminal path
	// redacts fully rather than asking field by field.
	if !decoded.Redacted {
		t.Error("the bundle does not record that it was redacted")
	}

	// And redaction actually ran, which the visible marker proves. Searching
	// the archive for this machine's hostname would not: a hostname can be
	// two characters long, and a substring that short matches noise inside a
	// compressed archive.
	if !bytes.Contains(manifest, []byte(redact.Marker)) {
		t.Errorf("nothing in the bundle was redacted; expected %q somewhere", redact.Marker)
	}
}

func TestTheBundleCanBeWrittenIntoAFolder(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	args := []string{"--ticket", dir, "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "supportone-ticket-") {
		t.Errorf("the folder holds %v, want one named bundle", entries)
	}
}

func TestAttachOnlyMeansSomethingWithATicket(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--attach", "shot.png"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("--attach was accepted without --ticket")
	}
}

func TestABundleRefusesAnAttachmentThatIsNotAnImage(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(secret, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"--ticket", filepath.Join(dir, "bundle.zip"),
		"--attach", secret,
		"--lang", "en",
		"--audit-log", filepath.Join(dir, "audit.log"),
	}
	// A support bundle is not a way to move anything off a machine under a
	// support label.
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("a private key was accepted as an attachment")
	}
}

// TestReportingToAFleetNeedsAServerAndAName is the first line of the second
// egress gate: there is nothing to fall back to.
func TestReportingToAFleetNeedsAServerAndAName(t *testing.T) {
	cases := [][]string{
		{"--text", "--report"},
		{"--text", "--report", "--fleet-server", "https://fleet.example.com"},
		{"--text", "--report", "--fleet-name", "Reception PC"},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
			t.Errorf("run accepted %v", args)
		}
	}
}

func TestAFleetServerThatWouldSendInTheClearIsRefused(t *testing.T) {
	for _, server := range []string{"http://fleet.example.com", "ftp://example.com", "not a url"} {
		var stdout, stderr bytes.Buffer
		args := []string{"--text", "--report", "--fleet-server", server, "--fleet-name", "Reception PC"}

		if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
			t.Errorf("--fleet-server %q was accepted", server)
		}
	}
}

func TestTheFleetReportShowsThePayloadAndSendsNothingWithoutSend(t *testing.T) {
	var reached int
	fleetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(`{"status":"stored","machine":"abc"}`))
	}))
	defer fleetServer.Close()

	t.Setenv("SUPPORTONE_FLEET_TOKEN", "a-real-token-long-enough-to-use")

	var stdout, stderr bytes.Buffer
	args := []string{
		"--text", "--report",
		"--fleet-server", fleetServer.URL,
		"--fleet-name", "Reception PC",
		"--lang", "en", "--no-explain",
		"--audit-log", filepath.Join(t.TempDir(), "audit.log"),
	}

	// An empty line at the prompt. Silence is not consent here either.
	if err := run(args, strings.NewReader("\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "This is exactly what would be sent:") {
		t.Errorf("the payload was not shown:\n%s", out)
	}
	if !strings.Contains(out, "Nothing was sent.") {
		t.Errorf("the refusal was not reported:\n%s", out)
	}
	if reached != 0 {
		t.Errorf("the server was contacted %d times without a confirmation", reached)
	}
}

func TestTypingSendReportsToTheFleet(t *testing.T) {
	var received string
	fleetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		received = string(body)
		_, _ = w.Write([]byte(`{"status":"stored","machine":"abc"}`))
	}))
	defer fleetServer.Close()

	t.Setenv("SUPPORTONE_FLEET_TOKEN", "a-real-token-long-enough-to-use")

	var stdout, stderr bytes.Buffer
	args := []string{
		"--text", "--report",
		"--fleet-server", fleetServer.URL,
		"--fleet-name", "Reception PC",
		"--lang", "en", "--no-explain",
		"--audit-log", filepath.Join(t.TempDir(), "audit.log"),
	}
	if err := run(args, strings.NewReader("send\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	if received == "" {
		t.Fatal("nothing was sent after the send was confirmed")
	}
	if !strings.Contains(received, "Reception PC") {
		t.Error("the report does not carry the name the user chose")
	}
	// The terminal path always redacts, and what went out proves it.
	if !strings.Contains(received, redact.Marker) {
		t.Error("nothing in the sent report was redacted")
	}
	if host := searchableHostname(t); host != "" && strings.Contains(received, host) {
		t.Error("the report carries this machine's hostname")
	}
	if !strings.Contains(stdout.String(), "can only receive") {
		t.Errorf("the output does not say what that server can and cannot do:\n%s", stdout.String())
	}
}

func TestNothingReachesAFleetWithoutReport(t *testing.T) {
	var reached int
	fleetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		_, _ = w.Write([]byte(`{}`))
	}))
	defer fleetServer.Close()

	t.Setenv("SUPPORTONE_FLEET_TOKEN", "a-real-token-long-enough-to-use")

	// The server and name are given but --report is not, so nothing is
	// offered and nothing is contacted.
	var stdout, stderr bytes.Buffer
	args := []string{
		"--text",
		"--fleet-server", fleetServer.URL,
		"--fleet-name", "Reception PC",
		"--lang", "en", "--no-explain",
		"--audit-log", filepath.Join(t.TempDir(), "audit.log"),
	}
	if err := run(args, strings.NewReader("send\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if reached != 0 {
		t.Errorf("the server was contacted %d times without --report", reached)
	}
}

func TestReportingWithNoTokenSaysSoBeforeAskingAnything(t *testing.T) {
	fleetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer fleetServer.Close()

	t.Setenv("SUPPORTONE_FLEET_TOKEN", "")

	var stdout, stderr bytes.Buffer
	args := []string{
		"--text", "--report",
		"--fleet-server", fleetServer.URL,
		"--fleet-name", "Reception PC",
		"--lang", "en", "--no-explain",
		"--audit-log", filepath.Join(t.TempDir(), "audit.log"),
	}

	err := run(args, strings.NewReader("send\n"), &stdout, &stderr)
	if err == nil {
		t.Fatal("run proceeded with no fleet token set")
	}
	if !strings.Contains(err.Error(), "SUPPORTONE_FLEET_TOKEN") {
		t.Errorf("the error does not say what to set: %v", err)
	}
	// And it said so before showing a payload and taking a decision on it.
	if strings.Contains(stdout.String(), "This is exactly what would be sent:") {
		t.Error("a payload was shown before the missing credential was noticed")
	}
}

// TestTheMonthlyReportGeneratesEndToEnd is the phase gate, as a test.
func TestTheMonthlyReportGeneratesEndToEnd(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "client-reports")

	var stdout, stderr bytes.Buffer
	args := []string{"--monthly", dir, "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the report folder: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("the folder holds %d files, want an HTML and a JSON report", len(entries))
	}

	var html, jsonPath string
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e.Name(), ".html"):
			html = filepath.Join(dir, e.Name())
		case strings.HasSuffix(e.Name(), ".json"):
			jsonPath = filepath.Join(dir, e.Name())
		}
	}
	if html == "" || jsonPath == "" {
		t.Fatalf("the folder holds %v, want one of each", entries)
	}

	rendered, err := os.ReadFile(html)
	if err != nil {
		t.Fatalf("read the HTML report: %v", err)
	}
	// A client report read weeks later still says what its findings mean,
	// and opens with no network.
	if !strings.Contains(string(rendered), "What this means:") {
		t.Error("the monthly report carries no explanation")
	}
	for _, forbidden := range []string{"http://", "https://", "<script"} {
		if strings.Contains(string(rendered), forbidden) {
			t.Errorf("the report contains %q, so opening it would reach outside the file", forbidden)
		}
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read the JSON report: %v", err)
	}
	var snap checks.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("the JSON report is not a snapshot: %v", err)
	}
	if len(snap.Results) == 0 {
		t.Error("the JSON report holds no results")
	}

	// It runs unattended, so it redacts fully without asking.
	if !bytes.Contains(raw, []byte(redact.Marker)) {
		t.Error("nothing in the unattended report was redacted")
	}
	if !strings.Contains(stdout.String(), "sent nowhere") {
		t.Errorf("the output does not say the report went nowhere:\n%s", stdout.String())
	}
}

func TestTheSchedulerEntryIsPrintedAndNotInstalled(t *testing.T) {
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	args := []string{"--schedule", dir, "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "--monthly") {
		t.Errorf("the entry does not run the monthly report:\n%s", out)
	}
	// The undo is printed with the command, not left to be looked up later.
	if !strings.Contains(out, "To stop it later:") {
		t.Errorf("the entry does not say how to remove it:\n%s", out)
	}
	if !strings.Contains(out, "has not installed anything") {
		t.Errorf("the output does not say nothing was installed:\n%s", out)
	}

	// And nothing was in fact installed: no report was written either.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("--schedule wrote %v", entries)
	}
}

func TestMonthlyAndScheduleAreNotAskedForTogether(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--monthly", "/tmp/a", "--schedule", "/tmp/b"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("run accepted --monthly and --schedule together")
	}
}

func TestARerunReplacesThatMonthsReport(t *testing.T) {
	dir := t.TempDir()
	audit := filepath.Join(t.TempDir(), "audit.log")

	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		args := []string{"--monthly", dir, "--lang", "en", "--audit-log", audit}
		if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	// A scheduler runs this every month for years. It must not accumulate a
	// new pair of files every time it runs within one month.
	if len(entries) != 2 {
		t.Errorf("the folder holds %d files after two runs in one month, want 2", len(entries))
	}
}

func writeProfile(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return path
}

func TestAProfileIsMeasuredAndReported(t *testing.T) {
	path := writeProfile(t, `{
	  "schema": 1,
	  "name": "Front desk laptops",
	  "expectations": [
	    {"check": "os.info", "worst": "attention", "why": "Support only covers supported releases."},
	    {"check": "disk.volumes", "worst": "attention"}
	  ]
	}`)

	var stdout, stderr bytes.Buffer
	args := []string{"--profile", path, "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	err := run(args, strings.NewReader(""), &stdout, &stderr)

	out := stdout.String()
	if !strings.Contains(out, "Front desk laptops") {
		t.Errorf("the report does not name the profile:\n%s", out)
	}
	if !strings.Contains(out, "os.info") || !strings.Contains(out, "disk.volumes") {
		t.Errorf("the report does not cover every expectation:\n%s", out)
	}
	// The technician's own note travels with the rule it explains.
	if !strings.Contains(out, "Support only covers supported releases.") {
		t.Errorf("the report drops the reason the rule exists:\n%s", out)
	}
	if !strings.Contains(out, "met") {
		t.Errorf("the report has no verdicts:\n%s", out)
	}

	// The exit is a summary of what was printed, either way.
	if err != nil && !errors.Is(err, errNotConforming) {
		t.Fatalf("run: %v", err)
	}
	if conforms := !strings.Contains(out, "does not meet the profile"); conforms != (err == nil) {
		t.Errorf("the printed verdict and the exit status disagree: err = %v\n%s", err, out)
	}
}

func TestAProfileNamingACheckThisBuildDoesNotCarryIsNotSilentlyPassed(t *testing.T) {
	path := writeProfile(t, `{
	  "schema": 1,
	  "name": "Wishful",
	  "expectations": [{"check": "check.that.does.not.exist", "worst": "ok"}]
	}`)

	var stdout, stderr bytes.Buffer
	args := []string{"--profile", path, "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	err := run(args, strings.NewReader(""), &stdout, &stderr)

	if !errors.Is(err, errNotConforming) {
		t.Fatalf("run = %v, want it to report non-conformance", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "not here") {
		t.Errorf("the missing check is not reported as missing:\n%s", out)
	}
	if !strings.Contains(out, "0 met") {
		t.Errorf("a check this build does not carry was counted as met:\n%s", out)
	}
}

func TestAProfileThisBuildCannotActOnHonestlyIsRefused(t *testing.T) {
	cases := map[string]string{
		"a schema from another version": `{"schema": 99, "name": "x", "expectations": [{"check": "os.info", "worst": "ok"}]}`,
		"no expectations at all":        `{"schema": 1, "name": "x", "expectations": []}`,
		"a misspelled field":            `{"schema": 1, "name": "x", "expectatiosn": []}`,
		"two rules for one check": `{"schema": 1, "name": "x", "expectations": [
			{"check": "os.info", "worst": "ok"}, {"check": "os.info", "worst": "urgent"}]}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"--profile", writeProfile(t, body), "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
			if err := run(args, strings.NewReader(""), &stdout, &stderr); err == nil {
				t.Fatalf("run accepted the profile and printed:\n%s", stdout.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("a refused profile still produced a report:\n%s", stdout.String())
			}
		})
	}
}

func TestAProfileCanBeReadAsJSON(t *testing.T) {
	path := writeProfile(t, `{"schema": 1, "name": "Fleet", "expectations": [{"check": "os.info", "worst": "urgent"}]}`)

	var stdout, stderr bytes.Buffer
	args := []string{"--profile", path, "--json", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil && !errors.Is(err, errNotConforming) {
		t.Fatalf("run: %v", err)
	}

	var measured profile.Report
	if err := json.Unmarshal(stdout.Bytes(), &measured); err != nil {
		t.Fatalf("the output is not a profile report: %v\n%s", err, stdout.String())
	}
	if measured.Profile != "Fleet" || len(measured.Findings) != 1 {
		t.Errorf("report = %+v, want one finding for Fleet", measured)
	}
}

func TestAProfileOffersOnlyRepairsThisBuildCarries(t *testing.T) {
	// A check no build carries is always missing, whatever platform this runs
	// on, which is what makes the offer path deterministic here.
	path := writeProfile(t, `{"schema": 1, "name": "x", "expectations": [
		{"check": "check.not.in.this.build", "worst": "ok",
		 "offer": ["temp.clear", "format.the.disk"]}]}`)
	audit := filepath.Join(t.TempDir(), "audit.log")

	var stdout, stderr bytes.Buffer
	args := []string{"--profile", path, "--lang", "en", "--audit-log", audit}
	if err := run(args, strings.NewReader("yes\ntemp.clear\n"), &stdout, &stderr); !errors.Is(err, errNotConforming) {
		t.Fatalf("run = %v, want it to report non-conformance", err)
	}

	out := stdout.String()
	// A profile is data and can name anything. Only what the registry knows
	// survives to be shown.
	if strings.Contains(out, "format.the.disk") {
		t.Errorf("the report offered a repair this build does not carry:\n%s", out)
	}

	// And offering is where a profile stops: nothing it named may be applied
	// without that fix's own description and its own confirmation.
	if log := readFile(t, audit); strings.Contains(log, string(consent.EventFixApplied)) {
		t.Errorf("measuring a profile applied a fix:\n%s", log)
	}
}

func TestTheRemoteToolListSaysWhatIsHereAndOffersNoDownload(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--list-remote-tools", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "does not install") {
		t.Errorf("the list does not say SupportOne installs none of these:\n%s", out)
	}
	for _, offer := range []string{"http://", "https://", "download", "apt install", "brew install"} {
		if strings.Contains(strings.ToLower(out), offer) {
			t.Errorf("the list offers to fetch something (%q):\n%s", offer, out)
		}
	}
}

func TestARemoteSessionNamesEveryConsequenceBeforeAskingAnything(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--remote", "Aisyah from IT", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	// Pressing Enter at the prompt.
	if err := run(args, strings.NewReader("\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Aisyah from IT") {
		t.Errorf("the prompt does not name who is being let in:\n%s", out)
	}
	for _, must := range []string{"see your screen", "read any file", "as you", "cannot watch"} {
		if !strings.Contains(out, must) {
			t.Errorf("the consequences do not mention %q:\n%s", must, out)
		}
	}
	if !strings.Contains(out, "nobody was let in") {
		t.Errorf("declining was not confirmed:\n%s", out)
	}
}

func TestARemoteSessionIsNotStartedWithoutTheWordAllow(t *testing.T) {
	audit := filepath.Join(t.TempDir(), "audit.log")

	for _, answer := range []string{"\n", "yes\n", "ok\n", "y\n", ""} {
		var stdout, stderr bytes.Buffer
		args := []string{"--remote", "Aisyah", "--lang", "en", "--audit-log", audit}
		if err := run(args, strings.NewReader(answer), &stdout, &stderr); err != nil {
			t.Fatalf("run with %q: %v", answer, err)
		}
		if !strings.Contains(stdout.String(), "nobody was let in") {
			t.Errorf("%q started a session:\n%s", answer, stdout.String())
		}
	}

	if log := readFile(t, audit); strings.Contains(log, string(consent.EventRemoteStarted)) {
		t.Errorf("a session was recorded as started:\n%s", log)
	}
}

func TestARemoteSessionIsRecordedFromAgreementToEnd(t *testing.T) {
	audit := filepath.Join(t.TempDir(), "audit.log")

	var stdout, stderr bytes.Buffer
	args := []string{"--remote", "Aisyah", "--lang", "en", "--audit-log", audit}
	// Agree, then say the session is over.
	if err := run(args, strings.NewReader("allow\n\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	// No tool was named, so SupportOne starts nothing and says so.
	if !strings.Contains(out, "Open the remote-help program now") {
		t.Errorf("the user was not told to open the tool themselves:\n%s", out)
	}
	if !strings.Contains(out, "not part of what happens next") {
		t.Errorf("the honest limit was not stated:\n%s", out)
	}
	if !strings.Contains(out, "recorded as lasting") {
		t.Errorf("the session was not reported as ended:\n%s", out)
	}

	log := readFile(t, audit)
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

func TestADryRunAgreesToNoRemoteSession(t *testing.T) {
	audit := filepath.Join(t.TempDir(), "audit.log")

	var stdout, stderr bytes.Buffer
	args := []string{"--remote", "Aisyah", "--dry-run", "--lang", "en", "--audit-log", audit}
	if err := run(args, strings.NewReader("allow\n\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(stdout.String(), "nothing on this computer will be changed") {
		t.Errorf("the dry run does not say it changed nothing:\n%s", stdout.String())
	}
	if log := readFile(t, audit); strings.Contains(log, string(consent.EventRemoteStarted)) {
		t.Errorf("a dry run started a session:\n%s", log)
	}
}

func TestAToolThisBuildDoesNotKnowIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"--remote", "Aisyah", "--remote-tool", "something-from-a-forum", "--lang", "en", "--audit-log", filepath.Join(t.TempDir(), "audit.log")}
	if err := run(args, strings.NewReader("allow\n"), &stdout, &stderr); err == nil {
		t.Fatal("run accepted a tool that is not in the whitelist")
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused tool still printed a consent prompt:\n%s", stdout.String())
	}
}

func TestRemoteToolOnlyMeansSomethingWithRemote(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--remote-tool", "rustdesk"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Error("run accepted --remote-tool with nobody named")
	}
}

func TestDecliningARemoteSessionIsRecordedAsAnAnswer(t *testing.T) {
	audit := filepath.Join(t.TempDir(), "audit.log")

	var stdout, stderr bytes.Buffer
	args := []string{"--remote", "Aisyah", "--lang", "en", "--audit-log", audit}
	if err := run(args, strings.NewReader("\n"), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}

	// A log showing a question asked and then nothing is a worse record than
	// one that says the answer was no.
	log := readFile(t, audit)
	if !strings.Contains(log, string(consent.EventConsentDenied)) {
		t.Errorf("declining was not recorded:\n%s", log)
	}
	if strings.Contains(log, string(consent.EventRemoteStarted)) {
		t.Errorf("a declined session was recorded as started:\n%s", log)
	}
}
