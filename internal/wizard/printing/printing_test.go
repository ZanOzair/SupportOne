package printing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/fixes/spooler"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

// stubRunner answers each command from a table, so the whole wizard can be
// exercised without a Windows machine.
func stubRunner(out map[string]string, fail map[string]bool) platform.Runner {
	return func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if fail[name] {
			return nil, errors.New(name + " could not be run")
		}
		return []byte(out[name]), nil
	}
}

const running = "SERVICE_NAME: spooler\n        STATE              : 4  RUNNING\n"
const stopped = "SERVICE_NAME: spooler\n        STATE              : 1  STOPPED\n"

func TestServiceStepReadsWhetherPrintingIsPossibleAtAll(t *testing.T) {
	cases := map[string]struct {
		out        string
		fail       bool
		wantOK     bool
		wantSummry string
		unknown    bool
	}{
		"running":    {running, false, true, KeyServiceRunning, false},
		"stopped":    {stopped, false, false, KeyServiceStopped, false},
		"unreadable": {"", true, false, KeyServiceUnreadable, true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			run := stubRunner(map[string]string{scExe: tc.out}, map[string]bool{scExe: tc.fail})

			got, err := serviceProbe(run)(context.Background())
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			if got.OK != tc.wantOK {
				t.Errorf("OK = %v, want %v", got.OK, tc.wantOK)
			}
			if got.Unknown != tc.unknown {
				t.Errorf("Unknown = %v, want %v", got.Unknown, tc.unknown)
			}
			if got.Summary != tc.wantSummry {
				t.Errorf("Summary = %q, want %q", got.Summary, tc.wantSummry)
			}
		})
	}
}

func TestQueueStepCountsWhatIsWaiting(t *testing.T) {
	dir := t.TempDir()

	got, err := queueProbe(dir)(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !got.OK || got.Summary != KeyQueueEmpty {
		t.Errorf("an empty queue gave %+v, want a clean answer", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "00001.SPL"), nil, 0o600); err != nil {
		t.Fatalf("queue a job: %v", err)
	}
	got, err = queueProbe(dir)(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	// One document, so the singular: "1 documents are waiting" is the kind of
	// detail that tells a user nobody read this screen.
	if got.OK || got.Summary != KeyQueueStuckOne {
		t.Errorf("one queued job gave %+v, want the singular summary", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "00002.SPL"), nil, 0o600); err != nil {
		t.Fatalf("queue a job: %v", err)
	}
	got, err = queueProbe(dir)(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got.OK || got.Summary != KeyQueueStuck {
		t.Errorf("two queued jobs gave %+v, want the plural summary", got)
	}
	if len(got.Args) != 1 || got.Args[0] != 2 {
		t.Errorf("Args = %v, want the count", got.Args)
	}
}

func TestTheAgentSOwnHoldingFolderIsNotCountedAsAPrintJob(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "SupportOne-quarantine-20260903"), 0o700); err != nil {
		t.Fatalf("create holding folder: %v", err)
	}

	got, err := queueProbe(dir)(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	// Otherwise clearing the queue would make the wizard report the queue as
	// still jammed, by its own leftovers.
	if !got.OK {
		t.Errorf("the holding folder was counted as a queued job: %+v", got)
	}
}

func TestAnUnreadableQueueIsNotAnEmptyQueue(t *testing.T) {
	got, err := queueProbe(filepath.Join(t.TempDir(), "not-there"))(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got.OK {
		t.Error("a queue that could not be read was reported as empty")
	}
	if !got.Unknown || got.Summary != KeyQueueUnreadable {
		t.Errorf("got %+v, want it reported as unanswered", got)
	}
}

func TestPrinterStepReadsWhetherADefaultIsSet(t *testing.T) {
	cases := map[string]struct {
		out     string
		fail    bool
		wantOK  bool
		summary string
	}{
		"a default is set": {`{"Name":"HP LaserJet"}`, false, true, KeyPrinterSet},
		"none is set":      {"", false, false, KeyPrinterNone},
		"unreadable":       {"", true, false, KeyPrinterUnreadable},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			run := stubRunner(map[string]string{psExe: tc.out}, map[string]bool{psExe: tc.fail})

			got, err := printerProbe(run)(context.Background())
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			if got.OK != tc.wantOK {
				t.Errorf("OK = %v, want %v", got.OK, tc.wantOK)
			}
			if got.Summary != tc.summary {
				t.Errorf("Summary = %q, want %q", got.Summary, tc.summary)
			}
		})
	}
}

func TestTheWizardIsShapedSoItCanBeRegistered(t *testing.T) {
	w := New(stubRunner(nil, nil), t.TempDir())

	registry := wizard.NewRegistry()
	if err := registry.Register(w); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if !w.RunsOn(platform.Windows) {
		t.Error("the wizard does not declare Windows, the only platform it reads")
	}
	if w.RunsOn(platform.Linux) || w.RunsOn(platform.Darwin) {
		t.Error("the wizard claims a CUPS platform, whose print service it does not read")
	}

	// The service question comes first: a stopped spooler explains every other
	// answer, and the queue cannot be cleared while it is down.
	if w.Steps[0].ID != "printing.service" {
		t.Errorf("first step is %q, want the print service", w.Steps[0].ID)
	}
	if w.Steps[1].FixID != spooler.ID {
		t.Errorf("the queue step offers %q, want %q", w.Steps[1].FixID, spooler.ID)
	}
	for _, step := range w.Steps {
		if step.FixID == "" && step.Advice == "" {
			t.Errorf("step %q offers neither a fix nor advice", step.ID)
		}
	}
}

func TestTheWholeWizardRunsAgainstAHealthyMachine(t *testing.T) {
	run := stubRunner(map[string]string{
		scExe: running,
		psExe: `{"Name":"HP LaserJet"}`,
	}, nil)

	s := wizard.Start(New(run, t.TempDir()), nil, 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	// Nothing this wizard knows how to check is wrong, and it says exactly
	// that rather than inventing something to repair.
	if got.Outcome != wizard.OutcomeNoFault {
		t.Errorf("Outcome = %q, want %q", got.Outcome, wizard.OutcomeNoFault)
	}
	if len(got.Done) != 3 {
		t.Errorf("asked %d questions, want all three", len(got.Done))
	}
}

func TestAStoppedServiceStopsTheWizardWithAdviceRatherThanARepair(t *testing.T) {
	run := stubRunner(map[string]string{scExe: stopped, psExe: `{"Name":"HP"}`}, nil)

	s := wizard.Start(New(run, t.TempDir()), nil, 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	if got.Step == nil || got.Step.StepID != "printing.service" {
		t.Fatalf("Step = %+v, want the print service step", got.Step)
	}
	// SupportOne does not start services for people; it tells them how.
	if got.Offer != nil {
		t.Error("a repair was offered for something the agent does not do")
	}
	if got.Advice != KeyServiceAdvice {
		t.Errorf("Advice = %q, want %q", got.Advice, KeyServiceAdvice)
	}
}
