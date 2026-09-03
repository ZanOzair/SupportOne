package startup

import "testing"

func TestParseDesktopEntry(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantName    string
		wantCommand string
		wantEnabled bool
	}{
		{
			name: "enabled entry",
			fixture: `[Desktop Entry]
Type=Application
Name=Nextcloud
Exec=/usr/bin/nextcloud --background
`,
			wantName:    "Nextcloud",
			wantCommand: "/usr/bin/nextcloud --background",
			wantEnabled: true,
		},
		{
			name: "hidden entry",
			fixture: `[Desktop Entry]
Name=Disabled Thing
Exec=/usr/bin/thing
Hidden=true
`,
			wantName:    "Disabled Thing",
			wantCommand: "/usr/bin/thing",
			wantEnabled: false,
		},
		{
			name: "gnome-disabled entry",
			fixture: `[Desktop Entry]
Name=Gnome Thing
Exec=/usr/bin/gthing
X-GNOME-Autostart-enabled=false
`,
			wantName:    "Gnome Thing",
			wantCommand: "/usr/bin/gthing",
			wantEnabled: false,
		},
		{
			name: "fields outside the Desktop Entry group are ignored",
			fixture: `[Desktop Entry]
Name=Real Name
Exec=/usr/bin/real

[Desktop Action Other]
Name=Action Name
Exec=/usr/bin/other
`,
			wantName:    "Real Name",
			wantCommand: "/usr/bin/real",
			wantEnabled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, command, enabled := parseDesktopEntry([]byte(tc.fixture))
			if name != tc.wantName || command != tc.wantCommand || enabled != tc.wantEnabled {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)",
					name, command, enabled, tc.wantName, tc.wantCommand, tc.wantEnabled)
			}
		})
	}
}

func TestParseWindowsStartup(t *testing.T) {
	fixture := []byte(`[{"Name":"OneDrive","Command":"C:\\Program Files\\OneDrive.exe /background",` +
		`"Location":"HKU\\...\\Run","User":"HOSTNAME\\alex"},` +
		`{"Name":"Updater","Command":"C:\\updater.exe","Location":"Common Startup","User":"Public"}]`)

	items, err := parseWindowsStartup(fixture)
	if err != nil {
		t.Fatalf("parseWindowsStartup: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Scope != scopeUser {
		t.Errorf("%s scope = %q, want user", items[0].Name, items[0].Scope)
	}
	if items[1].Scope != scopeSystem {
		t.Errorf("%s scope = %q, want system", items[1].Name, items[1].Scope)
	}
}

func TestLabelFromPlistName(t *testing.T) {
	if got := labelFromPlistName("com.example.updater.plist"); got != "com.example.updater" {
		t.Errorf("labelFromPlistName = %q, want com.example.updater", got)
	}
}
