package network

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Compiled-in query; see platform.RunRead.
const (
	psExe = "powershell"

	queryRouting = `$c = Get-NetIPConfiguration | Where-Object { $_.IPv4DefaultGateway -ne $null } | Select-Object -First 1; ` +
		`[pscustomobject]@{Gateway=$c.IPv4DefaultGateway.NextHop; Dns=@($c.DNSServer.ServerAddresses)} | ConvertTo-Json -Compress`
)

func collectRouting(ctx context.Context, run platform.Runner) (routing, error) {
	out, err := run(ctx, psExe, "-NoProfile", "-NonInteractive", "-Command", queryRouting)
	if err != nil {
		return routing{}, err
	}
	return parseWindowsRouting(out)
}
