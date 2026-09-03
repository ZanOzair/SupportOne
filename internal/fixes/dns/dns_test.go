package dns

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// recorder captures what a fix asked the operating system to do, so a test can
// assert on commands without any of them running.
type recorder struct {
	calls []string
	err   error
}

func (r *recorder) run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	return nil, r.err
}

func present(string) (string, error) { return "/usr/bin/stub", nil }

func missing(name string) (string, error) {
	return "", errors.New(name + " is not installed")
}

func newFix(os platform.OS, rec *recorder) *Fix {
	return &Fix{OS: os, run: rec.run, lookPath: present}
}

func TestEveryTargetedPlatformHasAProcedure(t *testing.T) {
	for _, os := range platform.All() {
		if _, ok := procedures[os]; !ok {
			t.Errorf("no way to clear the DNS cache is defined for %s", os.Display())
		}
	}
}

func TestApplyRunsTheCommandsForItsPlatform(t *testing.T) {
	cases := map[platform.OS][]string{
		platform.Windows: {"ipconfig /flushdns"},
		platform.Darwin:  {"dscacheutil -flushcache", "killall -HUP mDNSResponder"},
		platform.Linux:   {"resolvectl flush-caches"},
	}

	for os, want := range cases {
		t.Run(string(os), func(t *testing.T) {
			rec := &recorder{}
			outcome, err := newFix(os, rec).Apply(context.Background())
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if !outcome.Applied {
				t.Error("Outcome.Applied = false after a successful run")
			}
			if len(rec.calls) != len(want) {
				t.Fatalf("commands = %v, want %v", rec.calls, want)
			}
			for i := range want {
				if rec.calls[i] != want[i] {
					t.Errorf("command %d = %q, want %q", i, rec.calls[i], want[i])
				}
			}
		})
	}
}

func TestApplyStopsAtTheFirstCommandThatFails(t *testing.T) {
	rec := &recorder{err: errors.New("access is denied")}

	outcome, err := newFix(platform.Darwin, rec).Apply(context.Background())
	if err == nil {
		t.Fatal("Apply reported success though its first command failed")
	}
	if outcome.Applied {
		t.Error("Outcome.Applied = true after a failure")
	}
	// macOS needs both halves; running the second after the first failed would
	// report more work than was done.
	if len(rec.calls) != 1 {
		t.Errorf("commands = %v, want the run to stop after the first", rec.calls)
	}
}

func TestApplyStopsWhenItsContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := &recorder{}
	if _, err := newFix(platform.Windows, rec).Apply(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("commands = %v, want none after cancellation", rec.calls)
	}
}

func TestPreflightRefusesWhenTheToolIsNotInstalled(t *testing.T) {
	f := &Fix{OS: platform.Windows, run: (&recorder{}).run, lookPath: missing}

	err := f.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight offered a change it has no tool to make")
	}
	if !strings.Contains(err.Error(), KeyBlockedNoTool) {
		t.Errorf("Preflight error = %q, want it to carry %q", err, KeyBlockedNoTool)
	}
	if !strings.Contains(err.Error(), "ipconfig") {
		t.Errorf("Preflight error = %q, want it to name the missing tool", err)
	}
}

func TestPreflightChangesNothing(t *testing.T) {
	rec := &recorder{}
	f := newFix(platform.Windows, rec)

	if err := f.Preflight(context.Background()); err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	// Preflight decides whether a change makes sense. It is not allowed to
	// make one.
	if len(rec.calls) != 0 {
		t.Errorf("Preflight ran %v; it must run nothing that changes the machine", rec.calls)
	}
}

func TestAnUnknownPlatformIsRefusedRatherThanGuessed(t *testing.T) {
	f := &Fix{OS: platform.OS("plan9"), run: (&recorder{}).run, lookPath: present}

	if err := f.Preflight(context.Background()); err == nil {
		t.Error("Preflight succeeded on a platform with no defined procedure")
	}
	if _, err := f.Apply(context.Background()); err == nil {
		t.Error("Apply invented a way to clear a cache on an unknown platform")
	}
}

func TestRollbackIsAnHonestNoOp(t *testing.T) {
	f := New()

	if f.Reversible() {
		t.Error("Reversible = true; this fix cannot put the previous cache back and must not claim to")
	}
	if err := f.Rollback(context.Background()); err != nil {
		t.Errorf("Rollback: %v", err)
	}
	if f.Describe().Undo == "" {
		t.Error("a fix that is not reversible must still tell the user where that leaves them")
	}
}

func TestTheFixDescribesItselfWellEnoughToBeRegistered(t *testing.T) {
	registry := fixes.NewRegistry()
	if err := registry.Register(New()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := registry.Get(ID)
	if !ok {
		t.Fatalf("the fix is not reachable by its own ID %q", ID)
	}
	if !got.RequiresAdmin() {
		t.Error("RequiresAdmin = false; the resolver cache does not belong to the user's session")
	}

	e := got.Describe()
	for _, key := range append([]string{e.Summary, e.Undo}, e.Changes...) {
		if !strings.HasPrefix(key, "fix.") {
			t.Errorf("%q is not a message key; explanations carry keys, not prose", key)
		}
	}
}
