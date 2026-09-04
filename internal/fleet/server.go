package fleet

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/i18n"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

//go:embed templates/*.gohtml
var templateFS embed.FS

var dashboardTemplates = template.Must(template.ParseFS(templateFS, "templates/*.gohtml"))

// MaxReportBytes bounds one submitted report. A snapshot of fifteen checks is
// a few tens of kilobytes; anything approaching this is not one.
const MaxReportBytes = 2 << 20

// contentSecurityPolicy is the same shape the agent's local interface uses:
// the page loads its own stylesheet and nothing else, and there is no script
// at all because the dashboard is rendered on the server.
const contentSecurityPolicy = "default-src 'none'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Config is what the server needs to run.
type Config struct {
	Store *Store

	// Token is what an agent must present to submit, and a technician to
	// read. The server refuses to start without one: an unauthenticated
	// fleet dashboard is a list of other people's machines on the internet.
	Token string

	// Lang is the dashboard's language.
	Lang string

	// Logger records requests. It never receives a token or a machine name.
	Logger *slog.Logger
}

// Server serves the dashboard and receives reports.
type Server struct {
	cfg     Config
	bundle  *i18n.Bundle
	explain *explain.Explainer
	http    *http.Server

	// listener is set by Serve and read by Addr and Close, which callers
	// reach from other goroutines — a supervisor logging the address, a
	// signal handler shutting the server down. The mutex is what makes those
	// two safe to call while Serve is starting.
	mu       sync.Mutex
	listener net.Listener
}

// New prepares a server. It binds nothing until Serve is called.
func New(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("fleet: a store is required")
	}
	if len(strings.TrimSpace(cfg.Token)) < MinTokenLength {
		return nil, fmt.Errorf("fleet: a token of at least %d characters is required; the server will not serve a fleet without one", MinTokenLength)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	bundle, err := i18n.Load(cfg.Lang)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:    cfg,
		bundle: bundle,
		// The dashboard explains findings with the same table the agent
		// uses, so a technician reads what the user read. It offers no
		// repairs: nothing here can act on a machine.
		explain: explain.New(nil, nil, platform.Current()),
	}
	s.http = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s, nil
}

// MinTokenLength is the shortest token the server will accept. It is long
// enough that a guess is not worth attempting and short enough to paste.
const MinTokenLength = 24

// Serve listens on addr until the context is cancelled.
func (s *Server) Serve(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("fleet: listen on %s: %w", addr, err)
	}
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	// #nosec G118 -- ctx is already cancelled by the time this line runs; that
	// is what woke the goroutine. Deriving the shutdown deadline from it would
	// hand Shutdown an expired context, which drops in-flight requests instead
	// of letting them finish. The five seconds is the point of the goroutine.
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutdown)
	}()

	if err := s.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("fleet: serve: %w", err)
	}
	return nil
}

// Addr returns what the server is listening on, or "" before Serve has bound.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Close stops the server. Closing one that never listened does nothing.
func (s *Server) Close() error {
	s.mu.Lock()
	listening := s.listener != nil
	s.mu.Unlock()

	if !listening {
		return nil
	}
	return s.http.Close()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Liveness carries no data and needs no token, so a load balancer can
	// use it without being handed the fleet's credential.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})

	mux.Handle("POST /api/report", s.bearer(s.handleReport))

	// The dashboard is behind the same token, presented as a password. A
	// browser knows how to prompt for that; nothing here invents a session.
	mux.Handle("GET /", s.basic(s.handleDashboard))
	mux.Handle("GET /machine/{id}", s.basic(s.handleMachine))

	return s.headers(mux)
}

func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// bearer guards the submission endpoint.
func (s *Server) bearer(h func(http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !s.tokenMatches(presented) {
			http.Error(w, "this server's token is required", http.StatusUnauthorized)
			return
		}
		s.serve(w, r, h)
	})
}

