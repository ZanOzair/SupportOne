package network

import (
	"net"
	"strings"

	"github.com/ZanOzair/supportone/internal/checks/cim"
)

// winRouting mirrors the composite object the Windows routing query builds
// from Get-NetIPConfiguration.
type winRouting struct {
	Gateway string   `json:"Gateway"`
	DNS     []string `json:"Dns"`
}

func parseWindowsRouting(data []byte) (routing, error) {
	entries, err := cim.Unmarshal[winRouting](data)
	if err != nil {
		return routing{}, err
	}
	if len(entries) == 0 {
		return routing{}, nil
	}

	entry := entries[0]
	out := routing{Gateway: strings.TrimSpace(entry.Gateway)}
	if net.ParseIP(out.Gateway) == nil {
		out.Gateway = ""
	}

	seen := make(map[string]bool)
	for _, server := range entry.DNS {
		server = strings.TrimSpace(server)
		if net.ParseIP(server) == nil || seen[server] {
			continue
		}
		seen[server] = true
		out.DNS = append(out.DNS, server)
	}
	return out, nil
}
