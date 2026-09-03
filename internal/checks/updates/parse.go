package updates

import (
	"bufio"
	"bytes"
	"strings"
	"time"

	"github.com/ZanOzair/supportone/internal/checks/cim"
)

// countAptUpgrades counts the packages `apt-get -s upgrade` would install.
// The simulation reads the local package cache and makes no network request.
func countAptUpgrades(data []byte) int {
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "Inst ") {
			count++
		}
	}
	return count
}

// countDnfUpdates counts the packages `dnf -C check-update` lists. Its output
// starts with a blank line and optional "Obsoleting Packages" section; only the
// package lines before that are updates.
func countDnfUpdates(data []byte) int {
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(line, " ") {
			continue
		}
		if strings.HasPrefix(trimmed, "Obsoleting Packages") {
			break
		}
		if strings.HasPrefix(trimmed, "Last metadata expiration") {
			continue
		}
		// A package line is "name.arch  version  repository".
		if fields := strings.Fields(trimmed); len(fields) >= 3 && strings.Contains(fields[0], ".") {
			count++
		}
	}
	return count
}

// winUpdateResult mirrors the composite object the Windows query builds from
// the Windows Update install history and the hotfix list.
type winUpdateResult struct {
	LastSuccess string `json:"LastSuccess"`
	LastHotFix  string `json:"LastHotFix"`
}

// parseWindowsUpdates prefers the Windows Update agent's own record of its last
// successful install, falling back to the newest installed hotfix.
func parseWindowsUpdates(data []byte) (updateFacts, error) {
	entries, err := cim.Unmarshal[winUpdateResult](data)
	if err != nil {
		return updateFacts{}, err
	}
	facts := updateFacts{Pending: -1, Source: "Windows Update install history"}
	if len(entries) == 0 {
		return facts, nil
	}

	if t := parseWindowsUpdateTime(entries[0].LastSuccess); !t.IsZero() {
		facts.LastInstalled = t
		return facts, nil
	}
	if t := cim.ParseTime(entries[0].LastHotFix); !t.IsZero() {
		facts.LastInstalled = t
		facts.Source = "installed hotfix list"
	}
	return facts, nil
}

// parseWindowsUpdateTime reads the registry's LastSuccessTime, which is written
// as "2026-08-14 03:11:52" in UTC, as well as the CIM date shapes.
func parseWindowsUpdateTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC()
	}
	return cim.ParseTime(s)
}

// parseMacSoftwareUpdateDate reads the date `defaults read` prints for
// LastFullSuccessfulDate: "2026-08-01 10:22:31 +0000".
func parseMacSoftwareUpdateDate(data []byte) time.Time {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05 -0700", time.RFC3339, "2006-01-02 15:04:05 Z0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
