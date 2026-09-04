// Package fleet is the optional, self-hostable side of SupportOne: somewhere a
// technician can see the machines they look after, from the reports those
// machines chose to send.
//
// Two things shape it. It stores files, not rows — a technician who wants to
// run this should not also have to run a database, and `compose up` should be
// the whole of the setup. And it holds only what an agent sent, which is only
// what a person at that machine agreed to send: there is no polling, no agent
// installed as a service, and no way for the server to ask a machine anything.
// The arrow points one way.
package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

// Schema is the version of a stored record.
const Schema = 1

// Limits on what one machine may store.
const (
	// MaxNameLength bounds the label a machine reports itself under.
	MaxNameLength = 64

	// MaxHistory is how many reports are kept per machine. A fleet view is
	// about what is true now and what changed recently, not an archive.
	MaxHistory = 20
)

// ErrNotFound is returned for a machine the store does not hold.
var ErrNotFound = errors.New("fleet: no such machine")

// Report is what an agent sends. It carries the snapshot the user reviewed and
// nothing the server asked for.
type Report struct {
	// Name is the label the person at that machine chose. The server never
	// derives an identity from anything it was not given: what a machine is
	// called in someone else's dashboard is that person's decision.
	Name string `json:"name"`

	Snapshot checks.Snapshot `json:"snapshot"`

	// Redacted says whether the sender stripped identifying detail, so a
	// technician knows whether the blanks are deliberate.
	Redacted bool `json:"redacted"`

	// AgentVersion is the version that produced it.
	AgentVersion string `json:"agent_version"`
}

// Validate reports whether this is something the store can hold.
func (r Report) Validate() error {
	name := strings.TrimSpace(r.Name)
	switch {
	case name == "":
		return fmt.Errorf("fleet: a report must say which machine it is from")
	case len(name) > MaxNameLength:
		return fmt.Errorf("fleet: the machine name is %d characters, and %d is the limit", len(name), MaxNameLength)
	case r.Snapshot.Schema == 0:
		return fmt.Errorf("fleet: the report carries no snapshot")
	case len(r.Snapshot.Results) == 0:
		return fmt.Errorf("fleet: the report carries no check results")
	}
	return nil
}

// Entry is one stored report, with the time the server received it.
type Entry struct {
	Received time.Time       `json:"received"`
	Snapshot checks.Snapshot `json:"snapshot"`
	Redacted bool            `json:"redacted"`
}

// Machine is everything the store holds about one machine.
type Machine struct {
	Schema int    `json:"schema"`
	ID     string `json:"id"`
	Name   string `json:"name"`

	OS   string `json:"os"`
	Arch string `json:"arch"`

	AgentVersion string    `json:"agent_version"`
	LastSeen     time.Time `json:"last_seen"`

	// History is newest first, capped at MaxHistory.
	History []Entry `json:"history"`
}

// Latest returns the most recent report, and false when there is none.
func (m Machine) Latest() (Entry, bool) {
	if len(m.History) == 0 {
		return Entry{}, false
	}
	return m.History[0], true
}

// Counts returns the severity tally of the latest report.
func (m Machine) Counts() map[checks.Severity]int {
	latest, ok := m.Latest()
	if !ok {
		return nil
	}
	return latest.Snapshot.Counts()
}

// Store keeps machines as files under a directory.
//
// One file per machine, written to a temporary name and renamed into place, so
// a reader never sees a half-written record and a crash mid-write cannot
// destroy the previous one.
type Store struct {
	dir string

	// mu serialises writes. The server is one process; a lock here is enough
	// and is simpler to reason about than file locking.
	mu sync.Mutex
}

