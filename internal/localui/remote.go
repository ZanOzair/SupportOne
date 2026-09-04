package localui

import (
	"errors"
	"net/http"

	"github.com/ZanOzair/SupportOne/internal/remote"
)

// The remote-help routes.
//
// They wrap a tool the user already has; they are not a remote desktop. What
// this server contributes is the sentence before the session and the line in
// the audit log after it, and it says as much rather than implying it is
// watching over the session itself.

// remoteState is what the interface needs to draw the panel.
type remoteState struct {
	// Available says whether this build can offer remote help at all.
	Available bool `json:"available"`

	Tools []remote.Tool `json:"tools"`

	// Consequences is the list any session here is agreed against. It is sent
	// with the state so the panel can show it before anyone picks a tool.
	Consequences []string `json:"consequences"`

	// Session is the open one, or null. It is always present: a client that
	// reuses its state object would otherwise keep showing a session that
	// has ended, because the key simply stopped arriving.
	Session *remote.Session `json:"session"`
}

func (s *Server) handleRemoteState(w http.ResponseWriter, _ *http.Request) error {
	if s.cfg.Remote == nil {
		return writeJSON(w, remoteState{Consequences: remote.Consequences()})
	}

	state := remoteState{
		Available:    true,
		Tools:        s.cfg.Remote.Tools(),
		Consequences: remote.Consequences(),
	}
	if session, open := s.cfg.Remote.Current(); open {
		state.Session = &session
	}
	return writeJSON(w, state)
}

// handleRemotePlan says who is being let in and what that lets them do. It
// starts nothing.
func (s *Server) handleRemotePlan(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Remote == nil {
		return s.remoteUnavailable(w)
	}

	var body struct {
		Technician string `json:"technician"`
		ToolID     string `json:"tool_id"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}

	plan, err := s.cfg.Remote.Plan(body.Technician, body.ToolID)
	if err != nil {
		http.Error(w, err.Error(), remoteStatusFor(err))
		return nil
	}
	return writeJSON(w, plan)
}

// handleRemoteStart records the agreement and launches the tool.
//
// The gate is internal/remote's. All this handler does is carry the user's
// confirmation to it without editing it: a confirmation that does not repeat
// the plan is refused there, not here.
func (s *Server) handleRemoteStart(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Remote == nil {
		return s.remoteUnavailable(w)
	}

	var confirmation remote.Confirmation
	if err := decode(w, r, &confirmation); err != nil {
		return err
	}

	session, err := s.cfg.Remote.Start(r.Context(), confirmation)
	if err != nil {
		http.Error(w, err.Error(), remoteStatusFor(err))
		return nil
	}
	return writeJSON(w, session)
}

// handleRemoteDecline records that the user read the plan and said no.
func (s *Server) handleRemoteDecline(w http.ResponseWriter, _ *http.Request) error {
	if s.cfg.Remote == nil {
		return s.remoteUnavailable(w)
	}

	s.cfg.Remote.Decline()
	return writeJSON(w, map[string]string{"status": "declined"})
}

// handleRemoteEnd closes the record.
//
// It does not close the connection, because SupportOne has no way to. The
// interface says so next to the button.
func (s *Server) handleRemoteEnd(w http.ResponseWriter, _ *http.Request) error {
	if s.cfg.Remote == nil {
		return s.remoteUnavailable(w)
	}

	session, err := s.cfg.Remote.End()
	if err != nil {
		http.Error(w, err.Error(), remoteStatusFor(err))
		return nil
	}
	return writeJSON(w, session)
}

func (s *Server) remoteUnavailable(w http.ResponseWriter) error {
	http.Error(w, "this build cannot start a remote-help session", http.StatusNotFound)
	return nil
}

// remoteStatusFor maps the wrapper's refusals onto codes the interface can act
// on. A refused consent is not a server error and must not read like one.
func remoteStatusFor(err error) int {
	switch {
	case errors.Is(err, remote.ErrNotConfirmed):
		return http.StatusForbidden
	case errors.Is(err, remote.ErrNoTechnician):
		return http.StatusBadRequest
	case errors.Is(err, remote.ErrUnknownTool), errors.Is(err, remote.ErrToolNotInstalled), errors.Is(err, remote.ErrNoSession):
		return http.StatusNotFound
	default:
		return http.StatusConflict
	}
}
