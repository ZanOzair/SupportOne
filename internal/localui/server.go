// Package localui serves the agent's user interface to the browser already on
// the machine.
//
// The server exists to show one person their own snapshot, so it is built to be
// reachable by them and by nothing else: it listens on loopback, on a port
// chosen at random, and every API request must carry a token minted for this
// run. It validates Origin and Host on each request, which is what actually
// stops a page in the user's browser from talking to it, and it shuts itself
// down once nobody is using it.
//
// What it cannot defend against is named in docs/THREAT-MODEL.md: another
// process running as the same user.
package localui

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ZanOzair/SupportOne/internal/assist"
	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/redact"
	"github.com/ZanOzair/SupportOne/internal/remediate"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

// DefaultIdleTimeout is how long the server waits with nobody using it before
// shutting down. A diagnostic tool should not outlive the conversation it was
// opened for.
const DefaultIdleTimeout = 15 * time.Minute

// contentSecurityPolicy allows the page to load its own script and stylesheet
// and nothing else: no remote origin, no inline script, no frame, no form post.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; font-src 'self'; connect-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Config is what the server needs to run.
type Config struct {
	// Assets is the built user interface, embedded in the binary.
	Assets fs.FS

	// Snapshot runs the checks. It is a function so the server can re-run them
	// when the user asks, and so tests can supply a fixed snapshot.
	Snapshot func(ctx context.Context) checks.Snapshot

	// Audit records what the user did here.
	Audit *consent.Log

	// Fixes, Applier and Wizards are what lets this interface change
	// anything. All three are optional: a build with none of them serves a
	// read-only agent, and the repair routes say so rather than failing
	// obscurely.
	Fixes   *fixes.Registry
	Applier *remediate.Applier
	Wizards *wizard.Registry

	// CheckTimeout bounds one wizard question.
	CheckTimeout time.Duration

	// Explainer turns each finding into plain language, offline. Optional:
	// without it the interface shows verdicts and no advice.
	Explainer *explain.Explainer

	// Assistant is the optional second tier. Nil, or present and switched
	// off, means the interface never offers to send anything anywhere.
	Assistant *assist.Assistant

	Version     string
	Host        platform.Host
	Identity    redact.Identity
	Lang        string
	IdleTimeout time.Duration
}

// Server is a running local UI.
type Server struct {
	cfg      Config
	token    string
	listener net.Listener
	http     *http.Server

	mu          sync.Mutex
	snapshot    *checks.Snapshot
	lastSeen    time.Time
	escalations map[string]wizard.Escalation

	// wizards holds the sessions this run is part-way through.
	wizards *sessions

	idle chan struct{}
	once sync.Once
}

// New binds a loopback listener on a random port and prepares the server. It
// does not serve until Serve is called.
func New(cfg Config) (*Server, error) {
	if cfg.Snapshot == nil {
		return nil, fmt.Errorf("localui: a snapshot function is required")
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}

	token, err := newToken()
	if err != nil {
		return nil, err
	}

	// Port 0 asks the OS for a free port, which is what makes the address
	// unguessable to anything that is not told it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("localui: listen on loopback: %w", err)
	}

	s := &Server{
		cfg:         cfg,
		token:       token,
		listener:    listener,
		lastSeen:    time.Now(),
		idle:        make(chan struct{}),
		escalations: make(map[string]wizard.Escalation),
		wizards:     newSessions(),
	}
	s.http = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

// newToken mints the per-session bearer token. It comes from the operating
// system's cryptographic random source, not from a timestamp or a counter.
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("localui: generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Addr is the loopback address the server is listening on.
func (s *Server) Addr() string { return s.listener.Addr().String() }

// URL is the address to open, carrying the session token. It is the only place
// the token appears, and the page removes it from the address bar on load.
func (s *Server) URL() string {
	return fmt.Sprintf("http://%s/?t=%s", s.Addr(), url.QueryEscape(s.token))
}

// Serve runs until the context is cancelled, the idle timeout expires, or the
// user closes the session from the page.
func (s *Server) Serve(ctx context.Context) error {
	go s.watchIdle(ctx)

	errs := make(chan error, 1)
	go func() {
		err := s.http.Serve(s.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errs <- err
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	case <-s.idle:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.http.Shutdown(shutdownCtx)
}

// watchIdle closes the server once nothing has used it for the idle timeout.
func (s *Server) watchIdle(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.idle:
			return
		case <-ticker.C:
			s.mu.Lock()
			idleFor := time.Since(s.lastSeen)
			s.mu.Unlock()
			if idleFor >= s.cfg.IdleTimeout {
				s.Close()
				return
			}
		}
	}
}

// Close stops the server. It is safe to call more than once.
func (s *Server) Close() {
	s.once.Do(func() { close(s.idle) })
}

func (s *Server) touch() {
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
}
