package localui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/remediate"
	"github.com/ZanOzair/SupportOne/internal/wizard"
)

// maxBody bounds every request that carries one. These handlers take a fix ID
// and a list of acknowledged changes, never a document.
const maxBody = 16 << 10

// maxSessions bounds how many wizard sessions one run will hold. A local
// interface serves one person; more than a handful at once is a bug or a
// misuse, and either way it should not grow without limit.
const maxSessions = 8

// fixSummary is one fix as the interface lists it.
type fixSummary struct {
	ID            string            `json:"id"`
	Explanation   fixes.Explanation `json:"explanation"`
	RequiresAdmin bool              `json:"requires_admin"`
	Reversible    bool              `json:"reversible"`
}

func (s *Server) handleFixes(w http.ResponseWriter, _ *http.Request) error {
	if s.cfg.Fixes == nil {
		return writeJSON(w, []fixSummary{})
	}

	available := s.cfg.Fixes.ForPlatform(s.cfg.Host.OS)
	out := make([]fixSummary, 0, len(available))
	for _, f := range available {
		out = append(out, fixSummary{
			ID:            f.ID(),
			Explanation:   f.Describe(),
			RequiresAdmin: f.RequiresAdmin(),
			Reversible:    f.Reversible(),
		})
	}
	return writeJSON(w, out)
}

// handlePlan describes what a fix would do. It changes nothing, which is why
// it is the only step the interface can take without the user.
func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Applier == nil {
		return s.unavailable(w)
	}

	var body struct {
		FixID string `json:"fix_id"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}

	plan, err := s.cfg.Applier.Plan(r.Context(), body.FixID)
	if err != nil {
		// An ID that is not in the registry lands here, and says so plainly
		// rather than becoming a server error.
		http.Error(w, err.Error(), http.StatusNotFound)
		return nil
	}
	return writeJSON(w, plan)
}

// handleApply makes the change, and only on a confirmation that repeats what
// the plan showed. The gate is remediate's, not this handler's: everything
// here does is carry the user's decision to it intact.
func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Applier == nil {
		return s.unavailable(w)
	}

	var c remediate.Confirmation
	if err := decode(w, r, &c); err != nil {
		return err
	}

	result, err := s.cfg.Applier.Apply(r.Context(), c)
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return nil
	}
	return writeJSON(w, result)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Applier == nil {
		return s.unavailable(w)
	}

	var body struct {
		FixID string `json:"fix_id"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}

	if err := s.cfg.Applier.Rollback(r.Context(), body.FixID); err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return nil
	}
	return writeJSON(w, map[string]string{"status": "rolled back", "fix_id": body.FixID})
}

// wizardSummary is one wizard as the interface lists it.
type wizardSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Complaint string `json:"complaint"`
	Steps     int    `json:"steps"`
}

func (s *Server) handleWizards(w http.ResponseWriter, _ *http.Request) error {
	if s.cfg.Wizards == nil {
		return writeJSON(w, []wizardSummary{})
	}

	available := s.cfg.Wizards.ForPlatform(s.cfg.Host.OS)
	out := make([]wizardSummary, 0, len(available))
	for _, wz := range available {
		out = append(out, wizardSummary{
			ID:        wz.ID,
			Title:     wz.Title,
			Complaint: wz.Complaint,
			Steps:     len(wz.Steps),
		})
	}
	return writeJSON(w, out)
}

// sessions holds the wizard runs this server is part-way through.
type sessions struct {
	mu   sync.Mutex
	open map[string]*wizard.Session
}

func newSessions() *sessions { return &sessions{open: make(map[string]*wizard.Session)} }

func (s *sessions) add(id string, session *wizard.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open[id] = session
}

func (s *sessions) get(id string) (*wizard.Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.open[id]
	return session, ok
}

func (s *sessions) drop(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.open, id)
}

func (s *sessions) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.open)
}

// wizardProgress is what every wizard move returns: the session it belongs to
// and where that session now stands.
type wizardProgress struct {
	SessionID string          `json:"session_id"`
	Progress  wizard.Progress `json:"progress"`
}

