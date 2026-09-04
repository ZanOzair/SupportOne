// Package remote is the consent wrapper around a remote-help session.
//
// SupportOne implements no remote desktop protocol, and will not. Writing one
// would mean writing screen capture, input injection and transport encryption,
// which is three security-critical things done worse than the people who
// already do them. What this package adds is the part those tools mostly leave
// out: a moment where the person is told, in words, what they are about to let
// someone do, and a record afterwards of what they agreed to and when.
//
// # The honest limit
//
// Once a session starts, SupportOne can see nothing. It cannot watch what the
// technician does, cannot restrict it, and cannot end the session itself. Its
// record says a session was agreed at one time and marked ended at another —
// which is an account of a decision, not surveillance of a session. The record
// says so in those terms rather than implying more.
package remote

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// Message keys this package resolves through internal/i18n.
const (
	KeyHeading = "remote.heading"

	// The consequences, listed one at a time. A person agreeing to a remote
	// session is agreeing to all of these, so all of them are shown.
	KeyCanSeeScreen  = "remote.can.see_screen"
	KeyCanControl    = "remote.can.control"
	KeyCanReadFiles  = "remote.can.read_files"
	KeyCanActAsYou   = "remote.can.act_as_you"
	KeyCannotWatch   = "remote.cannot.watch"
	KeyEndIt         = "remote.end_it"
	KeyCloseAnything = "remote.close_anything"
)

// Consequences is the list every session is agreed against, in the order it
// should be read.
//
// It is deliberately blunt. "The technician can see your screen" is a sentence
// people skim; "they can read any file you can open" is one they stop at, and
// stopping at it is the point.
func Consequences() []string {
	return []string{
		KeyCanSeeScreen,
		KeyCanControl,
		KeyCanReadFiles,
		KeyCanActAsYou,
		KeyCannotWatch,
		KeyEndIt,
	}
}

// Errors callers distinguish between.
var (
	// ErrNotConfirmed means Start was called without a valid plan token, or
	// with an acknowledgement that did not repeat what was shown.
	ErrNotConfirmed = errors.New("remote: this session was not confirmed against what it allows")

	// ErrNoTechnician means nobody was named. A session record that does not
	// say who was let in is not a record.
	ErrNoTechnician = errors.New("remote: a session must name who is being let in")

	// ErrUnknownTool means the named tool is not in this build's whitelist.
	ErrUnknownTool = errors.New("remote: that is not a remote-help tool this build knows about")

	// ErrToolNotInstalled means the tool is one this build knows, but it was
	// not found on this machine. SupportOne does not offer to fetch it.
	ErrToolNotInstalled = errors.New("remote: that program is not installed on this machine")

	// ErrNoSession means End was called with no session running.
	ErrNoSession = errors.New("remote: no session is running")
)

// Tool is a remote-help program this build knows how to look for.
//
// The list is compiled in. Nothing here is assembled from user input, and
// SupportOne never installs, downloads or configures any of them: it looks for
// what is already on the machine and offers only that.
type Tool struct {
	// ID is stable and lowercase, e.g. "rustdesk".
	ID string `json:"id"`

	// Name is what the user sees on their own machine.
	Name string `json:"name"`

	// Installed says whether it was found.
	Installed bool `json:"installed"`

	// Path is where it was found, for display. Empty when it was not.
	Path string `json:"path,omitempty"`
}

// Plan is what the user is shown before a session.
type Plan struct {
	// Technician is who is being let in, as the user named them.
	Technician string `json:"technician"`

	Tool Tool `json:"tool"`

	// Consequences is the exact list this session is agreed against.
	Consequences []string `json:"consequences"`

	// Token proves this plan was shown. Start will not act without it.
	Token string `json:"token"`
}

// Confirmation is the user's decision, carrying back what they were shown.
type Confirmation struct {
	Token string `json:"token"`

	// Acknowledged must repeat the plan's consequences. A caller that cannot
	// reproduce the list did not display it.
	Acknowledged []string `json:"acknowledged"`
}

