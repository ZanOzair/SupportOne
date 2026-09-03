package all

import (
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/fixes/dns"
	"github.com/ZanOzair/SupportOne/internal/fixes/spooler"
	"github.com/ZanOzair/SupportOne/internal/fixes/temp"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/remediate"
)

// wantIDs is the full set of Phase 2 fixes, written out so that adding or
// renaming one that can change a machine is a deliberate edit here.
var wantIDs = []string{
	dns.ID,
	spooler.ID,
	temp.ID,
}

func TestEveryFixIsRegistered(t *testing.T) {
	registered := fixes.Default.All()
	if len(registered) != len(wantIDs) {
		t.Fatalf("registered %d fixes, want %d: %v", len(registered), len(wantIDs), ids(registered))
	}
	for i, want := range wantIDs {
		if registered[i].ID() != want {
			t.Errorf("fix %d = %q, want %q", i, registered[i].ID(), want)
		}
	}
}

// TestNothingOutsideTheRegistryCanBeNamed is the whitelist, stated as a test.
func TestNothingOutsideTheRegistryCanBeNamed(t *testing.T) {
	candidates := []string{
		"temp.clear",               // real
		"format.disk",              // never existed
		"temp.clear ",              // a trailing space is a different string
		"TEMP.CLEAR",               // case matters
		"../../../etc/passwd",      // a path, not an ID
		"temp.clear;net.flush-dns", // two IDs is not one ID
	}

	known, discarded := fixes.Default.Resolve(candidates, platform.Current())
	if len(known) != 1 || known[0].ID() != temp.ID {
		t.Fatalf("resolved %v, want only %q", ids(known), temp.ID)
	}
	if len(discarded) != len(candidates)-1 {
		t.Errorf("discarded %v, want everything but %q", discarded, temp.ID)
	}
}

func TestEveryFixDeclaresAPlatformItCanRunOn(t *testing.T) {
	for _, f := range fixes.Default.All() {
		if len(f.Platforms()) == 0 {
			t.Errorf("%s declares no platforms", f.ID())
		}
		for _, p := range f.Platforms() {
			if !p.Valid() {
				t.Errorf("%s declares unknown platform %q", f.ID(), p)
			}
		}
	}
}

func TestPlatformSpecificFixesAreOfferedOnlyWhereTheyWork(t *testing.T) {
	// print.clear-spooler understands the Windows spool directory and nothing
	// else. Offering it against a CUPS queue would be a different operation
	// wearing the same name.
	for _, os := range []platform.OS{platform.Darwin, platform.Linux} {
		for _, f := range fixes.Default.ForPlatform(os) {
			if f.ID() == spooler.ID {
				t.Errorf("%s is offered on %s, where it does not understand the print queue", spooler.ID, os)
			}
		}
	}
}

// TestEveryFixExplainsItselfBeforeItRuns guards the rule the whole consent
// gate rests on: a user cannot confirm a change they were not shown.
func TestEveryFixExplainsItselfBeforeItRuns(t *testing.T) {
	for _, f := range fixes.Default.All() {
		e := f.Describe()
		if e.Summary == "" {
			t.Errorf("%s has no summary", f.ID())
		}
		if len(e.Changes) == 0 {
			t.Errorf("%s lists no changes, so there is nothing for the user to confirm against", f.ID())
		}
		if e.Undo == "" {
			t.Errorf("%s does not say where the user stands afterwards", f.ID())
		}
	}
}

// TestEveryMessageKeyIsTranslated is the guard against a fix describing itself
// to the user as a bare message key.
func TestEveryMessageKeyIsTranslated(t *testing.T) {
	bundle, err := i18n.Load(i18n.Base)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	var keys []string
	for _, f := range fixes.Default.All() {
		e := f.Describe()
		keys = append(keys, e.Summary, e.Undo)
		keys = append(keys, e.Changes...)
	}
	keys = append(keys, outcomeKeys...)

	for _, key := range keys {
		if got := bundle.T(key); got == key {
			t.Errorf("message key %q has no translation", key)
		}
	}
}

// outcomeKeys are the keys a fix reports after the fact, or refuses with.
// They are not reachable from Describe, so they are listed.
var outcomeKeys = []string{
	remediate.KeyDryRun,

	temp.KeyBlockedNothing, temp.KeyBlockedUnreadable,
	temp.KeyOutcomeMoved, temp.KeyOutcomeMoved + ".one",
	temp.KeyOutcomeInUse, temp.KeyOutcomeInUse + ".one",

	dns.KeyBlockedNoTool, dns.KeyBlockedNoCache, dns.KeyBlockedElse,
	dns.KeyOutcomeCleared,

	spooler.KeyBlockedEmpty, spooler.KeyBlockedUnreadable,
	spooler.KeyOutcomeCleared, spooler.KeyOutcomeCleared + ".one",
}

// TestEveryFixCarriesKeysNotProse keeps English sentences out of the code that
// decides what happened.
func TestEveryFixCarriesKeysNotProse(t *testing.T) {
	for _, f := range fixes.Default.All() {
		e := f.Describe()
		for _, key := range append([]string{e.Summary, e.Undo}, e.Changes...) {
			if !strings.HasPrefix(key, "fix.") || strings.Contains(key, " ") {
				t.Errorf("%s: %q is not a message key", f.ID(), key)
			}
		}
	}
}

// TestAFixThatCannotUndoItselfSaysSo is the honesty check: the one fix here
// that does not restore prior state reports Reversible false rather than
// implying an undo that does not exist.
func TestAFixThatCannotUndoItselfSaysSo(t *testing.T) {
	f, ok := fixes.Default.Get(dns.ID)
	if !ok {
		t.Fatalf("%s is not registered", dns.ID)
	}
	if f.Reversible() {
		t.Errorf("%s claims to be reversible; the previous cache cannot be put back", dns.ID)
	}

	for _, id := range []string{temp.ID, spooler.ID} {
		f, ok := fixes.Default.Get(id)
		if !ok {
			t.Fatalf("%s is not registered", id)
		}
		if !f.Reversible() {
			t.Errorf("%s reports it cannot be undone, though it moves files rather than deleting them", id)
		}
	}
}

func ids(fs []fixes.Fix) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.ID()
	}
	return out
}
