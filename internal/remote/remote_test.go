package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// fixture builds a wrapper whose view of the machine is entirely under the
// test's control: nothing here touches a real remote-help program.
type fixture struct {
	wrapper *Wrapper
	audit   string
	started []string
	failOn  string
}

func newFixture(t *testing.T, os platform.OS, installed ...string) *fixture {
	t.Helper()

	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := consent.Open(path)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	present := make(map[string]bool, len(installed))
	for _, name := range installed {
		present[name] = true
	}

	f := &fixture{audit: path}
	f.wrapper = New(log, os)
	f.wrapper.lookPath = func(candidate string) (string, error) {
		if present[candidate] {
			return "/resolved/" + filepath.Base(candidate), nil
		}
		return "", errNoSuchTool
	}
	f.wrapper.start = func(_ context.Context, p string) error {
		if p == f.failOn {
			return errors.New("the program would not start")
		}
		f.started = append(f.started, p)
		return nil
	}
	f.wrapper.now = func() time.Time { return time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC) }
	return f
}

func (f *fixture) log(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.audit) // #nosec G304 -- a path this test just created
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	return string(data)
}

// agree returns a confirmation that repeats exactly what the plan showed.
func agree(p Plan) Confirmation {
	return Confirmation{Token: p.Token, Acknowledged: append([]string(nil), p.Consequences...)}
}

