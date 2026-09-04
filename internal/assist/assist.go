// Package assist is the optional second tier: an outside language model,
// asked for a second opinion on a snapshot the offline explainer has already
// explained.
//
// It is off. Not disabled-by-default in the sense of a switch someone might
// flip without noticing — the agent will not contact anything unless a person
// enabled this, supplied an endpoint, saw the exact bytes that would leave the
// machine, and confirmed that specific send. This is the only outbound
// connection SupportOne makes, and everything here exists to keep it that way.
//
// Three properties hold whatever the model says:
//
//   - It cannot execute anything. The only actionable thing it can return is a
//     list of fix IDs, and those are resolved against the compiled-in registry
//     before the user is offered them. An ID that was never built into this
//     binary is discarded at the boundary. So the worst a manipulated model —
//     or a machine's own data crafted to manipulate one — can do is name an ID
//     that does not resolve.
//   - Its prose is shown as its prose. It never becomes SupportOne's verdict,
//     and it never replaces the Tier-1 explanation, which stands on its own.
//   - No credential is stored. See Key.
package assist

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
)

// KeyEnv is the only place SupportOne looks for a credential.
//
// It is deliberately not stored anywhere — not in a config file, not in the
// OS keychain, not in the audit log. A tool that keeps your API key becomes a
// place your API key can leak from, and this one does not need to be that.
// The key is read from the environment for the life of the process and is
// never written down. Local model servers, which most people running this
// will use, need no key at all.
const KeyEnv = "SUPPORTONE_ASSIST_KEY"

// Limits. Every one of them is a refusal rather than a truncation, except the
// response cap, which is a read limit.
const (
	// MaxRequestBytes bounds what may leave the machine in one send.
	MaxRequestBytes = 256 << 10

	// MaxResponseBytes bounds what is read back. A model that answers with a
	// gigabyte does not get to exhaust memory.
	MaxResponseBytes = 64 << 10

	// MaxNotes bounds how much model prose is kept for display.
	MaxNotes = 4000

	// MaxFixIDs bounds how many suggestions are considered. A response
	// naming a hundred is not a response worth reading.
	MaxFixIDs = 12

	// DefaultTimeout bounds one request end to end.
	DefaultTimeout = 60 * time.Second
)

// Errors callers distinguish between.
var (
	// ErrDisabled means the assistant was not turned on. It is the default
	// state and it is not an error condition, only a refusal to act.
	ErrDisabled = errors.New("assist: the assistant is off; nothing was sent")

	// ErrNotConfirmed means Ask was called without the token from the payload
	// the user was shown, or with one already spent.
	ErrNotConfirmed = errors.New("assist: this send was not confirmed against the payload that was shown")

	// ErrInsecureEndpoint means the endpoint would send the snapshot in the
	// clear to somewhere other than this machine.
	ErrInsecureEndpoint = errors.New("assist: an endpoint that is not HTTPS is only allowed on this computer")

	// ErrTooLarge means the payload exceeds what one send may carry.
	ErrTooLarge = errors.New("assist: this payload is larger than one request may carry")
)

// Config is what the user turned on, and where they pointed it.
type Config struct {
	// Enabled is false unless a person set it. Nothing here runs otherwise.
	Enabled bool `json:"enabled"`

	// Endpoint is an OpenAI-shaped chat completions URL. That shape is what
	// Ollama, llama.cpp, LM Studio and most gateways already speak, so a
	// local model needs no adapter and no key.
	Endpoint string `json:"endpoint"`

	// Model names the model to ask for.
	Model string `json:"model"`

	// Timeout bounds one request. Zero means DefaultTimeout.
	Timeout time.Duration `json:"timeout"`
}

