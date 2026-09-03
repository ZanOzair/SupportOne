package events

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks/cim"
)

// maxMessageLength keeps a single log line from dominating a report. The full
// text stays in the OS log, which the technician can open.
const maxMessageLength = 300

// winEvent mirrors the fields the Windows query selects from Get-WinEvent.
type winEvent struct {
	TimeCreated      string  `json:"TimeCreated"`
	ID               cim.Int `json:"Id"`
	ProviderName     string  `json:"ProviderName"`
	LevelDisplayName string  `json:"LevelDisplayName"`
	Message          string  `json:"Message"`
}

func parseWindowsEvents(data []byte) ([]logEvent, error) {
	entries, err := cim.Unmarshal[winEvent](data)
	if err != nil {
		return nil, err
	}

	out := make([]logEvent, 0, len(entries))
	for _, e := range entries {
		out = append(out, logEvent{
			Time:    cim.ParseTime(e.TimeCreated),
			Source:  strings.TrimSpace(e.ProviderName),
			ID:      strconv.Itoa(int(e.ID)),
			Level:   normaliseWindowsLevel(e.LevelDisplayName),
			Message: truncate(e.Message),
		})
	}
	return out, nil
}

func normaliseWindowsLevel(name string) string {
	if strings.EqualFold(strings.TrimSpace(name), "critical") {
		return levelCritical
	}
	return levelError
}

// journalEntry mirrors the fields journalctl's JSON output provides. Every
// value arrives as a string, including the priority and the timestamp.
type journalEntry struct {
	Timestamp  string `json:"__REALTIME_TIMESTAMP"`
	Identifier string `json:"SYSLOG_IDENTIFIER"`
	Unit       string `json:"_SYSTEMD_UNIT"`
	Comm       string `json:"_COMM"`
	Priority   string `json:"PRIORITY"`
	Message    string `json:"MESSAGE"`
}

// parseJournal reads journalctl's newline-delimited JSON. A malformed line is
// skipped rather than failing the whole check.
func parseJournal(data []byte) []logEvent {
	var out []logEvent

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var entry journalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		event := logEvent{
			Source:  firstNonEmpty(entry.Identifier, entry.Unit, entry.Comm, "system"),
			Level:   normaliseSyslogPriority(entry.Priority),
			Message: truncate(entry.Message),
		}
		// __REALTIME_TIMESTAMP is microseconds since the epoch.
		if micros, err := strconv.ParseInt(entry.Timestamp, 10, 64); err == nil {
			event.Time = time.UnixMicro(micros).UTC()
		}
		out = append(out, event)
	}
	return out
}

// normaliseSyslogPriority maps syslog severities onto the two levels this
// check distinguishes: 0 (emergency), 1 (alert) and 2 (critical) are critical,
// everything the query returned below that is an error.
func normaliseSyslogPriority(priority string) string {
	value, err := strconv.Atoi(strings.TrimSpace(priority))
	if err != nil {
		return levelError
	}
	if value <= 2 {
		return levelCritical
	}
	return levelError
}

// macLogEntry mirrors the fields `log show --style ndjson` provides.
type macLogEntry struct {
	Timestamp    string `json:"timestamp"`
	Subsystem    string `json:"subsystem"`
	Process      string `json:"processImagePath"`
	Sender       string `json:"senderImagePath"`
	EventMessage string `json:"eventMessage"`
	MessageType  string `json:"messageType"`
}

// parseMacLog reads the unified log's newline-delimited JSON.
func parseMacLog(data []byte) []logEvent {
	var out []logEvent

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		// The stream is bracketed by a JSON array and a trailing summary
		// object on some macOS versions; both are skipped.
		if len(line) == 0 || line[0] != '{' {
			continue
		}

		var entry macLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.EventMessage == "" && entry.Subsystem == "" {
			continue
		}

		out = append(out, logEvent{
			Time:    parseMacLogTime(entry.Timestamp),
			Source:  firstNonEmpty(entry.Subsystem, baseName(entry.Process), baseName(entry.Sender), "system"),
			Level:   normaliseMacMessageType(entry.MessageType),
			Message: truncate(entry.EventMessage),
		})
	}
	return out
}

// parseMacLogTime reads the "2026-09-02 13:20:00.123456+0800" form the unified
// log prints.
func parseMacLogTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999-0700", "2006-01-02 15:04:05-0700", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func normaliseMacMessageType(messageType string) string {
	if strings.EqualFold(strings.TrimSpace(messageType), "fault") {
		return levelCritical
	}
	return levelError
}

func baseName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

func truncate(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= maxMessageLength {
		return s
	}
	return s[:maxMessageLength] + "…"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
