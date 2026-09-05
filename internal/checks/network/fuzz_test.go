package network

import "testing"

// Routing and DNS output. parseHexIPv4 in particular reads hex straight out of
// /proc/net/route, which is exactly the shape of input that rewards fuzzing.

func FuzzParseProcRoute(f *testing.F) {
	f.Add([]byte("Iface\tDestination\tGateway\n eth0\t00000000\t0102A8C0\n"))
	f.Add([]byte("eth0\t00000000"))
	f.Add([]byte("\t\t\t"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseProcRoute(data)
		_ = parseResolvConf(data)
	})
}

func FuzzParseHexIPv4(f *testing.F) {
	f.Add("0102A8C0")
	f.Add("ZZZZZZZZ")
	f.Add("")
	f.Add("0102A8C0FFFFFFFFFFFF")

	f.Fuzz(func(t *testing.T, s string) {
		ip, err := parseHexIPv4(s)
		if err == nil && ip == nil {
			t.Errorf("parseHexIPv4(%q) returned no error and no address", s)
		}
	})
}

func FuzzParseMacNetwork(f *testing.F) {
	f.Add([]byte("   gateway: 192.168.1.1\n   interface: en0\n"))
	f.Add([]byte("gateway:"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parseRouteGet(data)
		_ = parseSCUtilDNS(data)
	})
}

func FuzzParseWindowsRouting(f *testing.F) {
	f.Add([]byte(`{"NextHop":"192.168.1.1","InterfaceAlias":"Ethernet"}`))
	f.Add([]byte(`{"NextHop":`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWindowsRouting(data)
	})
}