func TestEveryConsequenceIsShown(t *testing.T) {
	got := Consequences()

	want := []string{KeyCanSeeScreen, KeyCanControl, KeyCanReadFiles, KeyCanActAsYou, KeyCannotWatch, KeyEndIt}
	if len(got) != len(want) {
		t.Fatalf("Consequences() = %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Consequences()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// The blunt ones are the point of the list. If a future edit softens the
	// wording it can, but it may not drop the facts.
	for _, key := range []string{KeyCanReadFiles, KeyCanActAsYou, KeyCannotWatch} {
		if !contains(got, key) {
			t.Errorf("Consequences() no longer mentions %q", key)
		}
	}
}

func TestConsequencesCannotBeMutatedByACaller(t *testing.T) {
	first := Consequences()
	first[0] = "remote.tampered"

	if Consequences()[0] != KeyCanSeeScreen {
		t.Fatal("a caller editing the returned slice changed what the next user is shown")
	}
}

func TestToolsReportsWhatIsThereAndWhatIsNot(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	tools := f.wrapper.Tools()
	if len(tools) != len(knownTools[platform.Linux]) {
		t.Fatalf("Tools() = %d, want %d", len(tools), len(knownTools[platform.Linux]))
	}

	var found, missing int
	for _, tool := range tools {
		if tool.Installed {
			found++
			if tool.ID != "rustdesk" {
				t.Errorf("reported %s as installed; only rustdesk was", tool.ID)
			}
			if tool.Path == "" {
				t.Errorf("%s is installed but has no path to show", tool.ID)
			}
			continue
		}
		missing++
		if tool.Path != "" {
			t.Errorf("%s is not installed but reports path %q", tool.ID, tool.Path)
		}
	}
	if found != 1 {
		t.Errorf("found %d installed tools, want 1", found)
	}
	if missing == 0 {
		t.Error("no tool was reported as missing; a build that finds everything is not looking")
	}
}

func TestAnUnknownOSOffersNothingRatherThanGuessing(t *testing.T) {
	f := newFixture(t, platform.OS("plan9"))

	if tools := f.wrapper.Tools(); len(tools) != 0 {
		t.Fatalf("Tools() on an unknown OS = %v, want none", tools)
	}
}

func TestASessionMustNameWhoIsBeingLetIn(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	for _, name := range []string{"", "   ", "\t\n"} {
		if _, err := f.wrapper.Plan(name, "rustdesk"); !errors.Is(err, ErrNoTechnician) {
			t.Errorf("Plan(%q) error = %v, want ErrNoTechnician", name, err)
		}
	}
}

func TestPlanRefusesAToolThisBuildDoesNotKnow(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	_, err := f.wrapper.Plan("Aisyah", "some-tool-from-a-forum-post")
	if !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("Plan error = %v, want ErrUnknownTool", err)
	}
}

func TestPlanSaysSoRatherThanOfferingToInstall(t *testing.T) {
	f := newFixture(t, platform.Linux) // nothing installed

	_, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if !errors.Is(err, ErrToolNotInstalled) {
		t.Fatalf("Plan error = %v, want ErrToolNotInstalled", err)
	}
	if errors.Is(err, ErrUnknownTool) {
		t.Error("a tool this build knows was reported as unknown")
	}
}

func TestAPlanShowsTheWholeListAndIsRecordedAsAsked(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	plan, err := f.wrapper.Plan("  Aisyah from IT  ", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Technician != "Aisyah from IT" {
		t.Errorf("Technician = %q, want the trimmed name", plan.Technician)
	}
	if plan.Tool.ID != "rustdesk" || !plan.Tool.Installed {
		t.Errorf("Tool = %+v, want the installed rustdesk", plan.Tool)
	}
	if plan.Token == "" {
		t.Error("the plan has no token, so nothing proves it was shown")
	}
	if len(plan.Consequences) != len(Consequences()) {
		t.Errorf("the plan shows %d consequences, want all %d", len(plan.Consequences), len(Consequences()))
	}

	entry := f.log(t)
	if !strings.Contains(entry, string(consent.EventConsentAsked)) {
		t.Errorf("the audit log does not record that consent was asked:\n%s", entry)
	}
	if !strings.Contains(entry, "Aisyah from IT") {
		t.Errorf("the audit log does not name the technician:\n%s", entry)
	}
}

func TestASessionWithNoToolStillGetsTheConsentRecord(t *testing.T) {
	f := newFixture(t, platform.Linux)

	plan, err := f.wrapper.Plan("Aisyah", "")
	if err != nil {
		t.Fatalf("Plan with no tool: %v", err)
	}
	if plan.Tool.ID != "" {
		t.Errorf("Tool = %+v, want empty", plan.Tool)
	}

	session, err := f.wrapper.Start(t.Context(), agree(plan))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if session.Launched {
		t.Error("Launched is true, but there was no tool to launch")
	}
	if len(f.started) != 0 {
		t.Errorf("started %v with no tool named", f.started)
	}
}

func TestStartRefusesWithoutTheTokenFromThePlan(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	confirmation := agree(plan)
	confirmation.Token = "a-token-nobody-issued"

	if _, err := f.wrapper.Start(t.Context(), confirmation); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("Start error = %v, want ErrNotConfirmed", err)
	}
	if len(f.started) != 0 {
		t.Errorf("a tool was started without a confirmed plan: %v", f.started)
	}
}

func TestStartRefusesAnAcknowledgementThatDoesNotMatchWhatWasShown(t *testing.T) {
	cases := map[string]func(Plan) []string{
		"nothing echoed back": func(Plan) []string { return nil },
		"a short list": func(p Plan) []string {
			return p.Consequences[:len(p.Consequences)-1]
		},
		"the awkward one dropped": func(p Plan) []string {
			out := []string{}
			for _, key := range p.Consequences {
				if key == KeyCanReadFiles {
					continue
				}
				out = append(out, key)
			}
			return append(out, "remote.can.do_something_harmless")
		},
		"reordered": func(p Plan) []string {
			out := append([]string(nil), p.Consequences...)
			out[0], out[1] = out[1], out[0]
			return out
		},
	}

	for name, mangle := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, platform.Linux, "rustdesk")
			plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}

			_, err = f.wrapper.Start(t.Context(), Confirmation{Token: plan.Token, Acknowledged: mangle(plan)})
			if !errors.Is(err, ErrNotConfirmed) {
				t.Fatalf("Start error = %v, want ErrNotConfirmed", err)
			}
			if len(f.started) != 0 {
				t.Errorf("a tool was started anyway: %v", f.started)
			}
			if entry := f.log(t); !strings.Contains(entry, string(consent.EventConsentDenied)) {
				t.Errorf("the refusal was not recorded:\n%s", entry)
			}
		})
	}
}

