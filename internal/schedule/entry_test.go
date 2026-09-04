package schedule

import (
	"os"
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

func TestEntryForEveryTargetedPlatform(t *testing.T) {
	for _, os := range platform.All() {
		entry, err := EntryFor(os, "/opt/supportone/supportone-agent", "/srv/reports")
		if err != nil {
			t.Fatalf("EntryFor(%s): %v", os, err)
		}

		if entry.Mechanism == "" {
			t.Errorf("%s: the instruction does not name what it uses", os)
		}
		if entry.Command == "" {
			t.Errorf("%s: the instruction has nothing to paste", os)
		}
		// The undo is produced with the command rather than left to be
		// looked up a year later.
		if entry.Undo == "" {
			t.Errorf("%s: the instruction does not say how to remove it", os)
		}
		if !strings.Contains(entry.Command, "--monthly") {
			t.Errorf("%s: the command does not run the monthly report: %s", os, entry.Command)
		}
	}
}

func TestAnUnknownPlatformGetsNoInventedInstruction(t *testing.T) {
	if _, err := EntryFor(platform.OS("plan9"), "/bin/agent", "/srv/reports"); err == nil {
		t.Error("EntryFor invented an instruction for a platform it knows nothing about")
	}
}

func TestBothPathsAreRequired(t *testing.T) {
	for _, args := range [][2]string{{"", "/srv/reports"}, {"/bin/agent", ""}, {"  ", "  "}} {
		if _, err := EntryFor(platform.Linux, args[0], args[1]); err == nil {
			t.Errorf("EntryFor accepted %q and %q", args[0], args[1])
		}
	}
}

// TestAPathWithASpaceDoesNotBreakTheLine is the everyday case: a home
// directory routinely has a space in it, and a scheduler line that broke on
// one would be a line that quietly stopped running.
func TestAPathWithASpaceDoesNotBreakTheLine(t *testing.T) {
	binary := "/Users/Alex Smith/Applications/supportone-agent"
	dir := "/Users/Alex Smith/Documents/IT Reports"

	for _, os := range platform.All() {
		entry, err := EntryFor(os, binary, dir)
		if err != nil {
			t.Fatalf("EntryFor(%s): %v", os, err)
		}

		switch os {
		case platform.Linux:
			// Single-quoted for a shell.
			if !strings.Contains(entry.Command, "'"+binary+"'") {
				t.Errorf("linux: the path is not quoted: %s", entry.Command)
			}
		case platform.Windows:
			// Escaped double quotes inside the /TR argument.
			if !strings.Contains(entry.Command, `\"`+binary+`\"`) {
				t.Errorf("windows: the path is not quoted: %s", entry.Command)
			}
		case platform.Darwin:
			// Its own element in the argument array, so quoting is the
			// plist's job rather than a shell's.
			if !strings.Contains(entry.Command, "<string>"+binary+"</string>") {
				t.Errorf("darwin: the path is not its own argument: %s", entry.Command)
			}
		}
	}
}

func TestAShellQuotedPathSurvivesAnApostrophe(t *testing.T) {
	binary := "/home/o'brien/supportone-agent"

	entry, err := EntryFor(platform.Linux, binary, "/srv/reports")
	if err != nil {
		t.Fatalf("EntryFor: %v", err)
	}
	// The apostrophe is escaped rather than ending the quoted string early.
	if !strings.Contains(entry.Command, `'/home/o'\''brien/supportone-agent'`) {
		t.Errorf("the apostrophe was not escaped: %s", entry.Command)
	}
}

func TestAPlistEscapesWhatXMLNeedsEscaped(t *testing.T) {
	entry, err := EntryFor(platform.Darwin, `/Apps/Tom & Jerry/agent`, `/Reports/<client>`)
	if err != nil {
		t.Fatalf("EntryFor: %v", err)
	}

	if strings.Contains(entry.Command, "Tom & Jerry") {
		t.Errorf("an ampersand reached the plist unescaped: %s", entry.Command)
	}
	if strings.Contains(entry.Command, "<client>") {
		t.Errorf("angle brackets reached the plist unescaped: %s", entry.Command)
	}
	if !strings.Contains(entry.Command, "Tom &amp; Jerry") {
		t.Errorf("the ampersand was not escaped: %s", entry.Command)
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/bin/agent":      "'/bin/agent'",
		"/home/a b/agent": "'/home/a b/agent'",
		"/home/o'brien/x": `'/home/o'\''brien/x'`,
		"":                "''",
	}
	for input, want := range cases {
		if got := shellQuote(input); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestXMLEscape(t *testing.T) {
	got := xmlEscape(`a & b < c > d " e ' f`)
	for _, raw := range []string{"&amp;", "&lt;", "&gt;", "&quot;", "&apos;"} {
		if !strings.Contains(got, raw) {
			t.Errorf("xmlEscape did not produce %s: %q", raw, got)
		}
	}
}

// TestNothingHereInstallsAnything is a statement about the package's shape:
// it returns text, and there is no path from this call to a changed machine.
func TestNothingHereInstallsAnything(t *testing.T) {
	before := t.TempDir()

	if _, err := EntryFor(platform.Linux, "/bin/agent", before); err != nil {
		t.Fatalf("EntryFor: %v", err)
	}

	entries, err := readDir(before)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("EntryFor wrote %v", entries)
	}
}

func readDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}
