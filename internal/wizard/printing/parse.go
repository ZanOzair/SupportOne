package printing

import (
	"encoding/json"
	"strings"
)

// The two parsers below read Windows tool output and carry no build
// constraint, so that output can be tested on any machine — the same split the
// diagnostic checks use.

// serviceRunning reads the state out of `sc query spooler`, whose output looks
// like:
//
//	SERVICE_NAME: spooler
//	        STATE              : 4  RUNNING
//
// Anything that is not clearly running is treated as not running: a service in
// the middle of starting is not one that can print.
func serviceRunning(raw []byte) bool {
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(strings.ToUpper(line), "STATE") {
			continue
		}
		return strings.Contains(strings.ToUpper(line), "RUNNING")
	}
	return false
}

// defaultPrinterName reads the printer name out of the PowerShell response.
// An empty result means no default printer is set, which is a finding rather
// than an error.
func defaultPrinterName(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}

	// ConvertTo-Json gives an object for one printer and an array if the
	// filter ever matched more than one.
	var single struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil {
		return strings.TrimSpace(single.Name)
	}

	var many []struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal([]byte(trimmed), &many); err == nil && len(many) > 0 {
		return strings.TrimSpace(many[0].Name)
	}
	return ""
}
