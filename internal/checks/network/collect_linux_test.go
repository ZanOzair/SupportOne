package network

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func fixtureRoots(t *testing.T, files map[string]string) {
	t.Helper()

	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	oldProc, oldEtc := procRoot, etcRoot
	procRoot, etcRoot = filepath.Join(root, "proc"), filepath.Join(root, "etc")
	t.Cleanup(func() { procRoot, etcRoot = oldProc, oldEtc })
}

const routeTable = "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
	"wlan0\t00000000\t0101A8C0\t0003\t0\t0\t600\t00000000\t0\t0\t0\n"

func TestCollectRoutingFromFixtureTree(t *testing.T) {
	fixtureRoots(t, map[string]string{
		"proc/net/route":  routeTable,
		"etc/resolv.conf": "nameserver 192.168.1.1\nnameserver 1.1.1.1\n",
	})

	route, err := collectRouting(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectRouting: %v", err)
	}
	if route.Gateway != "192.168.1.1" {
		t.Errorf("gateway = %q", route.Gateway)
	}
	if len(route.DNS) != 2 {
		t.Errorf("dns = %v, want both resolvers", route.DNS)
	}
}

func TestCollectRoutingReportsWhatItCouldNotRead(t *testing.T) {
	fixtureRoots(t, map[string]string{"proc/net/route": routeTable})

	route, err := collectRouting(context.Background(), nil)
	if err == nil {
		t.Fatal("collectRouting succeeded with no resolver configuration")
	}
	// The gateway it did read is still returned: a partial answer beats none.
	if route.Gateway != "192.168.1.1" {
		t.Errorf("gateway = %q, want the part that was readable", route.Gateway)
	}
}
