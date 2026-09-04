package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
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
)

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "supportone-agent") {
		t.Errorf("stdout = %q, want build information", stdout.String())
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
	// The terminal path always redacts, and what went out proves it.
	if strings.Contains(received, redactableHostname(t)) {
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

// redactableHostname returns this machine's hostname, which full redaction
// must remove from anything that leaves.
func redactableHostname(t *testing.T) string {
	t.Helper()

	name, err := os.Hostname()
	if err != nil || name == "" {
		t.Skip("this machine reports no hostname, so there is nothing to assert was removed")
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

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	// A bundle is built to be handed to someone else, so the terminal path
	// redacts fully rather than asking field by field.
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		if bytes.Contains(raw, []byte(hostname)) {
			t.Error("the bundle carries this machine's hostname")
		}
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
