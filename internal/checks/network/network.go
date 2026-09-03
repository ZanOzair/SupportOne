// Package network reports how the machine is connected: its interfaces and
// addresses, the router it sends traffic to, and the DNS servers it asks.
//
// The check is read-only and makes no outbound connection. It reads local
// configuration; it does not ping anything, and it does not tell anyone the
// machine exists.
package network

import (
	"context"
	"net"
	"sort"
	"strings"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/platform"
)

// iface is one network interface as the user's machine sees it.
type iface struct {
	Name      string   `json:"name"`
	Hardware  string   `json:"mac,omitempty"`
	Up        bool     `json:"up"`
	Addresses []string `json:"addresses,omitempty"`
}

// routing is the machine's view of where traffic goes.
type routing struct {
	Gateway string   `json:"gateway,omitempty"`
	DNS     []string `json:"dns,omitempty"`
}

// Message keys for this package's results.
const (
	keyNetworkOK         = "check.network.config.ok"
	keyNetworkNoAddress  = "check.network.config.no_address"
	keyNetworkNoGateway  = "check.network.config.no_gateway"
	keyNetworkNoDNS      = "check.network.config.no_dns"
	keyNetworkInterfaces = "check.network.config.interfaces_unreadable"
)

type configCheck struct{ run platform.Runner }

func (configCheck) ID() string               { return "network.config" }
func (configCheck) Platforms() []platform.OS { return platform.All() }
func (configCheck) RequiresAdmin() bool      { return false }

func (c configCheck) Run(ctx context.Context) (checks.Result, error) {
	interfaces, err := collectInterfaces()
	if err != nil {
		res := checks.Unknown(keyNetworkInterfaces)
		res.Err = err.Error()
		return res, nil
	}

	// Routing detail is best-effort: losing it should not cost the interface
	// list, which is the more useful half of the answer.
	route, routeErr := collectRouting(ctx, c.run)
	return configVerdict(interfaces, route, routeErr), nil
}

// configVerdict judges what the machine is connected to. It is separate from
// collection so each state can be tested without rewiring a network.
func configVerdict(interfaces []iface, route routing, routeErr error) checks.Result {
	detail := map[string]any{"interfaces": interfaces}
	if route.Gateway != "" {
		detail["gateway"] = route.Gateway
	}
	if len(route.DNS) > 0 {
		detail["dns"] = route.DNS
	}
	if routeErr != nil {
		detail["routing_error"] = routeErr.Error()
	}

	active := activeInterfaces(interfaces)
	if len(active) == 0 {
		return checks.Urgent(keyNetworkNoAddress).With(detail)
	}

	names := make([]string, 0, len(active))
	for _, i := range active {
		names = append(names, i.Name)
	}
	joined := strings.Join(names, ", ")

	switch {
	case route.Gateway == "" && routeErr == nil:
		return checks.Attention(keyNetworkNoGateway, joined).With(detail)
	case len(route.DNS) == 0 && routeErr == nil:
		return checks.Attention(keyNetworkNoDNS, joined).With(detail)
	default:
		return checks.OK(keyNetworkOK, joined, route.Gateway).With(detail)
	}
}

// collectInterfaces reads the interface list through the standard library, so
// every platform shares one implementation.
func collectInterfaces() ([]iface, error) {
	found, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	out := make([]iface, 0, len(found))
	for _, f := range found {
		if f.Flags&net.FlagLoopback != 0 {
			continue
		}

		entry := iface{
			Name:     f.Name,
			Hardware: f.HardwareAddr.String(),
			Up:       f.Flags&net.FlagUp != 0,
		}
		addrs, err := f.Addrs()
		if err != nil {
			// An interface that disappeared mid-scan is skipped rather than
			// reported with no addresses as though it had none.
			continue
		}
		for _, addr := range addrs {
			entry.Addresses = append(entry.Addresses, addr.String())
		}
		out = append(out, entry)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// activeInterfaces returns the interfaces that are up and hold a routable
// address. A link-local address alone means the machine did not get a lease.
func activeInterfaces(interfaces []iface) []iface {
	var out []iface
	for _, i := range interfaces {
		if !i.Up {
			continue
		}
		for _, addr := range i.Addresses {
			ip, _, err := net.ParseCIDR(addr)
			if err != nil {
				if ip = net.ParseIP(addr); ip == nil {
					continue
				}
			}
			if ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
				continue
			}
			out = append(out, i)
			break
		}
	}
	return out
}

func init() {
	checks.MustRegister(configCheck{run: platform.RunRead})
}
