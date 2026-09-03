package network

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ZanOzair/supportone/internal/platform"
)

// Roots are variables so tests can point them at recorded trees.
var (
	procRoot = "/proc"
	etcRoot  = "/etc"
)

func collectRouting(_ context.Context, _ platform.Runner) (routing, error) {
	var out routing

	routes, err := os.ReadFile(filepath.Join(procRoot, "net/route"))
	if err != nil {
		return out, fmt.Errorf("network: read route table: %w", err)
	}
	out.Gateway = parseProcRoute(routes)

	// systemd-resolved machines point resolv.conf at a stub resolver; that is
	// still the resolver this machine uses, so it is what gets reported.
	resolv, err := os.ReadFile(filepath.Join(etcRoot, "resolv.conf"))
	if err != nil {
		return out, fmt.Errorf("network: read resolver configuration: %w", err)
	}
	out.DNS = parseResolvConf(resolv)
	return out, nil
}