// Session is a running session as SupportOne understands it — which is to say,
// as the user reported it.
type Session struct {
	ID         string    `json:"id"`
	Technician string    `json:"technician"`
	Tool       string    `json:"tool"`
	Started    time.Time `json:"started"`

	// Ended is when the user marked it over. Zero while it is open.
	Ended time.Time `json:"ended,omitempty"`

	// Launched says whether SupportOne started the tool, or the user did.
	Launched bool `json:"launched"`
}

// MarshalJSON omits Ended entirely while the session is open.
//
// A zero time.Time renders as "0001-01-01T00:00:00Z", which a client reading
// the field would take for an end time. An open session has no end, and the
// JSON should say that by saying nothing.
func (s Session) MarshalJSON() ([]byte, error) {
	type alias Session
	out := struct {
		alias
		Ended *time.Time `json:"ended,omitempty"`
	}{alias: alias(s)}

	if !s.Ended.IsZero() {
		ended := s.Ended
		out.Ended = &ended
	}
	return json.Marshal(out)
}

// Duration returns how long the session was agreed to be running.
func (s Session) Duration() time.Duration {
	if s.Ended.IsZero() {
		return 0
	}
	return s.Ended.Sub(s.Started)
}

// Wrapper holds the consent gate for remote sessions.
type Wrapper struct {
	audit *consent.Log
	os    platform.OS

	// lookPath finds a tool. Swappable in tests.
	lookPath func(string) (string, error)

	// start launches a tool. Swappable in tests; production runs the
	// compiled-in path and nothing else.
	start func(ctx context.Context, path string) error

	// now is swappable in tests.
	now func() time.Time

	mu sync.Mutex

	// pending is the plan most recently shown, and the only one Start will
	// accept. Keeping one rather than a map means a plan cannot outlive the
	// screen that displayed it, and repeatedly asking what a session allows
	// cannot grow anything without bound.
	pending *Plan

	current *Session
}

// New returns a wrapper.
func New(audit *consent.Log, os platform.OS) *Wrapper {
	return &Wrapper{
		audit:    audit,
		os:       os,
		lookPath: look,
		start:    launch,
		now:      time.Now,
	}
}

// Tools reports which remote-help programs are on this machine.
//
// It looks and does not install. A tool that is not there is reported as not
// there; offering to fetch one would be downloading and running code, which is
// the one thing this project does not do.
func (w *Wrapper) Tools() []Tool {
	known := knownTools[w.os]

	out := make([]Tool, 0, len(known))
	for _, t := range known {
		tool := Tool{ID: t.id, Name: t.name}
		for _, candidate := range t.commands {
			if path, err := w.lookPath(candidate); err == nil {
				tool.Installed, tool.Path = true, path
				break
			}
		}
		out = append(out, tool)
	}
	return out
}

// Plan describes a session before it starts.
//
// The tool may be empty: someone connecting by a means this build does not
// know about still deserves the consent record, and refusing to make one would
// only mean the session happened without it.
func (w *Wrapper) Plan(technician, toolID string) (Plan, error) {
	technician = strings.TrimSpace(technician)
	if technician == "" {
		return Plan{}, ErrNoTechnician
	}

	plan := Plan{
		Technician:   technician,
		Consequences: Consequences(),
	}

	if toolID = strings.TrimSpace(toolID); toolID != "" {
		found, known := w.tool(toolID)
		switch {
		case !known:
			return Plan{}, fmt.Errorf("%w: %q", ErrUnknownTool, toolID)
		case !found.Installed:
			return Plan{}, fmt.Errorf("%w: %s", ErrToolNotInstalled, found.Name)
		}
		plan.Tool = found
	}

	token, err := newToken()
	if err != nil {
		return Plan{}, err
	}
	plan.Token = token

	w.mu.Lock()
	w.pending = &plan
	w.mu.Unlock()

	w.record(consent.EventConsentAsked, "remote session", map[string]string{
		"technician": technician,
		"tool":       plan.Tool.ID,
	})
	return plan, nil
}

