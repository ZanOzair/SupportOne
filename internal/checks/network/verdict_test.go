package network

import (
	"errors"
	"testing"

	"github.com/ZanOzair/supportone/internal/checks"
)

func connected() []iface {
	return []iface{{Name: "eth0", Up: true, Addresses: []string{"192.168.1.20/24"}}}
}

func TestConfigVerdict(t *testing.T) {
	full := routing{Gateway: "192.168.1.1", DNS: []string{"192.168.1.1"}}

	tests := []struct {
		name       string
		interfaces []iface
		route      routing
		routeErr   error
		want       checks.Severity
	}{
		{"connected with a gateway and DNS", connected(), full, nil, checks.SeverityOK},
		{"no routable address", []iface{{Name: "eth0", Up: true}}, full, nil, checks.SeverityUrgent},
		{"link-local only", []iface{{Name: "wlan0", Up: true, Addresses: []string{"169.254.4.4/16"}}}, full, nil, checks.SeverityUrgent},
		{"no gateway", connected(), routing{DNS: []string{"1.1.1.1"}}, nil, checks.SeverityAttention},
		{"no DNS", connected(), routing{Gateway: "192.168.1.1"}, nil, checks.SeverityAttention},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := configVerdict(tc.interfaces, tc.route, tc.routeErr).Severity; got != tc.want {
				t.Errorf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConfigVerdictDoesNotBlameTheMachineForItsOwnBlindSpot(t *testing.T) {
	// When the routing lookup itself failed, an empty gateway says nothing
	// about the network, so it must not be reported as a missing route.
	res := configVerdict(connected(), routing{}, errors.New("route table unreadable"))

	if res.Severity != checks.SeverityOK {
		t.Errorf("severity = %q, want ok when the gateway simply could not be read", res.Severity)
	}
	if res.Detail["routing_error"] == nil {
		t.Error("the reason the routing was unreadable was not recorded")
	}
}

func TestConfigVerdictNamesTheActiveInterfaces(t *testing.T) {
	res := configVerdict(
		[]iface{
			{Name: "eth0", Up: true, Addresses: []string{"192.168.1.20/24"}},
			{Name: "docker0", Up: false, Addresses: []string{"172.17.0.1/16"}},
		},
		routing{Gateway: "192.168.1.1", DNS: []string{"192.168.1.1"}},
		nil,
	)

	if res.Args[0] != "eth0" {
		t.Errorf("named %v, want only the interface that is up", res.Args[0])
	}
}
