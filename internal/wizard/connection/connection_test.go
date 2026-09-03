package connection

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/fixes/dns"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

// The interface list is the one thing this wizard reads directly, so tests
// supply their own rather than depending on whatever the machine running them
// happens to have.
func interfaces(found ...net.Interface) func() ([]net.Interface, error) {
	return func() ([]net.Interface, error) { return found, nil }
}

func TestIsRoutable(t *testing.T) {
	cases := map[string]bool{
		"192.168.1.20/24": true,
		"10.0.0.5/8":      true,
		"2001:db8::1/64":  true,
		"169.254.10.3/16": false, // no lease arrived
		"fe80::1/64":      false, // link-local
		"127.0.0.1/8":     false,
		"0.0.0.0/0":       false,
		"not an address":  false,
	}

	for addr, want := range cases {
		t.Run(addr, func(t *testing.T) {
			if got := isRoutable(stubAddr(addr)); got != want {
				t.Errorf("isRoutable(%q) = %v, want %v", addr, got, want)
			}
		})
	}
}

type stubAddr string

func (a stubAddr) Network() string { return "ip+net" }
func (a stubAddr) String() string  { return string(a) }

func TestLinkStepDistinguishesNoConnectionFromNoAddress(t *testing.T) {
	// Interface addresses come from the OS, and net.Interface cannot be given
	// them in a test, so the probe is exercised through the paths that do not
	// need them: no interfaces at all, and an interface that is down.
	cases := map[string]struct {
		found   []net.Interface
		summary string
	}{
		"nothing is connected": {nil, KeyLinkNone},
		"everything is down": {
			[]net.Interface{{Index: 1, Name: "eth0"}},
			KeyLinkNone,
		},
		"loopback does not count": {
			[]net.Interface{{Index: 1, Name: "lo", Flags: net.FlagUp | net.FlagLoopback}},
			KeyLinkNone,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := linkProbe(interfaces(tc.found...))(context.Background())
			if err != nil {
				t.Fatalf("probe: %v", err)
			}
			if got.OK {
				t.Error("OK = true with no usable connection")
			}
			if got.Summary != tc.summary {
				t.Errorf("Summary = %q, want %q", got.Summary, tc.summary)
			}
		})
	}
}

func TestAnInterfaceThatIsUpWithoutAnAddressIsNamedSeparately(t *testing.T) {
	// An interface that is up but got no lease is the everyday "connected to
	// the router, still no internet" case, and it deserves its own answer.
	up := []net.Interface{{Index: 1, Name: "wlan0", Flags: net.FlagUp}}

	got, err := linkProbe(interfaces(up...))(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got.OK {
		t.Error("OK = true for an interface with no routable address")
	}
	if got.Summary != KeyLinkNoAddress {
		t.Errorf("Summary = %q, want %q", got.Summary, KeyLinkNoAddress)
	}
	if len(got.Args) != 1 || got.Args[0] != 1 {
		t.Errorf("Args = %v, want the number of active connections", got.Args)
	}
}

func TestAnUnreadableInterfaceListIsNotNoConnection(t *testing.T) {
	fail := func() ([]net.Interface, error) { return nil, errors.New("permission denied") }

	got, err := linkProbe(fail)(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got.OK {
		t.Error("OK = true though the interface list could not be read")
	}
	if !got.Unknown || got.Summary != KeyLinkUnreadable {
		t.Errorf("got %+v, want it reported as unanswered", got)
	}
}

func TestCacheStepIsCleanWhenThereIsNoCacheToClear(t *testing.T) {
	// A machine with no resolver cache should not be invited to confirm a
	// change that would do nothing.
	noCache := &dns.Fix{OS: platform.OS("plan9")}

	got, err := cacheProbe(noCache)(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !got.OK || got.Summary != KeyCacheNone {
		t.Errorf("got %+v, want a clean answer", got)
	}
}

func TestTheWizardIsShapedSoItCanBeRegistered(t *testing.T) {
	w := New(checks.Default, dns.New(), interfaces(), wizard.DefaultTimeout)

	if err := wizard.NewRegistry().Register(w); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, os := range platform.All() {
		if !w.RunsOn(os) {
			t.Errorf("the wizard does not run on %s, though every platform has these questions", os)
		}
	}

	// The order is the order a technician asks in: nothing further is worth
	// asking until the machine is connected at all.
	want := []string{"connection.link", "connection.config", "connection.cache"}
	if len(w.Steps) != len(want) {
		t.Fatalf("the wizard has %d steps, want %d", len(w.Steps), len(want))
	}
	for i, id := range want {
		if w.Steps[i].ID != id {
			t.Errorf("step %d is %q, want %q", i, w.Steps[i].ID, id)
		}
	}

	if w.Steps[2].FixID != dns.ID {
		t.Errorf("the cache step offers %q, want %q", w.Steps[2].FixID, dns.ID)
	}
	// A cleared cache is indistinguishable from one that was never stale, and
	// the wizard must not claim otherwise.
	if !w.Steps[2].Unverifiable {
		t.Error("the cache step claims its repair can be verified by asking again")
	}
	for _, step := range w.Steps {
		if step.FixID == "" && step.Advice == "" {
			t.Errorf("step %q offers neither a fix nor advice", step.ID)
		}
	}
}

func TestTheWizardStopsAtTheFirstThingThatIsWrong(t *testing.T) {
	// No connection at all: there is no point asking about routers or caches
	// until that is fixed, and the wizard does not.
	w := New(checks.Default, dns.New(), interfaces(), wizard.DefaultTimeout)

	s := wizard.Start(w, nil, 0)
	got, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	if got.Step == nil || got.Step.StepID != "connection.link" {
		t.Fatalf("Step = %+v, want the connection step", got.Step)
	}
	if got.Advice != KeyLinkAdvice {
		t.Errorf("Advice = %q, want %q", got.Advice, KeyLinkAdvice)
	}
	if len(got.Done) != 0 {
		t.Errorf("Done = %+v, want nothing behind the first step", got.Done)
	}
}