func TestAPlanIsGoodForOneSessionOnly(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.wrapper.Start(t.Context(), agree(plan)); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if _, err := f.wrapper.End(); err != nil {
		t.Fatalf("End: %v", err)
	}

	if _, err := f.wrapper.Start(t.Context(), agree(plan)); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("second Start with the same token = %v, want ErrNotConfirmed", err)
	}
	if len(f.started) != 1 {
		t.Errorf("the tool was started %d times from one confirmation", len(f.started))
	}
}

func TestAConfirmedSessionStartsTheToolAndIsRecorded(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	session, err := f.wrapper.Start(t.Context(), agree(plan))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !session.Launched {
		t.Error("Launched is false, but the tool was installed and started")
	}
	if session.ID == "" {
		t.Error("the session has no ID")
	}
	if session.Technician != "Aisyah" || session.Tool != "rustdesk" {
		t.Errorf("session = %+v, want the technician and tool from the plan", session)
	}
	if !session.Ended.IsZero() {
		t.Error("a session that just started is already recorded as ended")
	}
	if len(f.started) != 1 || f.started[0] != plan.Tool.Path {
		t.Errorf("started %v, want exactly the path the plan showed (%q)", f.started, plan.Tool.Path)
	}

	entry := f.log(t)
	for _, want := range []string{string(consent.EventConsentGiven), string(consent.EventRemoteStarted), "Aisyah", "rustdesk"} {
		if !strings.Contains(entry, want) {
			t.Errorf("the audit log does not mention %q:\n%s", want, entry)
		}
	}
}

func TestAToolThatWillNotStartDoesNotUndoTheConsent(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	f.failOn = plan.Tool.Path

	session, err := f.wrapper.Start(t.Context(), agree(plan))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if session.Launched {
		t.Error("Launched is true, but the program did not start")
	}
	if _, open := f.wrapper.Current(); !open {
		t.Error("the session was dropped; the user may still start the tool themselves")
	}

	entry := f.log(t)
	if !strings.Contains(entry, "the program could not be started") {
		t.Errorf("the audit log does not say the launch failed:\n%s", entry)
	}
	if strings.Count(entry, string(consent.EventRemoteStarted)) != 1 {
		t.Errorf("one session produced more than one REMOTE_STARTED line:\n%s", entry)
	}
}

