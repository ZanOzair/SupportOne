// Package connection walks a user through "I can't get online".
//
// The questions are the ones a technician asks in order, and every one of them
// is answered by reading local configuration. Nothing here sends a packet: a
// tool that "tested your connection" by contacting a server would be making
// the outbound connection this agent promises not to make, and would be
// reporting on that server as much as on the machine.
package connection

import (
	"context"
	"net"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/fixes/dns"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

// ID is the stable identifier this wizard is offered and audited under.
const ID = "wizard.connection"

// Message keys this wizard resolves through internal/i18n.
const (
	KeyTitle     = "wizard.connection.title"
	KeyComplaint = "wizard.connection.complaint"

	KeyStepLink       = "wizard.connection.step.link"
	KeyLinkOK         = "wizard.connection.link.ok"
	KeyLinkNone       = "wizard.connection.link.none"
	KeyLinkNoAddress  = "wizard.connection.link.no_address"
	KeyLinkUnreadable = "wizard.connection.link.unreadable"
	KeyLinkAdvice     = "wizard.connection.link.advice"

	KeyStepConfig   = "wizard.connection.step.config"
	KeyConfigAdvice = "wizard.connection.config.advice"

	KeyStepCache   = "wizard.connection.step.cache"
	KeyCacheNone   = "wizard.connection.cache.none"
	KeyCacheStale  = "wizard.connection.cache.stale"
	KeyCacheAdvice = "wizard.connection.cache.advice"
)

// linkProbe answers the first question: is this machine connected to anything
// at all, and did it get an address?
//
// It reads the interface list the operating system already holds. A link that
// is up but carries only a link-local address is the everyday "connected to
// the router, no lease" case, and it is worth naming separately from having no
// connection at all.
func linkProbe(interfaces func() ([]net.Interface, error)) wizard.Probe {
	return func(context.Context) (wizard.Finding, error) {
		found, err := interfaces()
		if err != nil {
			return wizard.Finding{Unknown: true, Summary: KeyLinkUnreadable}, nil
		}

		var up, routable int
		for _, f := range found {
			if f.Flags&net.FlagLoopback != 0 || f.Flags&net.FlagUp == 0 {
				continue
			}
			up++

			addrs, err := f.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if isRoutable(addr) {
					routable++
					break
				}
			}
		}

		switch {
		case up == 0:
			return wizard.Finding{Summary: KeyLinkNone}, nil
		case routable == 0:
			return wizard.Finding{Summary: KeyLinkNoAddress, Args: []any{up}}, nil
		default:
			return wizard.Finding{OK: true, Summary: KeyLinkOK, Args: []any{routable}}, nil
		}
	}
}

// isRoutable reports whether an address is one the machine could actually send
// traffic from. A 169.254 or fe80:: address means the lease never arrived.
func isRoutable(addr net.Addr) bool {
	ip, _, err := net.ParseCIDR(addr.String())
	if err != nil {
		if ip = net.ParseIP(addr.String()); ip == nil {
			return false
		}
	}
	return !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsLoopback()
}

// cacheProbe answers the last question: is this machine keeping a saved list
// of website addresses that could be out of date?
//
// It asks the fix's own preflight, which is read-only. A machine with no cache
// gets a clean answer and no offer, rather than being invited to confirm a
// change that would do nothing.
func cacheProbe(fix *dns.Fix) wizard.Probe {
	return func(ctx context.Context) (wizard.Finding, error) {
		if err := fix.Preflight(ctx); err != nil {
			return wizard.Finding{OK: true, Summary: KeyCacheNone}, nil
		}
		return wizard.Finding{Summary: KeyCacheStale}, nil
	}
}

// New builds the wizard over the given check registry and DNS fix. The
// arguments are the seams tests substitute; production uses the defaults.
func New(registry *checks.Registry, fix *dns.Fix, interfaces func() ([]net.Interface, error), timeout time.Duration) *wizard.Wizard {
	return &wizard.Wizard{
		ID:        ID,
		Title:     KeyTitle,
		Complaint: KeyComplaint,
		Platforms: platform.All(),
		Steps: []wizard.Step{
			{
				// Nothing further is worth asking until this one is right,
				// which is why it is first.
				ID:     "connection.link",
				Title:  KeyStepLink,
				Ask:    linkProbe(interfaces),
				Advice: KeyLinkAdvice,
			},
			{
				// The router and DNS servers this machine is configured to
				// use, read by the same check the health report uses.
				ID:     "connection.config",
				Title:  KeyStepConfig,
				Ask:    wizard.FromCheck(registry, "network.config", timeout),
				Advice: KeyConfigAdvice,
			},
			{
				ID:     "connection.cache",
				Title:  KeyStepCache,
				Ask:    cacheProbe(fix),
				FixID:  dns.ID,
				Advice: KeyCacheAdvice,

				// A cleared cache looks exactly like a cache that was never
				// stale: it refills the moment anything is looked up. Asking
				// again would prove nothing, so the wizard does not pretend
				// it did.
				Unverifiable: true,
			},
		},
	}
}

func init() {
	wizard.MustRegister(New(checks.Default, dns.New(), net.Interfaces, wizard.DefaultTimeout))
}
