package startup

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ZanOzair/supportone/internal/platform"
)

// autostartDirs are the freedesktop locations that hold startup entries. The
// per-user directory is resolved at collection time.
var systemAutostartDirs = []string{"/etc/xdg/autostart"}

func collectStartupItems(_ context.Context, _ platform.Runner) ([]item, error) {
	var out []item

	dirs := map[string]string{}
	for _, dir := range systemAutostartDirs {
		dirs[dir] = scopeSystem
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs[filepath.Join(home, ".config", "autostart")] = scopeUser
	}

	paths := make([]string, 0, len(dirs))
	for dir := range dirs {
		paths = append(paths, dir)
	}
	sort.Strings(paths)

	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// A missing autostart directory is normal, not a failure.
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path) // #nosec G304 -- path comes from listing a compiled-in autostart directory.
			if err != nil {
				continue
			}

			name, command, enabled := parseDesktopEntry(data)
			if !enabled {
				continue
			}
			if name == "" {
				name = strings.TrimSuffix(entry.Name(), ".desktop")
			}
			out = append(out, item{
				Name:     name,
				Command:  command,
				Location: path,
				Scope:    dirs[dir],
			})
		}
	}
	return out, nil
}
