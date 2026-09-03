package network

import (
	"testing"
)

func TestParseProcRoute(t *testing.T) {
	fixture := []byte(`Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlan0	00000000	0101A8C0	0003	0	0	600	00000000	0	0	0
wlan0	0001A8C0	00000000	0001	0	0	600	00FFFFFF	0	0	0
`)

	if got := parseProcRoute(fixture); got != "192.168.1.1" {
		t.Errorf("parseProcRoute = %q, want 192.168.1.1", got)
	}
}

func TestParseProcRouteWithNoDefaultRoute(t *testing.T) {
	fixture := []byte(`Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	0001A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0
`)

	if got := parseProcRoute(fixture); got != "" {
		t.Errorf("parseProcRoute = %q, want empty when there is no default route", got)
	}
}

func TestParseResolvConf(t *testing.T) {
	fixture := []byte(`# Managed by systemd-resolved
nameserver 127.0.0.53
nameserver 1.1.1.1
; a comment
nameserver not-an-address
options edns0
search example.invalid
`)

	got := parseResolvConf(fixture)
	want := []string{"127.0.0.53", "1.1.1.1"}
	if len(got) != len(want) {
		t.Fatalf("parseResolvConf = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("nameserver %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseRouteGet(t *testing.T) {
	fixture := []byte(`   route to: default
destination: default
       mask: default
    gateway: 192.168.1.254
  interface: en0
`)

	if got := parseRouteGet(fixture); got != "192.168.1.254" {
		t.Errorf("parseRouteGet = %q, want 192.168.1.254", got)
	}
	if got := parseRouteGet([]byte("route: writing to routing socket: not in table\n")); got != "" {
		t.Errorf("parseRouteGet = %q, want empty when there is no default route", got)
	}
}

func TestParseSCUtilDNSDeduplicates(t *testing.T) {
	fixture := []byte(`DNS configuration

resolver #1
  nameserver[0] : 192.168.1.1
  nameserver[1] : 1.1.1.1

DNS configuration (for scoped queries)

resolver #1
  nameserver[0] : 192.168.1.1
  nameserver[1] : not-an-address
`)

	got := parseSCUtilDNS(fixture)
	want := []string{"192.168.1.1", "1.1.1.1"}
	if len(got) != len(want) {
		t.Fatalf("parseSCUtilDNS = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("resolver %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseWindowsRouting(t *testing.T) {
	fixture := []byte(`{"Gateway":"192.168.0.1","Dns":["192.168.0.1","8.8.8.8","192.168.0.1"]}`)

	got, err := parseWindowsRouting(fixture)
	if err != nil {
		t.Fatalf("parseWindowsRouting: %v", err)
	}
	if got.Gateway != "192.168.0.1" {
		t.Errorf("gateway = %q", got.Gateway)
	}
	if len(got.DNS) != 2 {
		t.Errorf("dns = %v, want the duplicate dropped", got.DNS)
	}
}

func TestParseWindowsRoutingWithNothingConfigured(t *testing.T) {
	got, err := parseWindowsRouting([]byte(`{"Gateway":null,"Dns":[]}`))
	if err != nil {
		t.Fatalf("parseWindowsRouting: %v", err)
	}
	if got.Gateway != "" || len(got.DNS) != 0 {
		t.Errorf("routing = %+v, want empty", got)
	}
}

func TestActiveInterfacesIgnoresLinkLocalOnly(t *testing.T) {
	interfaces := []iface{
		{Name: "eth0", Up: true, Addresses: []string{"192.168.1.20/24", "fe80::1/64"}},
		{Name: "wlan0", Up: true, Addresses: []string{"169.254.13.7/16"}},
		{Name: "eth1", Up: false, Addresses: []string{"10.0.0.5/8"}},
		{Name: "eth2", Up: true},
	}

	active := activeInterfaces(interfaces)
	if len(active) != 1 || active[0].Name != "eth0" {
		t.Errorf("active = %+v, want only eth0 — a link-local address means no lease", active)
	}
}
