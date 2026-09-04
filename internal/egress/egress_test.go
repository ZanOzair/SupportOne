package egress

import (
	"errors"
	"testing"
)

func TestCheckURLAllowsHTTPSAnywhereAndHTTPOnlyHere(t *testing.T) {
	allowed := []string{
		"https://api.example.com/v1/chat/completions",
		"https://fleet.example.com/api/report",
		"https://127.0.0.1:8443/api/report",
		// The traffic never leaves the computer, and refusing this would
		// push people towards a hosted service instead.
		"http://127.0.0.1:11434/v1/chat/completions",
		"http://localhost:8080/api/report",
		"http://[::1]:8080/api/report",
	}
	for _, endpoint := range allowed {
		if err := CheckURL(endpoint); err != nil {
			t.Errorf("CheckURL(%q) = %v, want allowed", endpoint, err)
		}
	}

	refused := map[string]bool{
		"":                             false,
		"   ":                          false,
		"http://fleet.example.com/api": true,
		"http://192.168.1.50:8080/api": true,
		"http://10.0.0.5/api":          true,
		"ftp://example.com/":           false,
		"file:///etc/passwd":           false,
		"ws://example.com/":            false,
		"https://":                     false,
		"not a url at all":             false,
	}
	for endpoint, insecure := range refused {
		err := CheckURL(endpoint)
		if err == nil {
			t.Errorf("CheckURL(%q) was allowed", endpoint)
			continue
		}
		// A plain-HTTP endpoint elsewhere is a distinct refusal, so a caller
		// can tell "you typed a scheme I do not send over" from "that would
		// go across the network in the clear".
		if insecure != errors.Is(err, ErrInsecure) {
			t.Errorf("CheckURL(%q) = %v; ErrInsecure should be %v", endpoint, err, insecure)
		}
	}
}

// TestAPrivateAddressIsNotThisMachine: a LAN address is somewhere else, and
// traffic to it crosses a network other people are on.
func TestAPrivateAddressIsNotThisMachine(t *testing.T) {
	for _, host := range []string{"192.168.1.50", "10.0.0.5", "172.16.4.4", "example.com", ""} {
		if IsLoopback(host) {
			t.Errorf("IsLoopback(%q) = true", host)
		}
	}
	for _, host := range []string{"127.0.0.1", "127.1.2.3", "localhost", "::1", "[::1]"} {
		if !IsLoopback(host) {
			t.Errorf("IsLoopback(%q) = false", host)
		}
	}
}

func TestHostDropsThePathAndQuery(t *testing.T) {
	// A query string can carry a credential, and this is what gets logged.
	got, err := Host("https://api.example.com:8443/v1/chat?key=supersecret")
	if err != nil {
		t.Fatalf("Host: %v", err)
	}
	if got != "api.example.com:8443" {
		t.Errorf("Host = %q", got)
	}

	if _, err := Host("://not a url"); err == nil {
		t.Error("Host accepted something that is not a URL")
	}
}
