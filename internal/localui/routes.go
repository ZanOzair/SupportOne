package localui

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/consent"
	"github.com/ZanOzair/supportone/internal/i18n"
	"github.com/ZanOzair/supportone/internal/redact"
	"github.com/ZanOzair/supportone/internal/report"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /api/session", s.api(s.handleSession))
	mux.Handle("GET /api/messages", s.api(s.handleMessages))
	mux.Handle("GET /api/snapshot", s.api(s.handleSnapshot))
	mux.Handle("POST /api/snapshot", s.api(s.handleRefresh))
	mux.Handle("POST /api/preview", s.api(s.handlePreview))
	mux.Handle("GET /api/report", s.api(s.handleReport))
	mux.Handle("POST /api/close", s.api(s.handleClose))

	// The interface itself is static and carries no data; the snapshot only
	// ever arrives through the token-protected API above.
	mux.Handle("/", s.static())

	return s.baseHeaders(s.checkOriginAndHost(mux))
}

// baseHeaders applies the hardening every response gets, static or not.
func (s *Server) baseHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// checkOriginAndHost is the defence against DNS rebinding. A page on another
// site can make the browser resolve a name to 127.0.0.1 and send requests here,
// but it cannot forge these two headers.
func (s *Server) checkOriginAndHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostIsLoopback(r.Host) {
			http.Error(w, "unexpected Host header", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !s.originIsOurs(origin) {
			http.Error(w, "unexpected Origin header", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostIsLoopback accepts only the address this server is actually listening on.
// A rebound name ("evil.example") does not match, even when it resolves here.
func (s *Server) hostIsLoopback(host string) bool {
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if _, listeningPort, err := net.SplitHostPort(s.Addr()); err != nil || port != listeningPort {
		return false
	}

	ip := net.ParseIP(strings.Trim(name, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (s *Server) originIsOurs(origin string) bool {
	const prefix = "http://"
	rest, ok := strings.CutPrefix(origin, prefix)
	if !ok {
		return false
	}
	return s.hostIsLoopback(rest)
}

// api wraps a handler with the session token requirement.
func (s *Server) api(h func(http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorised(r) {
			http.Error(w, "this session's token is required", http.StatusUnauthorized)
			return
		}
		s.touch()

		if err := h(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// authorised compares the presented token in constant time.
func (s *Server) authorised(r *http.Request) bool {
	presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if presented == "" {
		presented = r.URL.Query().Get("t")
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.token)) == 1
}

// static serves the built interface. Requests for unknown paths return the
// page itself, so the interface can own its own routing without the server
// growing a list of its screens.
func (s *Server) static() http.Handler {
	assets, err := fs.Sub(s.cfg.Assets, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "the interface is not built into this binary", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.touch()
		if _, err := fs.Stat(assets, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

type sessionInfo struct {
	Version   string   `json:"version"`
	OS        string   `json:"os"`
	Arch      string   `json:"arch"`
	Lang      string   `json:"lang"`
	Languages []string `json:"languages"`
	AuditPath string   `json:"audit_path"`
}

func (s *Server) handleSession(w http.ResponseWriter, _ *http.Request) error {
	info := sessionInfo{
		Version:   s.cfg.Version,
		OS:        s.cfg.Host.OS.Display(),
		Arch:      s.cfg.Host.Arch,
		Lang:      i18n.Resolve(s.cfg.Lang),
		Languages: i18n.Available(),
	}
	if s.cfg.Audit != nil {
		info.AuditPath = s.cfg.Audit.Path()
	}
	return writeJSON(w, info)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) error {
	bundle, err := i18n.Load(r.URL.Query().Get("lang"))
	if err != nil {
		return err
	}
	return writeJSON(w, map[string]any{"lang": bundle.Lang(), "messages": bundle.Messages()})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, s.currentSnapshot(r.Context()))
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) error {
	snap := s.cfg.Snapshot(r.Context())

	s.mu.Lock()
	s.snapshot = &snap
	s.mu.Unlock()

	s.record(consent.EventCheckRun, "snapshot", map[string]string{"checks_run": fmt.Sprint(len(snap.Results))})
	return writeJSON(w, snap)
}

// handlePreview shows the user exactly what a redaction policy would leave.
// Nothing is sent anywhere; this is the payload they get to inspect first.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) error {
	var policy redact.Policy
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&policy); err != nil {
		return fmt.Errorf("read redaction policy: %w", err)
	}

	redacted, err := policy.Snapshot(s.currentSnapshot(r.Context()), s.cfg.Identity)
	if err != nil {
		return err
	}
	return writeJSON(w, redacted)
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query()
	policy := redact.Policy{
		Hostnames: query.Get("hostnames") == "1",
		Usernames: query.Get("usernames") == "1",
		Serials:   query.Get("serials") == "1",
		Addresses: query.Get("addresses") == "1",
	}

	snap, err := policy.Snapshot(s.currentSnapshot(r.Context()), s.cfg.Identity)
	if err != nil {
		return err
	}

	format := query.Get("format")
	if format == "" {
		format = "html"
	}
	filename := report.Filename(snap, format)

	// The user asked for a file, so it downloads rather than opening in place.
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	s.record(consent.EventDataSent, filename, map[string]string{
		"destination": "local file",
		"redacted":    fmt.Sprint(!policy.Nothing()),
	})

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		return report.JSON(w, snap)
	case "html":
		bundle, err := i18n.Load(query.Get("lang"))
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		opts := report.Options{Bundle: bundle, Redacted: !policy.Nothing()}
		if s.cfg.Audit != nil {
			opts.AuditPath = s.cfg.Audit.Path()
		}
		return report.HTML(w, snap, opts)
	default:
		return fmt.Errorf("unknown report format %q", format)
	}
}

func (s *Server) handleClose(w http.ResponseWriter, _ *http.Request) error {
	if err := writeJSON(w, map[string]string{"status": "closing"}); err != nil {
		return err
	}
	// Give the response a moment to reach the browser before the listener goes.
	time.AfterFunc(200*time.Millisecond, s.Close)
	return nil
}

// currentSnapshot returns the snapshot this session is showing, running the
// checks once on first use.
func (s *Server) currentSnapshot(ctx context.Context) checks.Snapshot {
	s.mu.Lock()
	if s.snapshot != nil {
		snap := *s.snapshot
		s.mu.Unlock()
		return snap
	}
	s.mu.Unlock()

	snap := s.cfg.Snapshot(ctx)

	s.mu.Lock()
	s.snapshot = &snap
	s.mu.Unlock()
	return snap
}

func (s *Server) record(event consent.EventType, subject string, fields map[string]string) {
	if s.cfg.Audit == nil {
		return
	}
	// A failure to write the audit log must not silently drop the entry, but
	// it also must not take down the interface the user is looking at.
	_ = s.cfg.Audit.Append(consent.Event{Type: event, Subject: subject, Fields: fields})
}

func writeJSON(w http.ResponseWriter, value any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(value)
}
