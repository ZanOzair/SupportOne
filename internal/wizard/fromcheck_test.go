package wizard

import (
	"context"
	"errors"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// stubCheck returns a fixed result.
type stubCheck struct {
	id  string
	res checks.Result
	err error
}

func (c stubCheck) ID() string               { return c.id }
func (c stubCheck) Platforms() []platform.OS { return platform.All() }
func (c stubCheck) RequiresAdmin() bool      { return false }
func (c stubCheck) Run(context.Context) (checks.Result, error) {
	return c.res, c.err
}

func TestFromCheckCarriesTheCheckSVerdictIntoTheWizard(t *testing.T) {
	cases := map[string]struct {
		res     checks.Result
		wantOK  bool
		unknown bool
	}{
		"ok":        {checks.OK("check.ok"), true, false},
		"attention": {checks.Attention("check.attention"), false, false},
		"urgent":    {checks.Urgent("check.urgent"), false, false},
		"unknown":   {checks.Unknown("check.unknown"), false, true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			registry := checks.NewRegistry()
			if err := registry.Register(stubCheck{id: "stub.check", res: tc.res}); err != nil {
				t.Fatalf("register: %v", err)
			}

			probe := FromCheck(registry, "stub.check", 0)
			got, err := probe(context.Background())
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			if got.OK != tc.wantOK {
				t.Errorf("OK = %v, want %v", got.OK, tc.wantOK)
			}
			// A check that could not answer has cleared nothing.
			if got.Unknown != tc.unknown {
				t.Errorf("Unknown = %v, want %v", got.Unknown, tc.unknown)
			}
			if got.Summary != tc.res.Summary {
				t.Errorf("Summary = %q, want the check's own key %q", got.Summary, tc.res.Summary)
			}
		})
	}
}

func TestFromCheckRefusesACheckThatIsNotCompiledIn(t *testing.T) {
	probe := FromCheck(checks.NewRegistry(), "never.compiled-in", 0)

	if _, err := probe(context.Background()); err == nil {
		t.Error("the probe invented an answer for a check that does not exist")
	}
}

func TestAFailingCheckBecomesAnUnansweredQuestion(t *testing.T) {
	registry := checks.NewRegistry()
	if err := registry.Register(stubCheck{id: "stub.check", err: errors.New("the tool is missing")}); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, err := FromCheck(registry, "stub.check", 0)(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got.OK {
		t.Error("a check that failed was reported as clean")
	}
	if !got.Unknown {
		t.Error("a check that failed was not reported as unanswered")
	}
}
