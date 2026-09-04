package explain

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/wizard"

	_ "github.com/ZanOzair/SupportOne/internal/checks/all"
	_ "github.com/ZanOzair/SupportOne/internal/fixes/all"
	_ "github.com/ZanOzair/SupportOne/internal/wizard/all"
)

// catalogKeys reads the English catalog directly, so the guard tests below
// measure the shipped message set rather than a list kept alongside it.
func catalogKeys(t *testing.T) map[string]string {
	t.Helper()

	raw, err := os.ReadFile("../i18n/locales/en.json")
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var messages map[string]string
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	return messages
}

// TestEveryCheckVerdictHasAnExplanation is the phase's gate, stated as a test:
// every verdict any compiled-in check can report is explained offline.
func TestEveryCheckVerdictHasAnExplanation(t *testing.T) {
	for key := range catalogKeys(t) {
		if !strings.HasPrefix(key, "check.") {
			continue
		}
		// A plural variant is the same finding as its base and shares a rule.
		base := Base(key)
		if _, ok := rules[base]; !ok {
			t.Errorf("check verdict %q has no rule in internal/explain", base)
		}
	}
}

// TestNoRuleExplainsAVerdictThatDoesNotExist is the same guard the other way
// round: a rule left behind after a check was renamed is dead weight that
// would never be reached.
func TestNoRuleExplainsAVerdictThatDoesNotExist(t *testing.T) {
	messages := catalogKeys(t)

	for verdict := range rules {
		if _, ok := messages[verdict]; !ok {
			t.Errorf("rule %q explains a verdict no catalog carries", verdict)
		}
	}
}

// TestEveryExplanationIsTranslated covers the causes, which are derived from
// the verdict keys rather than written twice.
func TestEveryExplanationIsTranslated(t *testing.T) {
	bundle, err := i18n.Load(i18n.Base)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	for verdict, r := range rules {
		cause := CauseKey(verdict)
		if got := bundle.T(cause); got == cause {
			t.Errorf("%s: the explanation key %q has no translation", verdict, cause)
		}
		if len(r.Steps) == 0 {
			t.Errorf("%s: no steps at all, not even 'nothing to do'", verdict)
		}
		for _, step := range r.Steps {
			if got := bundle.T(step); got == step {
				t.Errorf("%s: the step key %q has no translation", verdict, step)
			}
		}
	}

	if got := bundle.T(KeyEscalate); got == KeyEscalate {
		t.Errorf("the closing line %q has no translation", KeyEscalate)
	}
}

// TestEveryCatalogCarriesEveryExplanation keeps a language from silently
// falling behind: a missing key would fall back to English mid-report.
func TestEveryCatalogCarriesEveryExplanation(t *testing.T) {
	base := catalogKeys(t)

	for _, lang := range i18n.Available() {
		bundle, err := i18n.Load(lang)
		if err != nil {
			t.Fatalf("load %s: %v", lang, err)
		}
		messages := bundle.Messages()

		for key := range base {
			if !strings.HasPrefix(key, "explain.") {
				continue
			}
			if _, ok := messages[key]; !ok {
				t.Errorf("%s is missing %q", lang, key)
			}
		}
	}
}

// TestEveryNamedFixAndWizardIsCompiledIn stops the table pointing at something
// this binary does not carry.
func TestEveryNamedFixAndWizardIsCompiledIn(t *testing.T) {
	for verdict, r := range rules {
		for _, id := range r.Fixes {
			if _, ok := fixes.Default.Get(id); !ok {
				t.Errorf("%s names fix %q, which is not compiled in", verdict, id)
			}
		}
		for _, id := range r.Wizards {
			if _, ok := wizard.Default.Get(id); !ok {
				t.Errorf("%s names walkthrough %q, which is not compiled in", verdict, id)
			}
		}
	}
}

func newExplainer(os platform.OS) *Explainer {
	return New(fixes.Default, wizard.Default, os)
}

