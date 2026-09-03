package startup

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// launchDirs are the locations macOS starts things from. The check reports
// what is configured; it does not open the property lists to guess at what each
// one runs, because the label is what a technician needs to identify it.
var launchDirs = []struct {
	path  string
	scope string
}{
	{"/Library/LaunchAgents", scopeSystem},
	{"/Library/LaunchDaemons", scopeSystem},
}

func collectStartupItems(_ context.Context, _ platform.Runner) ([]item, error) {
	dirs := make([]struct {
		path  string
		scope string
	}, len(launchDirs))
	copy(dirs, launchDirs)

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, struct {
			path  string
			scope string
		}{filepath.Join(home, "Library", "LaunchAgents"), scopeUser})
	}

	var out []item
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir.path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".plist") {
				continue
			}
			out = append(out, item{
				Name:     labelFromPlistName(entry.Name()),
				Location: filepath.Join(dir.path, entry.Name()),
				Scope:    dir.scope,
			})
		}
	}
	return out, nil
}
