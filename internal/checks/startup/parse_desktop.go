package startup

import (
	"bufio"
	"bytes"
	"strings"
)

// parseDesktopEntry reads the fields a freedesktop .desktop autostart file
// needs to be described, and reports whether the entry is actually enabled.
//
// Distributions disable an autostart entry by shipping a copy with
// Hidden=true, so a file's presence is not the same as a program that runs.
func parseDesktopEntry(data []byte) (name, command string, enabled bool) {
	enabled = true
	inDesktopEntry := true

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			// Only the [Desktop Entry] group describes the entry itself.
			inDesktopEntry = strings.EqualFold(line, "[Desktop Entry]")
			continue
		}
		if !inDesktopEntry {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "Name":
			if name == "" {
				name = value
			}
		case "Exec":
			if command == "" {
				command = value
			}
		case "Hidden", "NoDisplay":
			if strings.EqualFold(value, "true") {
				enabled = false
			}
		case "X-GNOME-Autostart-enabled":
			if strings.EqualFold(value, "false") {
				enabled = false
			}
		}
	}
	return name, command, enabled
}

// labelFromPlistName turns a launch agent's filename into the label macOS uses
// for it: com.example.updater.plist becomes com.example.updater.
func labelFromPlistName(filename string) string {
	return strings.TrimSuffix(filename, ".plist")
}