func (s *Server) handleWizardStart(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Wizards == nil {
		return s.unavailable(w)
	}

	var body struct {
		WizardID string `json:"wizard_id"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}

	wz, ok := s.cfg.Wizards.Get(body.WizardID)
	if !ok || !wz.RunsOn(s.cfg.Host.OS) {
		http.Error(w, fmt.Sprintf("no wizard with ID %q runs on %s", body.WizardID, s.cfg.Host.OS.Display()), http.StatusNotFound)
		return nil
	}
	if s.wizards.count() >= maxSessions {
		http.Error(w, "too many wizard sessions are already open", http.StatusTooManyRequests)
		return nil
	}

	id, err := newToken()
	if err != nil {
		return err
	}
	session := wizard.Start(wz, s.cfg.Applier, s.cfg.CheckTimeout)
	s.wizards.add(id, session)

	progress, err := session.Next(r.Context())
	if err != nil {
		return err
	}
	return writeJSON(w, wizardProgress{SessionID: id, Progress: progress})
}

// wizardMove is the shape every step of a wizard conversation takes.
type wizardMove struct {
	SessionID    string                  `json:"session_id"`
	Confirmation *remediate.Confirmation `json:"confirmation,omitempty"`
}

func (s *Server) handleWizardMove(move string) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		if s.cfg.Wizards == nil {
			return s.unavailable(w)
		}

		var body wizardMove
		if err := decode(w, r, &body); err != nil {
			return err
		}

		session, ok := s.wizards.get(body.SessionID)
		if !ok {
			http.Error(w, "no such wizard session", http.StatusNotFound)
			return nil
		}

		var (
			progress wizard.Progress
			err      error
		)
		switch move {
		case "next":
			progress, err = session.Next(r.Context())
		case "skip":
			progress, err = session.Skip(r.Context())
		case "confirm":
			if body.Confirmation == nil {
				http.Error(w, "a confirmation is required to change anything", http.StatusBadRequest)
				return nil
			}
			progress, err = session.Confirm(r.Context(), *body.Confirmation)
		case "stop":
			progress = session.Stop()
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return nil
		}

		// A finished session is not kept: its history is in the escalation the
		// interface has already been handed, and in the audit log.
		if progress.Outcome != wizard.OutcomeRunning {
			out := wizardProgress{SessionID: body.SessionID, Progress: progress}
			if err := writeJSON(w, out); err != nil {
				return err
			}
			s.finished(body.SessionID, session)
			return nil
		}
		return writeJSON(w, wizardProgress{SessionID: body.SessionID, Progress: progress})
	}
}

// finished records how a session ended and stops holding it.
func (s *Server) finished(id string, session *wizard.Session) {
	escalation := session.Escalate()
	s.record(consent.EventWizardEnded, escalation.WizardID, map[string]string{
		"outcome": string(escalation.Outcome),
		"steps":   fmt.Sprint(len(escalation.Steps)),
	})
	s.mu.Lock()
	s.escalations[id] = escalation
	s.mu.Unlock()
	s.wizards.drop(id)
}

// handleEscalation hands over what was asked and what came of it, for the
// person who helps next. It carries message keys and step IDs, not machine
// identifiers: what to send, and whether to send it, stays the user's call.
func (s *Server) handleEscalation(w http.ResponseWriter, r *http.Request) error {
	id := r.URL.Query().Get("session")

	s.mu.Lock()
	escalation, ok := s.escalations[id]
	s.mu.Unlock()

	if !ok {
		if session, live := s.wizards.get(id); live {
			return writeJSON(w, session.Escalate())
		}
		http.Error(w, "no such wizard session", http.StatusNotFound)
		return nil
	}
	return writeJSON(w, escalation)
}

func (s *Server) unavailable(w http.ResponseWriter) error {
	http.Error(w, "this build has no repairs compiled in", http.StatusNotFound)
	return nil
}

// statusFor maps the consent gate's refusals onto codes the interface can act
// on, rather than flattening every refusal into a server error.
func statusFor(err error) int {
	switch {
	case errors.Is(err, remediate.ErrNotConfirmed):
		return http.StatusForbidden
	case errors.Is(err, remediate.ErrNeedsAdmin):
		return http.StatusForbidden
	case errors.Is(err, remediate.ErrNoRestorePoint):
		return http.StatusPreconditionRequired
	case errors.Is(err, remediate.ErrNotApplied):
		return http.StatusNotFound
	default:
		return http.StatusConflict
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) error {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(into); err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	return nil
}
