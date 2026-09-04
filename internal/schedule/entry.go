package schedule

import (
	"fmt"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Entry is a scheduler instruction for one platform: what to paste, where, and
// how to undo it.
//
// SupportOne prints this rather than installing it. A scheduled task is a
// change to a machine, and every change here goes through the consent gate as
// a fix with a rollback. It is also the version that survives someone asking,
// a year later, "what is this thing on my computer and how do I stop it?" —
// which a silently installed task does not.
type Entry struct {
	// Mechanism names what this uses, so the reader knows which part of their
	// system they are about to touch.
	Mechanism string `json:"mechanism"`

	// Command is what to paste. It is the whole thing, not a fragment.
	Command string `json:"command"`

	// Where says where it goes, for the mechanisms that need saying.
	Where string `json:"where,omitempty"`

	// Undo is how to remove it. It is produced with the command rather than
	// left to be looked up later.
	Undo string `json:"undo"`
}

// EntryFor builds the instruction for one platform.
//
// The binary path and the report folder are quoted rather than concatenated
// raw: a person's home directory routinely has a space in it, and a scheduler
// line that breaks on that would be a line that quietly stops running.
func EntryFor(os platform.OS, binary, reportDir string) (Entry, error) {
	binary = strings.TrimSpace(binary)
	reportDir = strings.TrimSpace(reportDir)

	if binary == "" || reportDir == "" {
		return Entry{}, fmt.Errorf("schedule: both the program's path and the report folder are needed")
	}

	switch os {
	case platform.Linux:
		return Entry{
			Mechanism: "cron",
			// 07:00 on the first of the month. A monthly report wants to
			// exist before the person who asked for it looks for it.
			Command: fmt.Sprintf(`0 7 1 * * %s --monthly %s`, shellQuote(binary), shellQuote(reportDir)),
			Where:   "crontab -e",
			Undo:    "Run crontab -e again and delete that line.",
		}, nil

	case platform.Darwin:
		return Entry{
			Mechanism: "launchd",
			Command:   launchAgent(binary, reportDir),
			Where:     "~/Library/LaunchAgents/com.supportone.monthly.plist, then: launchctl load ~/Library/LaunchAgents/com.supportone.monthly.plist",
			Undo:      "launchctl unload ~/Library/LaunchAgents/com.supportone.monthly.plist, then delete that file.",
		}, nil

	case platform.Windows:
		return Entry{
			Mechanism: "Task Scheduler",
			Command: fmt.Sprintf(
				`schtasks /Create /TN "SupportOne monthly report" /SC MONTHLY /D 1 /ST 07:00 /TR "\"%s\" --monthly \"%s\""`,
				binary, reportDir),
			Where: "a Command Prompt",
			Undo:  `schtasks /Delete /TN "SupportOne monthly report" /F`,
		}, nil

	default:
		return Entry{}, fmt.Errorf("schedule: no scheduler instruction is written for %s", os.Display())
	}
}

// launchAgent renders the plist macOS wants. It is a whole file rather than a
// command, which is why Where says to save it before loading it.
func launchAgent(binary, reportDir string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.supportone.monthly</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + xmlEscape(binary) + `</string>
    <string>--monthly</string>
    <string>` + xmlEscape(reportDir) + `</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Day</key><integer>1</integer>
    <key>Hour</key><integer>7</integer>
    <key>Minute</key><integer>0</integer>
  </dict>
  <key>RunAtLoad</key><false/>
</dict>
</plist>`
}

// shellQuote wraps a path in single quotes for a shell, escaping any single
// quote inside it. A home directory with an apostrophe in it is unusual and
// not impossible, and a line that broke on one would stop running silently.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// xmlEscape makes a path safe inside a plist element.
func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
