package network

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

const (
	routeExe  = "route"
	scutilExe = "scutil"
)

func collectRouting(ctx context.Context, run platform.Runner) (routing, error) {
	var out routing

	// `route -n get default` exits non-zero when there is no default route,
	// which is a finding rather than a failure.
	if gateway, err := run(ctx, routeExe, "-n", "get", "default"); err == nil {
		out.Gateway = parseRouteGet(gateway)
	}

	resolvers, err := run(ctx, scutilExe, "--dns")
	if err != nil {
		return out, err
	}
	out.DNS = parseSCUtilDNS(resolvers)
	return out, nil
}
