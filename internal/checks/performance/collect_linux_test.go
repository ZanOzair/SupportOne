package performance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// procTree writes a recorded /proc and points the collector at it, so the
// Linux path is exercised against known numbers rather than whatever the
// machine running the tests happens to be doing.
func procTree(t *testing.T, files map[string]string) {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	previous := procRoot
	procRoot = dir
	t.Cleanup(func() { procRoot = previous })
}

func TestCollectLoadReadsProc(t *testing.T) {
	procTree(t, map[string]string{
		"loadavg": "3.25 2.80 2.10 4/1234 5678\n",
		"meminfo": `MemTotal:       16324812 kB
MemFree:          512344 kB
MemAvailable:    9218044 kB
SwapTotal:       2097152 kB
SwapFree:        1048576 kB
`,
	})

	got, err := collectLoad(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectLoad: %v", err)
	}
	if !got.HasLoadAverage || got.LoadAverage != 3.25 {
		t.Errorf("LoadAverage = %v (present %v), want 3.25", got.LoadAverage, got.HasLoadAverage)
	}
	if got.MemTotalBytes != 16324812*1024 {
		t.Errorf("MemTotalBytes = %d", got.MemTotalBytes)
	}
	if got.MemAvailableBytes != 9218044*1024 {
		t.Errorf("MemAvailableBytes = %d", got.MemAvailableBytes)
	}
	if got.SwapUsedBytes != (2097152-1048576)*1024 {
		t.Errorf("SwapUsedBytes = %d", got.SwapUsedBytes)
	}
	// Linux keeps no instantaneous percentage, so it must stay absent.
	if got.HasBusy {
		t.Error("HasBusy = true on Linux")
	}
}

func TestCollectLoadFallsBackToMemFree(t *testing.T) {
	// Kernels before 3.14 have no MemAvailable. MemFree is a worse answer,
	// but it is an answer, and it is better than reporting nothing.
	procTree(t, map[string]string{
		"loadavg": "0.10 0.20 0.30 1/100 200\n",
		"meminfo": "MemTotal:       8000000 kB\nMemFree:        2000000 kB\n",
	})

	got, err := collectLoad(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectLoad: %v", err)
	}
	if got.MemAvailableBytes != 2000000*1024 {
		t.Errorf("MemAvailableBytes = %d, want the MemFree fallback", got.MemAvailableBytes)
	}
}

func TestCollectLoadFailsRatherThanGuessing(t *testing.T) {
	procTree(t, map[string]string{"loadavg": "0.1 0.2 0.3 1/1 1\n"})

	if _, err := collectLoad(context.Background(), nil); err == nil {
		t.Error("collectLoad succeeded with no meminfo to read")
	}

	procTree(t, map[string]string{"meminfo": "MemTotal: 100 kB\n"})
	if _, err := collectLoad(context.Background(), nil); err == nil {
		t.Error("collectLoad succeeded with no loadavg to read")
	}
}

func TestNoSwapConfiguredReportsZero(t *testing.T) {
	procTree(t, map[string]string{
		"loadavg": "0.10 0.20 0.30 1/100 200\n",
		"meminfo": "MemTotal: 8000000 kB\nMemAvailable: 4000000 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n",
	})

	got, err := collectLoad(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectLoad: %v", err)
	}
	if got.SwapTotalBytes != 0 || got.SwapUsedBytes != 0 {
		t.Errorf("swap = %d/%d, want zero", got.SwapUsedBytes, got.SwapTotalBytes)
	}
}
