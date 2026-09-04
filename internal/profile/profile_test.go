package profile

import (
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"

	_ "github.com/ZanOzair/SupportOne/internal/fixes/all"
)

const valid = `{
  "schema": 1,
  "name": "Acme standard build",
  "expectations": [
    {"check": "security.posture", "worst": "ok", "why": "Client contract requires disk encryption."},
    {"check": "disk.volumes", "worst": "attention", "offer": ["temp.clear"]}
  ]
}`

func snapshot(results ...checks.Result) checks.Snapshot {
	return checks.Snapshot{
		Schema:  checks.SnapshotSchema,
		Host:    platform.Host{OS: platform.Linux, Arch: "amd64"},
		Results: results,
	}
}

func result(id string, severity checks.Severity) checks.Result {
	return checks.Result{CheckID: id, Severity: severity, Summary: "check." + id + ".ok"}
}

func TestLoadAcceptsAProfileItCanActOn(t *testing.T) {
	p, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Name != "Acme standard build" {
		t.Errorf("Name = %q", p.Name)
	}
	if len(p.Expectations) != 2 {
		t.Errorf("Expectations = %d, want 2", len(p.Expectations))
	}
	if p.Expectations[0].Why == "" {
		t.Error("the technician's own note did not survive")
	}
}

func TestLoadRefusesAProfileItCannotActOnHonestly(t *testing.T) {
	cases := map[string]string{
		"a schema this build does not understand": `{"schema":99,"name":"x","expectations":[{"check":"os.info","worst":"ok"}]}`,
		"no name":                        `{"schema":1,"expectations":[{"check":"os.info","worst":"ok"}]}`,
		"no expectations":                `{"schema":1,"name":"x","expectations":[]}`,
		"an expectation naming no check": `{"schema":1,"name":"x","expectations":[{"worst":"ok"}]}`,
		"a severity that is not one":     `{"schema":1,"name":"x","expectations":[{"check":"os.info","worst":"critical"}]}`,
		// Two rules for one check would let a profile contradict itself and
		// report both answers.
		"two rules for one check": `{"schema":1,"name":"x","expectations":[{"check":"os.info","worst":"ok"},{"check":"os.info","worst":"urgent"}]}`,
		// A misspelled field is a rule that would silently not apply, which
		// in a compliance document is worse than a parse error.
		"a misspelled field": `{"schema":1,"name":"x","expectations":[{"check":"os.info","wors":"ok"}]}`,
		"not JSON at all":    `nonsense`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(strings.NewReader(body)); err == nil {
				t.Error("Load accepted it")
			}
		})
	}
}

func TestAProfileWithTooManyRulesIsRefused(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"schema":1,"name":"x","expectations":[`)
	for i := 0; i < MaxExpectations+1; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"check":"check.`)
		b.WriteString(strings.Repeat("a", i%20+1))
		b.WriteString(`","worst":"ok"}`)
	}
	b.WriteString(`]}`)

	if _, err := Load(strings.NewReader(b.String())); err == nil {
		t.Error("Load accepted a profile larger than it reads")
	}
}

func TestMeasureAgainstAConformingMachine(t *testing.T) {
	p, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := p.Measure(snapshot(
		result("security.posture", checks.SeverityOK),
		result("disk.volumes", checks.SeverityAttention),
	), fixes.Default, platform.Linux)

	if !got.Conforms() {
		t.Errorf("Conforms = false: %+v", got)
	}
	if got.Met != 2 {
		t.Errorf("Met = %d, want 2", got.Met)
	}
	// "attention" was allowed for disk.volumes, so a warning is met.
	for _, f := range got.Findings {
		if f.State != StateMet {
			t.Errorf("%s = %q, want met", f.Check, f.State)
		}
	}
}

func TestAVerdictWorseThanAllowedIsUnmet(t *testing.T) {
	p, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := p.Measure(snapshot(
		result("security.posture", checks.SeverityAttention),
		result("disk.volumes", checks.SeverityUrgent),
	), fixes.Default, platform.Linux)

	if got.Conforms() {
		t.Error("Conforms = true with two rules broken")
	}
	if got.Unmet != 2 {
		t.Errorf("Unmet = %d, want 2", got.Unmet)
	}

	// The fix the profile named is offered, resolved through the registry.
	for _, f := range got.Findings {
		if f.Check != "disk.volumes" {
			continue
		}
		if len(f.Offer) != 1 || f.Offer[0] != "temp.clear" {
			t.Errorf("Offer = %v, want the one repair this build carries", f.Offer)
		}
	}
}