func TestForExplainsAFinding(t *testing.T) {
	res := checks.Result{
		CheckID:  "disk.smart",
		Severity: checks.SeverityUrgent,
		Summary:  "check.disk.smart.failing",
	}

	got, ok := newExplainer(platform.Linux).For(res)
	if !ok {
		t.Fatal("a shipped verdict had no explanation")
	}
	if got.CheckID != "disk.smart" || got.Severity != checks.SeverityUrgent {
		t.Errorf("advice = %+v, want it to carry the result's own identity", got)
	}
	if got.Cause != "explain.disk.smart.failing" {
		t.Errorf("Cause = %q", got.Cause)
	}
	// Backing up comes before replacing the drive. The order is the advice.
	if len(got.Steps) < 2 || got.Steps[0] != stepBackupNow {
		t.Errorf("Steps = %v, want backing up first", got.Steps)
	}
	if !got.Escalate {
		t.Error("a failing drive was not marked for escalation")
	}
	if got.Steps[len(got.Steps)-1] != KeyEscalate {
		t.Errorf("the closing line is missing from %v", got.Steps)
	}
}

func TestPluralVerdictsShareOneExplanation(t *testing.T) {
	plural := checks.Result{CheckID: "drivers.problem", Summary: "check.drivers.problem.found"}
	singular := checks.Result{CheckID: "drivers.problem", Summary: "check.drivers.problem.found.one"}

	e := newExplainer(platform.Windows)
	a, okA := e.For(plural)
	b, okB := e.For(singular)

	if !okA || !okB {
		t.Fatal("one of the two variants had no explanation")
	}
	if a.Cause != b.Cause {
		t.Errorf("causes differ: %q and %q", a.Cause, b.Cause)
	}
}

func TestAnExplanationOffersOnlyWhatThisBuildCanDo(t *testing.T) {
	res := checks.Result{CheckID: "disk.volumes", Summary: "check.disk.volumes.low"}

	got, ok := newExplainer(platform.Linux).For(res)
	if !ok {
		t.Fatal("no explanation")
	}
	if len(got.Fixes) != 1 || got.Fixes[0] != "temp.clear" {
		t.Errorf("Fixes = %v, want the one repair that runs here", got.Fixes)
	}

	// The same finding on a build with no fixes compiled in still explains
	// itself; it just has nothing to offer pressing.
	bare, ok := New(fixes.NewRegistry(), wizard.NewRegistry(), platform.Linux).For(res)
	if !ok {
		t.Fatal("no explanation from a build with no fixes")
	}
	if len(bare.Fixes) != 0 {
		t.Errorf("Fixes = %v, want none", bare.Fixes)
	}
	if bare.Cause == "" || len(bare.Steps) == 0 {
		t.Error("a build with no fixes lost its advice as well")
	}
}

func TestAFixThatDoesNotRunHereIsNotOffered(t *testing.T) {
	// print.clear-spooler is Windows-only. Nothing in the table points at it,
	// but the resolution path is what guarantees that stays true.
	e := newExplainer(platform.Linux)
	if got := e.knownFixes([]string{"print.clear-spooler"}); got != nil {
		t.Errorf("knownFixes = %v on Linux, want none", got)
	}
	if got := New(fixes.Default, wizard.Default, platform.Windows).knownFixes([]string{"print.clear-spooler"}); len(got) != 1 {
		t.Errorf("knownFixes = %v on Windows, want the one fix", got)
	}
}

func TestAWalkthroughThatDoesNotRunHereIsNotOffered(t *testing.T) {
	e := newExplainer(platform.Linux)
	if got := e.knownWizards([]string{"wizard.printing"}); got != nil {
		t.Errorf("knownWizards = %v on Linux, want none", got)
	}
	if got := e.knownWizards([]string{"wizard.nope"}); got != nil {
		t.Errorf("knownWizards = %v for an ID that does not exist, want none", got)
	}
}

func TestNetworkFindingsOfferTheWalkthrough(t *testing.T) {
	res := checks.Result{CheckID: "network.config", Summary: "check.network.config.no_dns"}

	got, ok := newExplainer(platform.Linux).For(res)
	if !ok {
		t.Fatal("no explanation")
	}
	if len(got.Wizards) != 1 || got.Wizards[0] != "wizard.connection" {
		t.Errorf("Wizards = %v, want the connection walkthrough", got.Wizards)
	}
	if len(got.Fixes) != 1 || got.Fixes[0] != "net.flush-dns" {
		t.Errorf("Fixes = %v, want the DNS repair", got.Fixes)
	}
}

