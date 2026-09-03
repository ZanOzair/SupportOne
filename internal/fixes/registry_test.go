package fixes

import (
	"context"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

type stubFix struct {
	id        string
	platforms []platform.OS
	explain   Explanation
}

func (s stubFix) ID() string               { return s.id }
func (s stubFix) Platforms() []platform.OS { return s.platforms }
func (s stubFix) RequiresAdmin() bool      { return false }
func (s stubFix) Reversible() bool         { return true }

func (s stubFix) Describe() Explanation { return s.explain }

func describedFix(id string, platforms ...platform.OS) stubFix {
	return stubFix{
		id:        id,
		platforms: platforms,
		explain: Explanation{
			Summary: "fix.stub.summary",
			Changes: []string{"fix.stub.change"},
			Undo:    "fix.stub.undo",
		},
	}
}

func (s stubFix) Preflight(context.Context) error { return nil }
func (s stubFix) Rollback(context.Context) error  { return nil }

func (s stubFix) Apply(context.Context) (Outcome, error) {
	return Outcome{FixID: s.id, Applied: true}, nil
}

func TestRegisterRequiresDescribedChanges(t *testing.T) {
	tests := []struct {
		name string
		fix  Fix
	}{
		{"no summary", stubFix{
			id:        "net.flush-dns",
			platforms: []platform.OS{platform.Windows},
			explain:   Explanation{Changes: []string{"fix.stub.change"}},
		}},
		{"no listed changes", stubFix{
			id:        "net.flush-dns",
			platforms: []platform.OS{platform.Windows},
			explain:   Explanation{Summary: "fix.stub.summary"},
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := NewRegistry().Register(tc.fix); err == nil {
				t.Fatal("Register succeeded, want error — a fix the user cannot review must not be offerable")
			}
		})
	}
}

func TestRegisterRejectsInvalidFixes(t *testing.T) {
	tests := []struct {
		name string
		fix  Fix
	}{
		{"nil fix", nil},
		{"undotted id", describedFix("flushdns", platform.Windows)},
		{"no platforms", describedFix("net.flush-dns")},
		{"unknown platform", describedFix("net.flush-dns", "beos")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := NewRegistry().Register(tc.fix); err == nil {
				t.Fatal("Register succeeded, want error")
			}
		})
	}
}

func TestResolveDropsUnknownAndOffPlatformIDs(t *testing.T) {
	r := NewRegistry()
	for _, f := range []Fix{
		describedFix("net.flush-dns", platform.Windows, platform.Linux),
		describedFix("print.clear-spooler", platform.Windows),
	} {
		if err := r.Register(f); err != nil {
			t.Fatalf("Register(%s): %v", f.ID(), err)
		}
	}

	candidates := []string{
		"net.flush-dns",
		"print.clear-spooler", // registered, but Windows-only
		"disk.format-c",       // never existed
		"rm -rf /",            // not even an ID
	}
	known, discarded := r.Resolve(candidates, platform.Linux)

	if len(known) != 1 || known[0].ID() != "net.flush-dns" {
		t.Fatalf("known = %v, want [net.flush-dns]", fixIDs(known))
	}
	want := []string{"print.clear-spooler", "disk.format-c", "rm -rf /"}
	if len(discarded) != len(want) {
		t.Fatalf("discarded = %v, want %v", discarded, want)
	}
	for i := range want {
		if discarded[i] != want[i] {
			t.Errorf("discarded[%d] = %q, want %q", i, discarded[i], want[i])
		}
	}
}

func fixIDs(fs []Fix) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.ID()
	}
	return out
}

func TestGetAllAndForPlatform(t *testing.T) {
	r := NewRegistry()
	for _, f := range []Fix{
		describedFix("print.clear-spooler", platform.Windows),
		describedFix("net.flush-dns", platform.Windows, platform.Linux),
	} {
		if err := r.Register(f); err != nil {
			t.Fatalf("Register(%s): %v", f.ID(), err)
		}
	}

	if _, ok := r.Get("net.flush-dns"); !ok {
		t.Error("Get(net.flush-dns) not found")
	}
	if _, ok := r.Get("net.flush-cache"); ok {
		t.Error("Get(net.flush-cache) found an unregistered ID")
	}

	if got := fixIDs(r.All()); !equalIDs(got, []string{"net.flush-dns", "print.clear-spooler"}) {
		t.Errorf("All() = %v, want IDs in sorted order", got)
	}
	if got := fixIDs(r.ForPlatform(platform.Linux)); !equalIDs(got, []string{"net.flush-dns"}) {
		t.Errorf("ForPlatform(linux) = %v, want [net.flush-dns]", got)
	}
}

func TestMustRegisterPanicsOnInvalidFix(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustRegister did not panic on an invalid fix")
		}
	}()
	MustRegister(describedFix("nodots"))
}

func TestApplyReportsTheFixThatRan(t *testing.T) {
	f := describedFix("net.flush-dns", platform.Linux)
	if err := f.Preflight(context.Background()); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	out, err := f.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out.FixID != f.ID() || !out.Applied {
		t.Errorf("Outcome = %+v, want applied outcome for %s", out, f.ID())
	}
	if err := f.Rollback(context.Background()); err != nil {
		t.Errorf("Rollback: %v", err)
	}
}

func equalIDs(a, b []string) bool {
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
