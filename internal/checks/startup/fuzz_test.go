package startup

import "testing"

// .desktop files are written by anything a user installs, and Windows startup
// entries carry command lines with whatever quoting their author felt like.

func FuzzParseDesktopEntry(f *testing.F) {
	f.Add([]byte("[Desktop Entry]\nName=Thing\nExec=/usr/bin/thing\nHidden=false\n"))
	f.Add([]byte("[Desktop Entry]\nName="))
	f.Add([]byte("Name=\x00\xff"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = parseDesktopEntry(data)
	})
}

func FuzzParseWindowsStartup(f *testing.F) {
	f.Add([]byte(`[{"Name":"Thing","Command":"C:\\thing.exe","Location":"HKLM"}]`))
	f.Add([]byte(`[{"Name":`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWindowsStartup(data)
	})
}
