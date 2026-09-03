package startup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

func writeDesktopEntry(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestCollectStartupItemsReadsBothScopes(t *testing.T) {
	home := t.TempDir()
	system := t.TempDir()
	t.Setenv("HOME", home)

	old := systemAutostartDirs
	systemAutostartDirs = []string{system}
	t.Cleanup(func() { systemAutostartDirs = old })

	writeDesktopEntry(t, system, "printer.desktop",
		"[Desktop Entry]\nName=Printer Applet\nExec=/usr/bin/printer-applet\n")
	writeDesktopEntry(t, system, "disabled.desktop",
		"[Desktop Entry]\nName=Disabled Thing\nExec=/usr/bin/thing\nHidden=true\n")
	writeDesktopEntry(t, filepath.Join(home, ".config", "autostart"), "sync.desktop",
		"[Desktop Entry]\nName=Cloud Sync\nExec=/usr/bin/sync --background\n")
	// A stray file that is not an autostart entry.
	writeDesktopEntry(t, system, "notes.txt", "not an entry")

	items, err := collectStartupItems(context.Background(), nil)
	if err != nil {
		t.Fatalf("collectStartupItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (a hidden entry does not run): %+v", len(items), items)
	}

	scopes := map[string]string{}
	for _, item := range items {
		scopes[item.Name] = item.Scope
	}
	if scopes["Printer Applet"] != scopeSystem {
		t.Errorf("system entry scope = %q", scopes["Printer Applet"])
	}
	if scopes["Cloud Sync"] != scopeUser {
		t.Errorf("user entry scope = %q", scopes["Cloud Sync"])
	}
}

func TestStartupCheckReportsAnInventoryNotAVerdict(t *testing.T) {
	system := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	old := systemAutostartDirs
	systemAutostartDirs = []string{system}
	t.Cleanup(func() { systemAutostartDirs = old })

	// Twenty startup programs is a lot, and it is still not a fault.
	for i := 0; i < 20; i++ {
		writeDesktopEntry(t, system, string(rune('a'+i))+".desktop",
			"[Desktop Entry]\nName=Program\nExec=/usr/bin/program\n")
	}

	res, err := itemsCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Severity != checks.SeverityOK {
		t.Errorf("severity = %q, want ok — a long list is not a problem to invent", res.Severity)
	}
	if res.Summary != keyStartupOK {
		t.Errorf("summary = %q", res.Summary)
	}
}

func TestStartupCheckWithNothingConfigured(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := systemAutostartDirs
	systemAutostartDirs = []string{t.TempDir()}
	t.Cleanup(func() { systemAutostartDirs = old })

	res, err := itemsCheck{}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Summary != keyStartupNone {
		t.Errorf("summary = %q, want the 'nothing starts on its own' message", res.Summary)
	}
}
