// Package egress holds the one rule about where this agent may connect.
//
// SupportOne makes no outbound connection unless a person asked for a specific
// one. There are exactly two things that can ask — the optional assistant and
// the optional fleet report — and both come through here, so the rule is
// written once and cannot drift between them.
package egress

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrInsecure means the endpoint would put the payload on the wire in the
// clear to somewhere other than this machine.
var ErrInsecure = errors.New("egress: an endpoint that is not HTTPS is only allowed on this computer")

// CheckURL refuses an endpoint that would send in the clear.
//
// HTTPS is required everywhere except this machine. Plain HTTP on loopback is
// the common, sensible case — a local model server, or a fleet server being
// tried out on the same box — and the traffic never leaves the computer.
// Refusing it would push people towards a hosted service instead, which is the
// opposite of what any of this is for.
func CheckURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("egress: no address is configured")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("egress: %q is not a URL: %w", raw, err)
	}
	if parsed.Host == "" {
		return fmt.Errorf("egress: %q names no host", raw)
	}

	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if IsLoopback(parsed.Hostname()) {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrInsecure, parsed.Host)
	default:
		return fmt.Errorf("egress: %q is not a scheme this sends over", parsed.Scheme)
	}
}

// IsLoopback reports whether a host names this machine.
func IsLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// Host returns the host and port of an endpoint, without the path or query.
//
// This is what gets logged and shown. A full URL can carry a credential in its
// query, and an error or an audit line that quoted one would put it somewhere
// it was never meant to be.
func Host(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("egress: %q is not a URL: %w", raw, err)
	}
	return parsed.Host, nil
}
