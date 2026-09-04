// Package enrol sends one report to a fleet server the user chose.
//
// This is the second and last thing in SupportOne that can open an outbound
// connection, and it is gated exactly like the first. It is off. There is no
// default server. The exact bytes are built and shown before anything leaves,
// and a send happens only against a confirmation of that specific payload.
//
// What makes a fleet here different from most is what is absent. Nothing is
// installed as a service, nothing polls, and the server has no way to ask this
// machine anything: every report exists because a person at the machine looked
// at it and chose to send it. A technician's dashboard is therefore a record
// of what people told them, not a feed from machines they are watching.
package enrol

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/egress"
	"github.com/ZanOzair/SupportOne/internal/fleet"
	"github.com/ZanOzair/SupportOne/internal/redact"
)

// TokenEnv is the only place a fleet credential is read from. It is never
// written anywhere: not a config file, not the OS keychain, not the audit log.
// It is the same variable the server reads, so an operator sets one secret.
const TokenEnv = "SUPPORTONE_FLEET_TOKEN"

// Limits.
const (
	// MaxRequestBytes bounds what may leave in one send. It matches what the
	// server will accept, so an oversized report is refused here rather than
	// travelling and being rejected there.
	MaxRequestBytes = 2 << 20

	// MaxResponseBytes bounds what is read back.
	MaxResponseBytes = 32 << 10

	// DefaultTimeout bounds one request end to end.
	DefaultTimeout = 30 * time.Second
)

// Errors callers distinguish between.
var (
	// ErrDisabled means reporting to a fleet was not turned on. It is the
	// default and it is not an error condition, only a refusal to act.
	ErrDisabled = errors.New("enrol: reporting to a fleet server is off; nothing was sent")

	// ErrNotConfirmed means Send was called without the token from the
	// payload the user was shown, or with one already spent.
	ErrNotConfirmed = errors.New("enrol: this send was not confirmed against the payload that was shown")

	// ErrNoName means no machine name was given. There is no fallback: what
	// a machine is called in someone else's dashboard is a decision, not
	// something to harvest from the hostname.
	ErrNoName = errors.New("enrol: this machine needs a name to report under")

	// ErrTooLarge means the report exceeds what one send may carry.
	ErrTooLarge = errors.New("enrol: this report is larger than one request may carry")

	// ErrNoCredential means the server's token was not in the environment.
	ErrNoCredential = errors.New("enrol: no fleet token is set")
)

// Config is what the user turned on, and where they pointed it.
type Config struct {
	// Enabled is false unless a person set it.
	Enabled bool `json:"enabled"`

	// Server is the fleet server's base URL.
	Server string `json:"server"`

	// Name is what this machine is called in the dashboard. The person at
	// the machine chooses it.
	Name string `json:"name"`

	// Timeout bounds one request. Zero means DefaultTimeout.
	Timeout time.Duration `json:"timeout"`
}

// Payload is exactly what would leave this computer.
type Payload struct {
	Server string `json:"server"`
	Host   string `json:"host"`
	Name   string `json:"name"`

	// Body is the request as it would be sent, formatted for a person to
	// read. It is the payload, not a summary of it.
	Body string `json:"body"`

	Bytes    int  `json:"bytes"`
	Redacted bool `json:"redacted"`

	// Token proves this payload was shown. Send will not act without it.
	Token string `json:"token"`
}

// Result is what the server said.
type Result struct {
	// Machine is the identifier the server filed it under. The name is not
	// echoed back; the identifier is what a technician's URL uses.
	Machine string `json:"machine"`
	Host    string `json:"host"`
	Bytes   int    `json:"bytes"`
}

// Enroller sends reports, once the user has said to.
type Enroller struct {
	cfg          Config
	audit        *consent.Log
	agentVersion string

	client *http.Client

	// key reads the credential. Swappable in tests; production reads the
	// environment and nothing else.
	key func() string

	mu      sync.Mutex
	pending map[string]Payload
}

// New returns an enroller. It makes no connection and reads no credential
// until asked to send.
func New(cfg Config, audit *consent.Log, agentVersion string) *Enroller {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	return &Enroller{
		cfg:          cfg,
		audit:        audit,
		agentVersion: agentVersion,
		client:       &http.Client{Timeout: timeout},
		key:          readToken,
		pending:      make(map[string]Payload),
	}
}

// readToken is the one credential read in this package, named so it is easy to
// find in a review. Nothing writes one back.
func readToken() string { return strings.TrimSpace(os.Getenv(TokenEnv)) }

// Enabled reports whether the user turned this on.
func (e *Enroller) Enabled() bool { return e.cfg.Enabled }