// basic guards the dashboard, using the browser's own credential prompt
// rather than a session mechanism of our own.
func (s *Server) basic(h func(http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, ok := r.BasicAuth()
		if !ok || !s.tokenMatches(password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="SupportOne", charset="UTF-8"`)
			http.Error(w, "this server's token is required", http.StatusUnauthorized)
			return
		}
		s.serve(w, r, h)
	})
}

// tokenMatches compares in constant time, so a wrong token takes as long to
// reject as a nearly-right one.
func (s *Server) tokenMatches(presented string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.cfg.Token)) == 1
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request, h func(http.ResponseWriter, *http.Request) error) {
	if err := h(w, r); err != nil {
		// The reason is logged for the operator and not returned: a stored
		// report's contents are not this server's to echo back.
		s.cfg.Logger.Error("request failed", "path", r.URL.Path, "error", err.Error())
		http.Error(w, "the server could not complete that request", http.StatusInternalServerError)
	}
}

// handleReport receives one report from an agent.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) error {
	var report Report
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxReportBytes)).Decode(&report); err != nil {
		http.Error(w, "that report could not be read", http.StatusBadRequest)
		return nil
	}

	machine, err := s.cfg.Store.Put(report, time.Now())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return err
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	// The machine's ID is logged, never its name: a fleet server's log should
	// not become a directory of whose computer is whose.
	s.cfg.Logger.Info("report received",
		"machine", machine.ID,
		"results", len(report.Snapshot.Results),
		"redacted", report.Redacted,
	)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(map[string]string{"status": "stored", "machine": machine.ID})
}

// dashboardRow is one machine as the list shows it.
type dashboardRow struct {
	ID       string
	Name     string
	OS       string
	LastSeen string
	Stale    bool
	Counts   []severityCount
}

type severityCount struct {
	Severity string
	Label    string
	Value    int
}

// staleAfter is how long since a report before a machine is shown as out of
// touch. A fleet view that treats a month-old report as current is worse than
// no fleet view.
const staleAfter = 7 * 24 * time.Hour

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) error {
	machines, err := s.cfg.Store.List()
	if err != nil {
		return err
	}

	rows := make([]dashboardRow, 0, len(machines))
	for _, m := range machines {
		rows = append(rows, s.row(m))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return dashboardTemplates.ExecuteTemplate(w, "dashboard.gohtml", map[string]any{
		"Machines": rows,
		"Strings":  s.strings(),
		"Empty":    len(rows) == 0,
	})
}

func (s *Server) row(m Machine) dashboardRow {
	counts := m.Counts()

	row := dashboardRow{
		ID:       m.ID,
		Name:     m.Name,
		OS:       platform.OS(m.OS).Display(),
		LastSeen: m.LastSeen.Format("2006-01-02 15:04 MST"),
		Stale:    time.Since(m.LastSeen) > staleAfter,
	}
	for _, severity := range []checks.Severity{
		checks.SeverityUrgent, checks.SeverityAttention, checks.SeverityUnknown, checks.SeverityOK,
	} {
		row.Counts = append(row.Counts, severityCount{
			Severity: string(severity),
			Label:    s.bundle.T("severity." + string(severity)),
			Value:    counts[severity],
		})
	}
	return row
}

// finding is one check result as the machine page shows it.
type finding struct {
	ID       string
	Severity string
	Label    string
	Summary  string
	Cause    string
	Steps    []string
	Error    string
}

func (s *Server) handleMachine(w http.ResponseWriter, r *http.Request) error {
	machine, err := s.cfg.Store.Get(r.PathValue("id"))
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "no such machine", http.StatusNotFound)
		return nil
	}
	if err != nil {
		http.Error(w, "no such machine", http.StatusNotFound)
		return nil
	}

	latest, ok := machine.Latest()
	var findings []finding
	if ok {
		findings = s.findings(latest.Snapshot)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return dashboardTemplates.ExecuteTemplate(w, "machine.gohtml", map[string]any{
		"Machine":  s.row(machine),
		"Findings": findings,
		"Reports":  len(machine.History),
		"Redacted": ok && latest.Redacted,
		"Strings":  s.strings(),
	})
}

// findings renders a snapshot worst-first, with the same offline explanation
// the person at the machine was shown.
func (s *Server) findings(snap checks.Snapshot) []finding {
	advice := make(map[string]explain.Advice)
	for _, a := range s.explain.ForSnapshot(snap) {
		advice[a.CheckID] = a
	}

	ordered := make([]checks.Result, len(snap.Results))
	copy(ordered, snap.Results)
	rank := map[checks.Severity]int{
		checks.SeverityUrgent: 0, checks.SeverityAttention: 1,
		checks.SeverityUnknown: 2, checks.SeverityOK: 3,
	}
	sortStable(ordered, func(a, b checks.Result) bool {
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		return a.CheckID < b.CheckID
	})

	out := make([]finding, 0, len(ordered))
	for _, res := range ordered {
		row := finding{
			ID:       res.CheckID,
			Severity: string(res.Severity),
			Label:    s.bundle.T("severity." + string(res.Severity)),
			Summary:  s.bundle.T(res.Summary, res.Args...),
			Error:    res.Err,
		}
		if a, ok := advice[res.CheckID]; ok {
			row.Cause = s.bundle.T(a.Cause)
			for _, step := range a.Steps {
				row.Steps = append(row.Steps, s.bundle.T(step))
			}
		}
		out = append(out, row)
	}
	return out
}

// strings collects the dashboard's fixed labels, so the template holds no
// English. Keys are stored with underscores because a Go template cannot
// address a map key containing a dot with field syntax.
func (s *Server) strings() map[string]string {
	keys := []string{
		"fleet.title", "fleet.subtitle", "fleet.machine", "fleet.last_seen",
		"fleet.no_machines", "fleet.no_machines_note", "fleet.stale", "fleet.reports",
		"fleet.redacted", "fleet.back", "fleet.findings", "fleet.no_findings",
		"fleet.what_this_means", "fleet.what_to_do", "fleet.one_way",
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[strings.ReplaceAll(key, ".", "_")] = s.bundle.T(key)
	}
	return out
}
