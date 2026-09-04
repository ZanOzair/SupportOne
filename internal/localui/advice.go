package localui

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ZanOzair/SupportOne/internal/assist"
	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/explain"
	"github.com/ZanOzair/SupportOne/internal/redact"
)

// handleExplain returns the offline explanation of the current snapshot,
// worst first. It reads a table compiled into the binary and contacts nothing.
func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Explainer == nil {
		return writeJSON(w, []explain.Advice{})
	}
	advice := s.cfg.Explainer.ForSnapshot(s.currentSnapshot(r.Context()))
	return writeJSON(w, explain.Ordered(advice))
}

// assistState is what the interface needs to decide whether to offer the
// second tier at all. It never carries a credential.
type assistState struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint,omitempty"`
	Pending  int    `json:"pending"`
}

func (s *Server) handleAssistState(w http.ResponseWriter, _ *http.Request) error {
	if s.cfg.Assistant == nil || !s.cfg.Assistant.Enabled() {
		return writeJSON(w, assistState{})
	}
	return writeJSON(w, assistState{
		Enabled:  true,
		Endpoint: s.cfg.Assistant.Endpoint(),
		Pending:  s.cfg.Assistant.Pending(),
	})
}

// handleAssistPrepare builds the exact bytes that would leave this computer,
// under the redaction the user chose, and sends none of them.
//
// This is the whole point of the two-step design: what the user confirms is
// the payload, not a description of the payload.
func (s *Server) handleAssistPrepare(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Assistant == nil {
		return s.assistUnavailable(w)
	}

	var policy redact.Policy
	if err := decode(w, r, &policy); err != nil {
		return err
	}

	payload, err := s.cfg.Assistant.Prepare(s.currentSnapshot(r.Context()), policy, s.cfg.Identity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil
	}

	// Preparing is not sending, and the log says which happened.
	s.record(consent.EventConsentAsked, payload.Host, map[string]string{
		"purpose":  "assistant",
		"bytes":    strconv.Itoa(payload.Bytes),
		"redacted": strconv.FormatBool(payload.Redacted),
	})
	return writeJSON(w, payload)
}

// handleAssistAsk performs the one outbound connection this agent makes, and
// only against a token from a payload the user was shown.
func (s *Server) handleAssistAsk(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Assistant == nil {
		return s.assistUnavailable(w)
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}

	answer, err := s.cfg.Assistant.Ask(r.Context(), body.Token)
	if err != nil {
		http.Error(w, err.Error(), assistStatusFor(err))
		return nil
	}
	return writeJSON(w, answer)
}

// handleAssistDiscard drops a prepared payload the user decided against, so a
// declined send does not sit waiting to be confirmed by something else.
func (s *Server) handleAssistDiscard(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Assistant == nil {
		return s.assistUnavailable(w)
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}

	s.cfg.Assistant.Discard(body.Token)
	s.record(consent.EventConsentDenied, "assistant", map[string]string{"purpose": "assistant"})
	return writeJSON(w, map[string]string{"status": "discarded"})
}

func (s *Server) assistUnavailable(w http.ResponseWriter) error {
	http.Error(w, "the assistant is off; nothing was sent", http.StatusNotFound)
	return nil
}

// assistStatusFor keeps the assistant's refusals distinguishable, the same way
// the consent gate's are.
func assistStatusFor(err error) int {
	switch {
	case errors.Is(err, assist.ErrDisabled):
		return http.StatusNotFound
	case errors.Is(err, assist.ErrNotConfirmed):
		return http.StatusForbidden
	case errors.Is(err, assist.ErrInsecureEndpoint), errors.Is(err, assist.ErrTooLarge):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}
