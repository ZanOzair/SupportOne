package updates

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ZanOzair/supportone/internal/platform"
)

// runnerFor answers each command name with recorded output, and reports every
// other tool as not installed — which is what a machine without it does.
func runnerFor(responses map[string]string) platform.Runner {
	return func(_ context.Context, name string, _ ...string) ([]byte, error) {
		out, ok := responses[name]
		if !ok {
			return nil, fmt.Errorf("%w: %s", platform.ErrToolMissing, name)
		}
		return []byte(out), nil
	}
}

func TestCollectUpdatesPrefersAptWhereItExists(t *testing.T) {
	facts, err := collectUpdates(context.Background(), runnerFor(map[string]string{
		aptGetExe: "Inst curl [8.5.0-2ubuntu1] (8.5.0-2ubuntu2 Ubuntu:24.04/noble-updates [amd64])\n" +
			"Inst openssl [3.0.13] (3.0.13-0ubuntu3.1 Ubuntu:24.04/noble-updates [amd64])\n",
	}))
	if err != nil {
		t.Fatalf("collectUpdates: %v", err)
	}
	if facts.Pending != 2 {
		t.Errorf("pending = %d, want 2", facts.Pending)
	}
	if facts.Source != "apt package cache" {
		t.Errorf("source = %q, want the cache it actually read", facts.Source)
	}
}

func TestCollectUpdatesFallsBackToDnf(t *testing.T) {
	facts, err := collectUpdates(context.Background(), runnerFor(map[string]string{
		dnfExe: "Last metadata expiration check: 0:12:34 ago.\n\n" +
			"kernel.x86_64   6.9.7-200.fc40   updates\n",
	}))
	if err != nil {
		t.Fatalf("collectUpdates: %v", err)
	}
	if facts.Pending != 1 || facts.Source != "dnf package cache" {
		t.Errorf("facts = %+v, want one pending update from the dnf cache", facts)
	}
}

func TestCollectUpdatesWithNoPackageManagerSaysSo(t *testing.T) {
	_, err := collectUpdates(context.Background(), runnerFor(nil))
	if !errors.Is(err, platform.ErrToolMissing) {
		t.Errorf("err = %v, want the missing tool named", err)
	}
}

func TestCollectUpdatesMakesNoNetworkRequest(t *testing.T) {
	// The arguments are the guarantee: -s simulates against the local cache
	// and -C keeps dnf offline. A change that drops them would let the check
	// contact a mirror, which this agent must never do on its own.
	var aptArgs, dnfArgs []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case aptGetExe:
			aptArgs = args
			return []byte("Inst curl [1] (2 repo [amd64])\n"), nil
		case dnfExe:
			dnfArgs = args
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %s", platform.ErrToolMissing, name)
	}

	if _, err := collectUpdates(context.Background(), run); err != nil {
		t.Fatalf("collectUpdates: %v", err)
	}
	if len(aptArgs) == 0 || aptArgs[0] != "-s" {
		t.Errorf("apt args = %v, want a simulation that touches no mirror", aptArgs)
	}
	if len(dnfArgs) != 0 {
		t.Errorf("dnf ran as well: %v", dnfArgs)
	}
}