// Payload is exactly what would leave this computer, and nothing that is not
// in it will.
type Payload struct {
	// Endpoint and Model are shown so the user knows where it is going.
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`

	// Host is the endpoint's host alone, which is what the audit log records:
	// a full URL can carry a key in its query.
	Host string `json:"host"`

	// Body is the request as it would be sent, formatted for a person to
	// read. It is the payload, not a summary of it.
	Body string `json:"body"`

	// Bytes is what Body weighs.
	Bytes int `json:"bytes"`

	// Redacted says whether anything was removed before this was built.
	Redacted bool `json:"redacted"`

	// Token proves this payload was shown. Ask will not send without it.
	Token string `json:"token"`
}

// Answer is what came back, after everything untrusted in it was contained.
type Answer struct {
	// Notes is the model's own prose, and is presented as the model's own.
	// It is never SupportOne's verdict and never replaces the offline
	// explanation.
	Notes string `json:"notes"`

	// Fixes are the suggestions that survived the registry. Every entry is a
	// repair this build actually carries and this platform actually runs.
	Fixes []string `json:"fixes,omitempty"`

	// Discarded counts suggestions that named something this binary does not
	// carry. It is shown, because a model that keeps suggesting things that
	// do not exist is worth knowing about.
	Discarded int `json:"discarded"`

	// Model is what answered, as reported by the endpoint.
	Model string `json:"model,omitempty"`
}

// Assistant asks an outside model, once the user has said to.
type Assistant struct {
	cfg   Config
	fixes *fixes.Registry
	os    platform.OS
	audit *consent.Log

	// client is swappable in tests, which is how every test in this package
	// runs without a network.
	client *http.Client

	// key reads the credential. Swappable in tests; production reads the
	// environment and nothing else.
	key func() string

	mu      sync.Mutex
	pending map[string]Payload
}

// New returns an assistant. It makes no connection and reads no credential
// until asked to send.
func New(cfg Config, registry *fixes.Registry, os platform.OS, audit *consent.Log) *Assistant {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	return &Assistant{
		cfg:     cfg,
		fixes:   registry,
		os:      os,
		audit:   audit,
		client:  &http.Client{Timeout: timeout},
		key:     readKey,
		pending: make(map[string]Payload),
	}
}

// readKey is the one credential read in the whole codebase, named so it is
// easy to find and easy to point at in a review. Nothing writes one back.
func readKey() string { return os.Getenv(KeyEnv) }

// Enabled reports whether the user turned this on.
func (a *Assistant) Enabled() bool { return a.cfg.Enabled }

// Endpoint returns where this is pointed, for display.
func (a *Assistant) Endpoint() string { return a.cfg.Endpoint }

// Prepare builds the exact payload for a snapshot, applying the redaction the
// user chose, and sends nothing.
//
// This is the whole point of the two-step design: what the user confirms is
// the bytes, not a description of the bytes.
func (a *Assistant) Prepare(snap checks.Snapshot, policy redact.Policy, id redact.Identity) (Payload, error) {
	if !a.cfg.Enabled {
		return Payload{}, ErrDisabled
	}
	if err := CheckEndpoint(a.cfg.Endpoint); err != nil {
		return Payload{}, err
	}

	redacted, err := policy.Snapshot(snap, id)
	if err != nil {
		return Payload{}, fmt.Errorf("assist: apply redaction: %w", err)
	}

	body, err := a.request(redacted)
	if err != nil {
		return Payload{}, err
	}
	if len(body) > MaxRequestBytes {
		return Payload{}, fmt.Errorf("%w: %d bytes, limit %d", ErrTooLarge, len(body), MaxRequestBytes)
	}

	host, err := endpointHost(a.cfg.Endpoint)
	if err != nil {
		return Payload{}, err
	}

	token, err := newToken()
	if err != nil {
		return Payload{}, err
	}

	payload := Payload{
		Endpoint: a.cfg.Endpoint,
		Model:    a.cfg.Model,
		Host:     host,
		Body:     string(body),
		Bytes:    len(body),
		Redacted: !policy.Nothing(),
		Token:    token,
	}

	a.mu.Lock()
	a.pending[token] = payload
	a.mu.Unlock()
	return payload, nil
}

// Ask sends a payload the user confirmed, and contains what comes back.
func (a *Assistant) Ask(ctx context.Context, token string) (Answer, error) {
	if !a.cfg.Enabled {
		return Answer{}, ErrDisabled
	}

	a.mu.Lock()
	payload, ok := a.pending[token]
	if ok {
		// One agreement, one send. A reused token would let a second request
		// ride on a confirmation given for the first.
		delete(a.pending, token)
	}
	a.mu.Unlock()

	if !ok {
		return Answer{}, ErrNotConfirmed
	}

	answer, err := a.send(ctx, payload)

	// The audit records that bytes left the machine, whether or not the
	// answer was any good, and records the host rather than the URL: a query
	// string can carry a credential.
	a.record(consent.EventDataSent, payload.Host, map[string]string{
		"purpose":  "assistant",
		"bytes":    fmt.Sprint(payload.Bytes),
		"redacted": fmt.Sprint(payload.Redacted),
		"model":    payload.Model,
		"answered": fmt.Sprint(err == nil),
	})
	if err != nil {
		return Answer{}, err
	}
	return answer, nil
}

// Pending reports how many prepared payloads are awaiting a decision. It is
// here so a caller can tell an unconfirmed send from a spent one.
func (a *Assistant) Pending() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

// Discard drops a prepared payload the user decided against.
func (a *Assistant) Discard(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.pending, token)
}

func (a *Assistant) record(event consent.EventType, subject string, fields map[string]string) {
	if a.audit == nil {
		return
	}
	_ = a.audit.Append(consent.Event{Type: event, Subject: subject, Fields: fields})
}

func newToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("assist: generate payload token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// CheckEndpoint refuses an endpoint that would put the snapshot on the wire in
// the clear.
//
// HTTPS is required everywhere except this machine. A local model server on
// 127.0.0.1 over plain HTTP is the common, sensible case — the traffic never
// leaves the computer — and refusing it would push people towards a hosted
// service instead, which is the opposite of what this is for.
func CheckEndpoint(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("assist: no endpoint is configured")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("assist: the endpoint is not a URL: %w", err)
	}

	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopback(parsed.Hostname()) {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrInsecureEndpoint, parsed.Host)
	default:
		return fmt.Errorf("assist: %q is not a scheme this sends over", parsed.Scheme)
	}
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// endpointHost returns the host and port, without the path or query.
func endpointHost(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("assist: the endpoint is not a URL: %w", err)
	}
	return parsed.Host, nil
}

// send performs the request. It is the only function in SupportOne that opens
// an outbound connection.
func (a *Assistant) send(ctx context.Context, payload Payload) (Answer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payload.Endpoint, bytes.NewReader([]byte(payload.Body)))
	if err != nil {
		return Answer{}, fmt.Errorf("assist: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if key := a.key(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	res, err := a.client.Do(req)
	if err != nil {
		// The URL can carry a key in its query, and an http error quotes the
		// URL. Report the host instead.
		return Answer{}, fmt.Errorf("assist: %s did not answer", payload.Host)
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(res.Body, MaxResponseBytes))
	if err != nil {
		return Answer{}, fmt.Errorf("assist: read the answer from %s: %w", payload.Host, err)
	}
	if res.StatusCode != http.StatusOK {
		return Answer{}, fmt.Errorf("assist: %s refused the request (%d)", payload.Host, res.StatusCode)
	}

	return a.answer(raw)
}

// answer turns a response into a contained Answer. Everything in it is
// untrusted: the prose is cleaned and capped, and the IDs are resolved.
func (a *Assistant) answer(raw []byte) (Answer, error) {
	content, model, err := completionContent(raw)
	if err != nil {
		return Answer{}, err
	}

	suggested, notes := parseSuggestion(content)

	answer := Answer{Notes: cleanNotes(notes), Model: model}
	if len(suggested) > MaxFixIDs {
		answer.Discarded += len(suggested) - MaxFixIDs
		suggested = suggested[:MaxFixIDs]
	}

	// The whitelist. This is the line the model cannot cross: an ID that was
	// never compiled in resolves to nothing and is dropped here, before the
	// user is offered anything.
	if a.fixes != nil {
		known, discarded := a.fixes.Resolve(suggested, a.os)
		for _, f := range known {
			answer.Fixes = append(answer.Fixes, f.ID())
		}
		answer.Discarded += len(discarded)
	} else {
		answer.Discarded += len(suggested)
	}
	return answer, nil
}