// Server returns where this is pointed, for display.
func (e *Enroller) Server() string { return e.cfg.Server }

// Prepare builds the exact payload, applying the redaction the user chose, and
// sends nothing.
func (e *Enroller) Prepare(snap checks.Snapshot, policy redact.Policy, id redact.Identity) (Payload, error) {
	if !e.cfg.Enabled {
		return Payload{}, ErrDisabled
	}
	if strings.TrimSpace(e.cfg.Name) == "" {
		return Payload{}, ErrNoName
	}
	if err := egress.CheckURL(e.cfg.Server); err != nil {
		return Payload{}, err
	}
	if e.key() == "" {
		// Better to say so now than to build a payload, show it, take a
		// confirmation, and only then discover there is nothing to send with.
		return Payload{}, fmt.Errorf("%w: set %s to the server's token", ErrNoCredential, TokenEnv)
	}

	redacted, err := policy.Snapshot(snap, id)
	if err != nil {
		return Payload{}, fmt.Errorf("enrol: apply redaction: %w", err)
	}

	report := fleet.Report{
		Name:         strings.TrimSpace(e.cfg.Name),
		Snapshot:     redacted,
		Redacted:     !policy.Nothing(),
		AgentVersion: e.agentVersion,
	}
	if err := report.Validate(); err != nil {
		return Payload{}, err
	}

	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return Payload{}, fmt.Errorf("enrol: build the report: %w", err)
	}
	if len(body) > MaxRequestBytes {
		return Payload{}, fmt.Errorf("%w: %d bytes, limit %d", ErrTooLarge, len(body), MaxRequestBytes)
	}

	host, err := egress.Host(e.cfg.Server)
	if err != nil {
		return Payload{}, err
	}

	token, err := newToken()
	if err != nil {
		return Payload{}, err
	}

	payload := Payload{
		Server:   e.cfg.Server,
		Host:     host,
		Name:     report.Name,
		Body:     string(body),
		Bytes:    len(body),
		Redacted: report.Redacted,
		Token:    token,
	}

	e.mu.Lock()
	e.pending[token] = payload
	e.mu.Unlock()
	return payload, nil
}

// Send delivers a payload the user confirmed.
func (e *Enroller) Send(ctx context.Context, token string) (Result, error) {
	if !e.cfg.Enabled {
		return Result{}, ErrDisabled
	}

	e.mu.Lock()
	payload, ok := e.pending[token]
	if ok {
		// One agreement, one send.
		delete(e.pending, token)
	}
	e.mu.Unlock()

	if !ok {
		return Result{}, ErrNotConfirmed
	}

	result, err := e.post(ctx, payload)

	// The audit records that bytes left the machine, whether or not the
	// server accepted them, and records the host rather than the URL.
	e.record(payload, err == nil)
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// Discard drops a prepared payload the user decided against.
func (e *Enroller) Discard(token string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pending, token)
}

// Pending reports how many prepared payloads await a decision.
func (e *Enroller) Pending() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.pending)
}

func (e *Enroller) post(ctx context.Context, payload Payload) (Result, error) {
	endpoint := strings.TrimSuffix(payload.Server, "/") + "/api/report"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte(payload.Body)))
	if err != nil {
		return Result{}, fmt.Errorf("enrol: build the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.key())

	res, err := e.client.Do(req)
	if err != nil {
		// The URL can carry a credential in its query, and an http error
		// quotes the URL. Report the host instead.
		return Result{}, fmt.Errorf("enrol: %s did not answer", payload.Host)
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(res.Body, MaxResponseBytes))
	if err != nil {
		return Result{}, fmt.Errorf("enrol: read the answer from %s: %w", payload.Host, err)
	}
	if res.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("enrol: %s refused the report (%d)", payload.Host, res.StatusCode)
	}

	var answer struct {
		Machine string `json:"machine"`
	}
	// A server that answered 200 with something unexpected still took the
	// report; losing the identifier is not losing the send.
	_ = json.Unmarshal(raw, &answer)

	return Result{Machine: answer.Machine, Host: payload.Host, Bytes: payload.Bytes}, nil
}

func (e *Enroller) record(payload Payload, delivered bool) {
	if e.audit == nil {
		return
	}
	_ = e.audit.Append(consent.Event{
		Type:    consent.EventDataSent,
		Subject: payload.Host,
		Fields: map[string]string{
			"purpose":   "fleet report",
			"bytes":     fmt.Sprint(payload.Bytes),
			"redacted":  fmt.Sprint(payload.Redacted),
			"delivered": fmt.Sprint(delivered),
		},
	})
}

func newToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("enrol: generate payload token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