// OpenStore prepares a store at dir, creating it if it is not there.
func OpenStore(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("fleet: a data directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fleet: create the data directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns where the store keeps its files.
func (s *Store) Dir() string { return s.dir }

// Put files a report, merging it into that machine's history.
func (s *Store) Put(report Report, received time.Time) (Machine, error) {
	if err := report.Validate(); err != nil {
		return Machine{}, err
	}

	id := MachineID(report.Name)

	s.mu.Lock()
	defer s.mu.Unlock()

	machine, err := s.read(id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Machine{}, err
	}

	machine.Schema = Schema
	machine.ID = id
	machine.Name = strings.TrimSpace(report.Name)
	machine.OS = string(report.Snapshot.Host.OS)
	machine.Arch = report.Snapshot.Host.Arch
	machine.AgentVersion = report.AgentVersion
	machine.LastSeen = received.UTC()

	entry := Entry{Received: received.UTC(), Snapshot: report.Snapshot, Redacted: report.Redacted}
	machine.History = append([]Entry{entry}, machine.History...)
	if len(machine.History) > MaxHistory {
		machine.History = machine.History[:MaxHistory]
	}

	if err := s.write(machine); err != nil {
		return Machine{}, err
	}
	return machine, nil
}

// Get returns one machine.
func (s *Store) Get(id string) (Machine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

// List returns every machine, worst first: the one a technician should look at
// is the one at the top, and within a severity the most recently seen.
func (s *Store) List() ([]Machine, error) {
	names, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("fleet: list the data directory: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Machine, 0, len(names))
	for _, name := range names {
		id := strings.TrimSuffix(filepath.Base(name), ".json")
		machine, err := s.read(id)
		if err != nil {
			// One unreadable record does not hide the rest of the fleet.
			continue
		}
		out = append(out, machine)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Counts(), out[j].Counts()
		if a[checks.SeverityUrgent] != b[checks.SeverityUrgent] {
			return a[checks.SeverityUrgent] > b[checks.SeverityUrgent]
		}
		if a[checks.SeverityAttention] != b[checks.SeverityAttention] {
			return a[checks.SeverityAttention] > b[checks.SeverityAttention]
		}
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out, nil
}

// read loads one machine. It is called with the lock held.
func (s *Store) read(id string) (Machine, error) {
	path, err := s.path(id)
	if err != nil {
		return Machine{}, err
	}

	// #nosec G304 -- the path is built from a validated ID under the store's
	// own directory; see path.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Machine{}, ErrNotFound
	}
	if err != nil {
		return Machine{}, fmt.Errorf("fleet: read machine: %w", err)
	}

	var machine Machine
	if err := json.Unmarshal(data, &machine); err != nil {
		return Machine{}, fmt.Errorf("fleet: parse machine record: %w", err)
	}
	return machine, nil
}

// write saves one machine atomically. It is called with the lock held.
func (s *Store) write(machine Machine) error {
	path, err := s.path(machine.ID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(machine, "", "  ")
	if err != nil {
		return fmt.Errorf("fleet: encode machine record: %w", err)
	}

	// Write beside the target and rename over it: a reader never sees a
	// half-written record, and a crash cannot destroy the previous one.
	temp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("fleet: create a temporary file: %w", err)
	}
	tempName := temp.Name()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return fmt.Errorf("fleet: write machine record: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("fleet: finish writing machine record: %w", err)
	}
	if err := os.Chmod(tempName, 0o600); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("fleet: set permissions on machine record: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("fleet: replace machine record: %w", err)
	}
	return nil
}

// path builds the file for an ID, refusing anything that is not one.
//
// IDs are produced by MachineID and are hex, so this can never be a path from
// somewhere else. The check is here anyway, because the ID reaches this
// function from a URL.
func (s *Store) path(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("fleet: %q is not a machine identifier", id)
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func validID(id string) bool {
	if len(id) != idLength {
		return false
	}
	for _, r := range id {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// idLength is how much of the digest an ID keeps. 32 hex characters is 128
// bits, which is far more than enough to keep every machine in any fleet
// distinct without carrying the name itself in the URL.
const idLength = 32

// MachineID derives a stable identifier from the name a machine reports.
//
// The same name always maps to the same record, so a machine that reports
// twice updates rather than duplicating. It is a digest rather than the name
// itself so the name does not end up in a URL, a log line, or a file listing
// on the server.
func MachineID(name string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name))))
	return hex.EncodeToString(sum[:])[:idLength]
}

// sortStable is sort.SliceStable over a typed slice, so server.go does not
// reach for reflection at every call site.
func sortStable[T any](items []T, less func(a, b T) bool) {
	sort.SliceStable(items, func(i, j int) bool { return less(items[i], items[j]) })
}