// TestAResultThatIsFineIsStillExplained is the rule that keeps the report from
// teaching people that silence means it did not look.
func TestAResultThatIsFineIsStillExplained(t *testing.T) {
	res := checks.Result{CheckID: "os.info", Severity: checks.SeverityOK, Summary: "check.os.info.ok"}

	got, ok := newExplainer(platform.Linux).For(res)
	if !ok {
		t.Fatal("an OK result had no explanation")
	}
	if got.Cause == "" {
		t.Error("an OK result was explained with nothing")
	}
	if got.Escalate {
		t.Error("an OK result was marked for escalation")
	}
	if len(got.Steps) != 1 || got.Steps[0] != stepNothing {
		t.Errorf("Steps = %v, want the one 'nothing to do' step", got.Steps)
	}
}

func TestAVerdictWithNoRuleIsRefusedRatherThanInvented(t *testing.T) {
	res := checks.Result{CheckID: "made.up", Summary: "check.made.up.verdict"}

	if _, ok := newExplainer(platform.Linux).For(res); ok {
		t.Error("an explanation was produced for a verdict with no rule")
	}
}

func TestForSnapshotExplainsEveryResult(t *testing.T) {
	snap := checks.Snapshot{Results: []checks.Result{
		{CheckID: "os.info", Severity: checks.SeverityOK, Summary: "check.os.info.ok"},
		{CheckID: "disk.volumes", Severity: checks.SeverityAttention, Summary: "check.disk.volumes.low"},
		{CheckID: "disk.smart", Severity: checks.SeverityUrgent, Summary: "check.disk.smart.failing"},
	}}

	got := newExplainer(platform.Linux).ForSnapshot(snap)
	if len(got) != 3 {
		t.Fatalf("explained %d of %d results", len(got), len(snap.Results))
	}
	// ForSnapshot keeps the snapshot's order; Ordered is what re-sorts.
	if got[0].CheckID != "os.info" {
		t.Errorf("order = %v, want the snapshot's own", got)
	}
}

func TestOrderedPutsTheWorstFirst(t *testing.T) {
	advice := []Advice{
		{CheckID: "a", Severity: checks.SeverityOK},
		{CheckID: "b", Severity: checks.SeverityUnknown},
		{CheckID: "c", Severity: checks.SeverityUrgent},
		{CheckID: "d", Severity: checks.SeverityAttention},
		{CheckID: "e", Severity: checks.SeverityUrgent},
	}

	got := Ordered(advice)
	want := []string{"c", "e", "d", "b", "a"}
	for i, id := range want {
		if got[i].CheckID != id {
			t.Errorf("Ordered[%d] = %q, want %q (got %v)", i, got[i].CheckID, id, ids(got))
		}
	}

	// Sorting must not disturb the caller's slice.
	if advice[0].CheckID != "a" {
		t.Error("Ordered reordered its argument")
	}
}

func TestBaseAndCauseKey(t *testing.T) {
	if got := Base("check.disk.smart.ok.one"); got != "check.disk.smart.ok" {
		t.Errorf("Base = %q", got)
	}
	if got := Base("check.disk.smart.ok"); got != "check.disk.smart.ok" {
		t.Errorf("Base = %q", got)
	}
	if got := CauseKey("check.disk.smart.failing"); got != "explain.disk.smart.failing" {
		t.Errorf("CauseKey = %q", got)
	}
}

func TestVerdictsListsWhatTheTableAnswers(t *testing.T) {
	got := Verdicts()
	if len(got) != len(rules) {
		t.Fatalf("Verdicts returned %d entries, want %d", len(got), len(rules))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("Verdicts is not sorted: %q before %q", got[i-1], got[i])
		}
	}
}

func ids(advice []Advice) []string {
	out := make([]string, len(advice))
	for i, a := range advice {
		out[i] = a.CheckID
	}
	return out
}