// TestCouldNotCheckIsNeverCompliance is the rule a standard rests on: a check
// that did not run is not a check that passed.
func TestCouldNotCheckIsNeverCompliance(t *testing.T) {
	p, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := p.Measure(snapshot(
		result("security.posture", checks.SeverityUnknown),
		result("disk.volumes", checks.SeverityOK),
	), fixes.Default, platform.Linux)

	if got.Conforms() {
		t.Error("Conforms = true with a check that could not answer")
	}
	if got.Unknown != 1 {
		t.Errorf("Unknown = %d, want 1", got.Unknown)
	}
}

func TestACheckSkippedForWantOfRightsIsUnknownNotMissing(t *testing.T) {
	p, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	snap := snapshot(result("disk.volumes", checks.SeverityOK))
	snap.SkippedAdmin = []string{"security.posture"}

	got := p.Measure(snap, fixes.Default, platform.Linux)

	// The check exists and was not run. That is a question we did not answer,
	// not a rule naming something that does not exist.
	if got.Unknown != 1 || got.Missing != 0 {
		t.Errorf("unknown=%d missing=%d, want 1 and 0", got.Unknown, got.Missing)
	}
	if got.Conforms() {
		t.Error("Conforms = true with a check that was skipped")
	}
}

func TestARuleNamingACheckThisBuildDoesNotHaveIsMissing(t *testing.T) {
	body := `{"schema":1,"name":"x","expectations":[{"check":"never.compiled-in","worst":"ok"}]}`
	p, err := Load(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := p.Measure(snapshot(result("os.info", checks.SeverityOK)), fixes.Default, platform.Linux)

	if got.Missing != 1 {
		t.Errorf("Missing = %d, want 1", got.Missing)
	}
	// A standard that could not be measured is not a standard that was met.
	if got.Conforms() {
		t.Error("Conforms = true with a rule that could not be measured")
	}
}

// TestAProfileCannotNameAnActionThisBuildLacks is the whitelist again: a
// profile is data, and cannot become a program.
func TestAProfileCannotNameAnActionThisBuildLacks(t *testing.T) {
	body := `{"schema":1,"name":"x","expectations":[
	  {"check":"disk.volumes","worst":"ok","offer":["temp.clear","rm -rf /","format.disk","print.clear-spooler"]}
	]}`
	p, err := Load(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := p.Measure(snapshot(result("disk.volumes", checks.SeverityUrgent)), fixes.Default, platform.Linux)

	offer := got.Findings[0].Offer
	// temp.clear survives; the shell string and the invented ID resolve to
	// nothing, and print.clear-spooler is Windows-only.
	if len(offer) != 1 || offer[0] != "temp.clear" {
		t.Errorf("Offer = %v, want only the repair that runs here", offer)
	}
}

func TestAMetExpectationOffersNothing(t *testing.T) {
	p, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := p.Measure(snapshot(
		result("security.posture", checks.SeverityOK),
		result("disk.volumes", checks.SeverityOK),
	), fixes.Default, platform.Linux)

	for _, f := range got.Findings {
		if len(f.Offer) != 0 {
			t.Errorf("%s offers %v though it is met", f.Check, f.Offer)
		}
	}
}

func TestABuildWithNoFixesStillMeasures(t *testing.T) {
	p, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := p.Measure(snapshot(result("disk.volumes", checks.SeverityUrgent)), nil, platform.Linux)

	if got.Unmet != 1 {
		t.Errorf("Unmet = %d, want 1", got.Unmet)
	}
	if len(got.Findings[1].Offer) != 0 {
		t.Error("a build with no fixes offered one")
	}
}

func TestOrderedPutsFailuresFirst(t *testing.T) {
	findings := []Finding{
		{Check: "a", State: StateMet},
		{Check: "b", State: StateMissing},
		{Check: "c", State: StateUnmet},
		{Check: "d", State: StateUnknown},
	}

	got := Ordered(findings)
	want := []string{"c", "d", "b", "a"}
	for i, id := range want {
		if got[i].Check != id {
			t.Errorf("position %d = %q, want %q", i, got[i].Check, id)
		}
	}
	// Sorting must not disturb the caller's slice.
	if findings[0].Check != "a" {
		t.Error("Ordered reordered its argument")
	}
}

func TestTheReportNamesTheStandardItMeasuredAgainst(t *testing.T) {
	p, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := p.Measure(snapshot(result("os.info", checks.SeverityOK)), fixes.Default, platform.Linux)
	if got.Profile != "Acme standard build" {
		t.Errorf("Profile = %q", got.Profile)
	}
}
