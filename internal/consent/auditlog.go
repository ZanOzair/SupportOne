// Package consent holds the record of what SupportOne did and what the user
// agreed to.
//
// The audit log is append-only plain text the user can open in any editor. It
// records every check run, every fix applied or rolled back, every consent
// decision, and every byte sent off the machine. It records no secrets: field
// values are facts about actions, never credentials or file contents.
package consent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// EventType names a kind of auditable action.
type EventType string

// The auditable actions SupportOne records.
const (
	EventAgentStart    EventType = "AGENT_START"
	EventAgentStop     EventType = "AGENT_STOP"
	EventCheckRun      EventType = "CHECK_RUN"
	EventConsentAsked  EventType = "CONSENT_ASKED"
	EventConsentGiven  EventType = "CONSENT_GIVEN"
	EventConsentDenied EventType = "CONSENT_DENIED"
	EventFixPreflight  EventType = "FIX_PREFLIGHT"
	EventFixApplied    EventType = "FIX_APPLIED"
	EventFixRolledBack EventType = "FIX_ROLLED_BACK"
	EventDataSent      EventType = "DATA_SENT"
)

// Event is one line of the audit log.
type Event struct {
	// Type is what happened.
	Type EventType

	// Subject is the thing it happened to: a check ID, a fix ID, a
	// destination. Empty where there is no single subject.
	Subject string

	// Fields carry the specifics — severity, duration, byte counts. Keys and
	// values are written sorted so the log is stable and diffable. Never put
	// a credential, token or file content here.
	Fields map[string]string
}

// Log is an append-only audit log.
type Log struct {
	mu   sync.Mutex
	file *os.File
	path string

	// now is swappable in tests; production always uses time.Now.
	now func() time.Time
}

// Open opens (or creates) the audit log at path for appending. The file is
// created 0600: it is the user's record of their own machine, not a shared one.
func Open(path string) (*Log, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("consent: create audit log directory: %w", err)
	}
	// #nosec G304 -- the path is the user's own audit log location: a CLI
	// flag they passed or their config directory, never attacker-controlled.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("consent: open audit log: %w", err)
	}
	return &Log{file: f, path: path, now: time.Now}, nil
}

// DefaultPath returns the per-user audit log location for this OS.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("consent: locate user config directory: %w", err)
	}
	return filepath.Join(dir, "SupportOne", "audit.log"), nil
}

// Path returns the file the log is writing to, so the UI can offer to open it.
func (l *Log) Path() string { return l.path }

// Append writes one event. Every event is exactly one line: control characters
// in values are escaped so a value can never forge a second entry.
func (l *Log) Append(ev Event) error {
	line := l.format(ev)

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.file.WriteString(line); err != nil {
		return fmt.Errorf("consent: append audit entry: %w", err)
	}
	return l.file.Sync()
}

func (l *Log) format(ev Event) string {
	var b strings.Builder
	b.WriteString(l.now().UTC().Format(time.RFC3339))
	b.WriteByte('\t')
	b.WriteString(string(ev.Type))
	b.WriteByte('\t')
	b.WriteString(sanitize(ev.Subject))

	keys := make([]string, 0, len(ev.Fields))
	for k := range ev.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteByte('\t')
		b.WriteString(sanitize(k))
		b.WriteByte('=')
		b.WriteString(sanitize(ev.Fields[k]))
	}
	b.WriteByte('\n')
	return b.String()
}

// sanitize keeps one event on one line and strips control characters that
// would corrupt the log or a terminal reading it.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

// Close closes the underlying file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