// Start records a session the user confirmed, and launches the tool if one was
// named and found.
func (w *Wrapper) Start(ctx context.Context, c Confirmation) (Session, error) {
	w.mu.Lock()
	pending := w.pending
	ok := pending != nil && subtle.ConstantTimeCompare([]byte(pending.Token), []byte(c.Token)) == 1
	if ok {
		// A plan is good for one session. Reusing a token would let a second
		// session ride on a confirmation given for the first.
		w.pending = nil
	}
	running := w.current
	w.mu.Unlock()

	if !ok {
		return Session{}, ErrNotConfirmed
	}
	plan := *pending
	if !acknowledges(plan.Consequences, c.Acknowledged) {
		w.record(consent.EventConsentDenied, "remote session", map[string]string{
			"reason": "acknowledgement did not match what was shown",
		})
		return Session{}, ErrNotConfirmed
	}
	if running != nil && running.Ended.IsZero() {
		return Session{}, fmt.Errorf("remote: a session with %s is already open; end it first", running.Technician)
	}

	id, err := newToken()
	if err != nil {
		return Session{}, err
	}

	session := Session{
		ID:         id,
		Technician: plan.Technician,
		Tool:       plan.Tool.ID,
		Started:    w.now().UTC(),
	}

	fields := map[string]string{
		"technician": session.Technician,
		"tool":       session.Tool,
	}

	if plan.Tool.Installed && plan.Tool.Path != "" {
		if err := w.start(ctx, plan.Tool.Path); err != nil {
			// Failing to launch is not failing to consent. The session is
			// still recorded, because the user may well start the tool
			// themselves and the record should cover that.
			fields["reason"] = "the program could not be started"
		} else {
			session.Launched = true
		}
	}
	fields["launched"] = fmt.Sprint(session.Launched)

	w.mu.Lock()
	w.current = &session
	w.mu.Unlock()

	w.record(consent.EventConsentGiven, "remote session", map[string]string{
		"technician": session.Technician,
		"tool":       session.Tool,
	})
	w.record(consent.EventRemoteStarted, "remote session", fields)
	return session, nil
}

// Decline discards a plan the user refused, and records the refusal.
//
// A log that shows a question asked and then nothing is a worse record than
// one that says the answer was no.
func (w *Wrapper) Decline() {
	w.mu.Lock()
	pending := w.pending
	w.pending = nil
	w.mu.Unlock()

	if pending == nil {
		return
	}
	w.record(consent.EventConsentDenied, "remote session", map[string]string{
		"technician": pending.Technician,
		"tool":       pending.Tool.ID,
	})
}

// End marks the session over.
//
// It ends the record, not the session: SupportOne cannot close someone else's
// connection. The record says the user marked it ended, and the interface tells
// them to close the tool itself.
func (w *Wrapper) End() (Session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.current == nil || !w.current.Ended.IsZero() {
		return Session{}, ErrNoSession
	}

	w.current.Ended = w.now().UTC()
	session := *w.current

	w.record(consent.EventRemoteEnded, "remote session", map[string]string{
		"technician": session.Technician,
		"duration":   session.Duration().Round(time.Second).String(),
	})
	return session, nil
}

// Current returns the open session, if there is one.
func (w *Wrapper) Current() (Session, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.current == nil || !w.current.Ended.IsZero() {
		return Session{}, false
	}
	return *w.current, true
}

// tool finds one whitelisted tool by ID, reporting whether this build knows
// the ID at all. Whether it is installed is on the returned Tool.
func (w *Wrapper) tool(id string) (Tool, bool) {
	for _, t := range w.Tools() {
		if t.ID == id {
			return t, true
		}
	}
	return Tool{}, false
}

func (w *Wrapper) record(event consent.EventType, subject string, fields map[string]string) {
	if w.audit == nil {
		return
	}
	_ = w.audit.Append(consent.Event{Type: event, Subject: subject, Fields: fields})
}

// acknowledges reports whether the user echoed back exactly what they were
// shown. Order and content must match.
func acknowledges(shown, acknowledged []string) bool {
	if len(shown) == 0 || len(shown) != len(acknowledged) {
		return false
	}
	for i := range shown {
		if shown[i] != acknowledged[i] {
			return false
		}
	}
	return true
}

func newToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("remote: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
