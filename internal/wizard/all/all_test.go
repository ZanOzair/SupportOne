package all

import (
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/wizard"
	"github.com/ZanOzair/SupportOne/internal/wizard/connection"
	"github.com/ZanOzair/SupportOne/internal/wizard/printing"

	_ "github.com/ZanOzair/SupportOne/internal/fixes/all"
)

// wantIDs is the full set of Phase 2 wizards, written out so that adding or
// removing one is a deliberate edit here.
var wantIDs = []string{
	connection.ID,
	printing.ID,
}

func TestEveryWizardIsRegistered(t *testing.T) {
	registered := wizard.Default.All()
	if len(registered) != len(wantIDs) {
		t.Fatalf("registered %d wizards, want %d: %v", len(registered), len(wantIDs), ids(registered))
	}
	for i, want := range wantIDs {
		if registered[i].ID != want {
			t.Errorf("wizard %d = %q, want %q", i, registered[i].ID, want)
		}
	}
}

// TestEveryStepEndsSomewhereUseful is the rule that keeps a wizard from asking
// a question, finding a problem, and leaving the user holding it.
func TestEveryStepEndsSomewhereUseful(t *testing.T) {
	for _, w := range wizard.Default.All() {
		for _, step := range w.Steps {
			if step.FixID == "" && step.Advice == "" {
				t.Errorf("%s: step %q offers neither a fix nor advice", w.ID, step.ID)
			}
		}
	}
}

// TestEveryNamedFixIsCompiledIn guards against a wizard offering a repair that
// does not exist — the user would be shown a button that cannot work.
func TestEveryNamedFixIsCompiledIn(t *testing.T) {
	for _, w := range wizard.Default.All() {
		for _, step := range w.Steps {
			if step.FixID == "" {
				continue
			}
			fix, ok := fixes.Default.Get(step.FixID)
			if !ok {
				t.Errorf("%s: step %q names fix %q, which is not compiled in", w.ID, step.ID, step.FixID)
				continue
			}
			// And it must run somewhere the wizard runs, or the offer can
			// never appear.
			var reachable bool
			for _, os := range w.Platforms {
				if fixes.RunsOn(fix, os) {
					reachable = true
				}
			}
			if !reachable {
				t.Errorf("%s: step %q names fix %q, which runs on none of the wizard's platforms", w.ID, step.ID, step.FixID)
			}
		}
	}
}

// TestEveryMessageKeyIsTranslated is the guard against a wizard showing the
// user a bare message key.
func TestEveryMessageKeyIsTranslated(t *testing.T) {
	bundle, err := i18n.Load(i18n.Base)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	var keys []string
	for _, w := range wizard.Default.All() {
		keys = append(keys, w.Title, w.Complaint)
		for _, step := range w.Steps {
			keys = append(keys, step.Title)
			if step.Advice != "" {
				keys = append(keys, step.Advice)
			}
		}
	}
	keys = append(keys, engineKeys...)
	keys = append(keys, findingKeys...)
	keys = append(keys, interfaceKeys...)

	for _, key := range keys {
		if got := bundle.T(key); got == key {
			t.Errorf("message key %q has no translation", key)
		}
	}
}

// engineKeys are the keys the wizard engine itself reports with.
var engineKeys = []string{
	wizard.KeyOutcomeFixed, wizard.KeyOutcomeUnresolved, wizard.KeyOutcomeUnverified,
	wizard.KeyOutcomeNoFault, wizard.KeyOutcomeStopped,

	wizard.KeyStepChecking, wizard.KeyStepClean, wizard.KeyStepTried,
	wizard.KeyStepApplied, wizard.KeyStepNoHelp, wizard.KeyStepDeclined, wizard.KeyStepBlocked,
}

// findingKeys are the answers a step can give. They are produced inside the
// probes rather than being reachable from the wizard definition, so they are
// listed.
var findingKeys = []string{
	connection.KeyLinkOK, connection.KeyLinkNone, connection.KeyLinkNoAddress,
	connection.KeyLinkUnreadable,
	connection.KeyCacheNone, connection.KeyCacheStale,

	printing.KeyServiceRunning, printing.KeyServiceStopped, printing.KeyServiceUnreadable,
	printing.KeyQueueEmpty, printing.KeyQueueStuck, printing.KeyQueueStuckOne,
	printing.KeyQueueUnreadable,
	printing.KeyPrinterSet, printing.KeyPrinterNone, printing.KeyPrinterUnreadable,
}

// interfaceKeys are the labels the walkthrough screens use, including one per
// step status: a status with no label would render as a bare key beside the
// answer it belongs to.
var interfaceKeys = []string{
	"ui.wizards.heading", "ui.wizards.note", "ui.wizards.start",
	"ui.wizards.continue", "ui.wizards.stop", "ui.wizards.done",

	"agent.wizards.none", "agent.wizards.available", "agent.wizard.summary",
}

// TestEveryStepStatusHasALabel walks the statuses rather than listing them, so
// adding a ninth one to the engine fails here until the catalogs carry it.
func TestEveryStepStatusHasALabel(t *testing.T) {
	bundle, err := i18n.Load(i18n.Base)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	for _, status := range wizard.AllStatuses() {
		key := "ui.wizards.status." + string(status)
		if got := bundle.T(key); got == key {
			t.Errorf("step status %q has no label at %q", status, key)
		}
	}
}

// TestEveryWizardCarriesKeysNotProse keeps English sentences out of the code
// that decides what the user is told.
func TestEveryWizardCarriesKeysNotProse(t *testing.T) {
	for _, w := range wizard.Default.All() {
		for _, key := range []string{w.Title, w.Complaint} {
			if !strings.HasPrefix(key, "wizard.") {
				t.Errorf("%s: %q is not a message key", w.ID, key)
			}
		}
		for _, step := range w.Steps {
			for _, key := range []string{step.Title, step.Advice} {
				if key != "" && !strings.HasPrefix(key, "wizard.") {
					t.Errorf("%s: step %q carries %q, which is not a message key", w.ID, step.ID, key)
				}
			}
		}
	}
}

// TestPlatformSpecificWizardsAreOfferedOnlyWhereTheyRead mirrors the same rule
// for checks and fixes.
func TestPlatformSpecificWizardsAreOfferedOnlyWhereTheyRead(t *testing.T) {
	for _, os := range []platform.OS{platform.Darwin, platform.Linux} {
		for _, w := range wizard.Default.ForPlatform(os) {
			if w.ID == printing.ID {
				t.Errorf("%s is offered on %s, where it does not read the print service", printing.ID, os)
			}
		}
	}

	var found bool
	for _, w := range wizard.Default.ForPlatform(platform.Windows) {
		if w.ID == printing.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("%s is not offered on Windows, the one platform it reads", printing.ID)
	}
}

func ids(ws []*wizard.Wizard) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.ID
	}
	return out
}
