package network

import (
	"bufio"
	"bytes"
	"net"
	"strings"
)

// parseRouteGet reads the gateway from `route -n get default`, whose output is
// a block of "key: value" lines.
func parseRouteGet(data []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok || strings.TrimSpace(key) != "gateway" {
			continue
		}
		gateway := strings.TrimSpace(value)
		if net.ParseIP(gateway) != nil {
			return gateway
		}
	}
	return ""
}

// parseSCUtilDNS reads the "nameserver[n] : address" lines of `scutil --dns`,
// which lists each resolver once per scoped configuration.
func parseSCUtilDNS(data []byte) []string {
	var out []string
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "nameserver[") {
			continue
		}
		_, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		address := strings.TrimSpace(value)
		if net.ParseIP(address) == nil || seen[address] {
			continue
		}
		seen[address] = true
		out = append(out, address)
	}
	return out
}
