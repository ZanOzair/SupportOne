package checks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

type stubCheck struct {
	id        string
	platforms []platform.OS
	admin     bool
	run       func(ctx context.Context) (Result, error)
}

func (s stubCheck) ID() string               { return s.id }
func (s stubCheck) Platforms() []platform.OS { return s.platforms }
func (s stubCheck) RequiresAdmin() bool      { return s.admin }
func (s stubCheck) Run(ctx context.Context) (Result, error) {
	if s.run != nil {
		return s.run(ctx)
	}
	return Result{Severity: SeverityOK, Summary: "check.stub.ok"}, nil
}

func TestRegisterRejectsInvalidChecks(t *testing.T) {
	all := []platform.OS{platform.Windows, platform.Darwin, platform.Linux}

	tests := []struct {
		name  string
		check Check
	}{
		{"nil check", nil},
		{"undotted id", stubCheck{id: "osinfo", platforms: all}},
		{"uppercase id", stubCheck{id: "OS.Info", platforms: all}},
		{"trailing dot", stubCheck{id: "os.", platforms: all}},
		{"no platforms", stubCheck{id: "os.info"}},
		{"unknown platform", stubCheck{id: "os.info", platforms: []platform.OS{"plan9"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := NewRegistry().Register(tc.check); err == nil {
				t.Fatal("Register succeeded, want error")
			}
		})
	}
}

func TestRegisterRejectsDuplicateID(t *testing.T) {
	r := NewRegistry()
	c := stubCheck{id: "os.info", platforms: []platform.OS{platform.Linux}}
	if err := r.Register(c); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(c); err == nil {
		t.Fatal("duplicate Register succeeded, want error")
	}
}

func TestForPlatformFiltersAndOrders(t *testing.T) {
	r := NewRegistry()
	mustRegister(t, r, stubCheck{id: "network.config", platforms: []platform.OS{platform.Windows, platform.Linux}})
	mustRegister(t, r, stubCheck{id: "drivers.problem", platforms: []platform.OS{platform.Windows}})
	mustRegister(t, r, stubCheck{id: "battery.health", platforms: []platform.OS{platform.Darwin}})

	got := ids(r.ForPlatform(platform.Windows))
	want := []string{"drivers.problem", "network.config"}
	if !equal(got, want) {
		t.Errorf("ForPlatform(windows) = %v, want %v", got, want)
	}

	if got := ids(r.ForPlatform(platform.Linux)); !equal(got, []string{"network.config"}) {
		t.Errorf("ForPlatform(linux) = %v, want [network.config]", got)
	}
}

func TestRunRecordsOutcome(t *testing.T) {
	res := Run(context.Background(), stubCheck{
		id:        "os.info",
		platforms: []platform.OS{platform.Linux},
		run: func(context.Context) (Result, error) {
			return Result{Severity: SeverityAttention, Summary: "check.os.outdated"}, nil
		},
	}, time.Second)

	if res.CheckID != "os.info" {
		t.Errorf("CheckID = %q, want os.info", res.CheckID)
	}
	if res.Severity != SeverityAttention {
		t.Errorf("Severity = %q, want attention", res.Severity)
	}
	if res.StartedAt.IsZero() {
		t.Error("StartedAt not set")
	}
}

func TestRunReportsUnknownOnFailure(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context) (Result, error)
	}{
		{"error", func(context.Context) (Result, error) {
			return Result{}, errors.New("smart data unavailable without elevation")
		}},
		{"panic", func(context.Context) (Result, error) { panic("nil map write") }},
		{"invalid severity", func(context.Context) (Result, error) {
			return Result{Severity: "great"}, nil
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Run(context.Background(), stubCheck{
				id:        "disk.smart",
				platforms: []platform.OS{platform.Linux},
				run:       tc.run,
			}, time.Second)

			if res.Severity != SeverityUnknown {
				t.Errorf("Severity = %q, want unknown — a check that cannot answer must never report ok", res.Severity)
			}
			if res.Err == "" {
				t.Error("Err is empty, want the reason recorded")
			}
		})
	}
}

func TestRunTimesOut(t *testing.T) {
	res := Run(context.Background(), stubCheck{
		id:        "disk.smart",
		platforms: []platform.OS{platform.Linux},
		run: func(ctx context.Context) (Result, error) {
			<-ctx.Done()
			return Result{}, ctx.Err()
		},
	}, 10*time.Millisecond)

	if res.Severity != SeverityUnknown {
		t.Errorf("Severity = %q, want unknown", res.Severity)
	}
}

func mustRegister(t *testing.T, r *Registry, c Check) {
	t.Helper()
	if err := r.Register(c); err != nil {
		t.Fatalf("Register(%s): %v", c.ID(), err)
	}
}

func ids(cs []Check) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID()
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