func TestASecondSessionCannotOpenWhileOneIsRunning(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	first, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.wrapper.Start(t.Context(), agree(first)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	second, err := f.wrapper.Plan("Someone Else", "rustdesk")
	if err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	_, err = f.wrapper.Start(t.Context(), agree(second))
	if err == nil {
		t.Fatal("a second session opened while the first was still running")
	}
	if !strings.Contains(err.Error(), "Aisyah") {
		t.Errorf("the refusal does not say who is already in: %v", err)
	}
	if len(f.started) != 1 {
		t.Errorf("started %v, want only the first session's tool", f.started)
	}
}

func TestEndClosesTheRecordAndSaysHowLongItWasAgreedFor(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.wrapper.Start(t.Context(), agree(plan)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	start := f.wrapper.now()
	f.wrapper.now = func() time.Time { return start.Add(23 * time.Minute) }

	session, err := f.wrapper.End()
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	if session.Ended.IsZero() {
		t.Fatal("the session is still open after End")
	}
	if got := session.Duration(); got != 23*time.Minute {
		t.Errorf("Duration() = %v, want 23m", got)
	}
	if _, open := f.wrapper.Current(); open {
		t.Error("Current() still reports an open session after End")
	}

	if entry := f.log(t); !strings.Contains(entry, string(consent.EventRemoteEnded)) || !strings.Contains(entry, "23m0s") {
		t.Errorf("the audit log does not record the end and its length:\n%s", entry)
	}
}

func TestEndWithNoSessionIsAnError(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	if _, err := f.wrapper.End(); !errors.Is(err, ErrNoSession) {
		t.Fatalf("End with nothing running = %v, want ErrNoSession", err)
	}

	plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.wrapper.Start(t.Context(), agree(plan)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := f.wrapper.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if _, err := f.wrapper.End(); !errors.Is(err, ErrNoSession) {
		t.Fatalf("second End = %v, want ErrNoSession", err)
	}
}

func TestAnEndedSessionDurationIsZeroWhileOpen(t *testing.T) {
	if got := (Session{Started: time.Now()}).Duration(); got != 0 {
		t.Errorf("Duration() of an open session = %v, want 0", got)
	}
}

func TestAWrapperWithNoAuditLogStillWorks(t *testing.T) {
	// The agent always has a log. A caller constructing one without a log
	// should not panic on the path that would have written to it.
	w := New(nil, platform.Linux)
	w.lookPath = func(string) (string, error) { return "", errNoSuchTool }

	plan, err := w.Plan("Aisyah", "")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := w.Start(t.Context(), agree(plan)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := w.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
}

func TestTokensAreNotReused(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	seen := make(map[string]bool)
	for range 50 {
		plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if seen[plan.Token] {
			t.Fatalf("token %q was issued twice", plan.Token)
		}
		seen[plan.Token] = true
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestShowingANewPlanSupersedesTheOldOne(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	first, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("first Plan: %v", err)
	}
	second, err := f.wrapper.Plan("Someone Else", "rustdesk")
	if err != nil {
		t.Fatalf("second Plan: %v", err)
	}

	// Only the plan the user is actually looking at can be confirmed. A stale
	// one, shown and then replaced, cannot be started behind it.
	if _, err := f.wrapper.Start(t.Context(), agree(first)); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("Start on the superseded plan = %v, want ErrNotConfirmed", err)
	}
	if len(f.started) != 0 {
		t.Errorf("a superseded plan started %v", f.started)
	}

	if _, err := f.wrapper.Start(t.Context(), agree(second)); err != nil {
		t.Fatalf("Start on the current plan: %v", err)
	}
}

func TestDecliningIsRecordedAsAnAnswer(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	f.wrapper.Decline()

	entry := f.log(t)
	if !strings.Contains(entry, string(consent.EventConsentDenied)) {
		t.Errorf("declining was not recorded:\n%s", entry)
	}
	if !strings.Contains(entry, "Aisyah") {
		t.Errorf("the refusal does not name who was refused:\n%s", entry)
	}

	// And the plan is gone, so a later confirmation cannot resurrect it.
	if _, err := f.wrapper.Start(t.Context(), agree(plan)); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("Start after Decline = %v, want ErrNotConfirmed", err)
	}
	if len(f.started) != 0 {
		t.Errorf("a declined plan started %v", f.started)
	}
}

func TestDecliningNothingRecordsNothing(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	f.wrapper.Decline()

	if entry := f.log(t); strings.Contains(entry, string(consent.EventConsentDenied)) {
		t.Errorf("a refusal was recorded with nothing to refuse:\n%s", entry)
	}
}

func TestAnOpenSessionCarriesNoEndTime(t *testing.T) {
	f := newFixture(t, platform.Linux, "rustdesk")

	plan, err := f.wrapper.Plan("Aisyah", "rustdesk")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	session, err := f.wrapper.Start(t.Context(), agree(plan))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// A zero time rendered as 0001-01-01 reads like an end time to anything
	// that checks the field rather than its value.
	if bytes.Contains(raw, []byte("0001-01-01")) {
		t.Errorf("an open session reports an end time:\n%s", raw)
	}
	if bytes.Contains(raw, []byte(`"ended"`)) {
		t.Errorf("an open session carries an ended field:\n%s", raw)
	}

	ended, err := f.wrapper.End()
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	if raw, err = json.Marshal(ended); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"ended"`)) {
		t.Errorf("an ended session does not report when:\n%s", raw)
	}

	// And it survives the round trip the interface makes.
	var back Session
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.Ended.Equal(ended.Ended) || back.ID != ended.ID {
		t.Errorf("round trip lost the session: %+v, want %+v", back, ended)
	}
}
