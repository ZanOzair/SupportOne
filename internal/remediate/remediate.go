// Package remediate is the gate every change to a machine passes through.
//
// Nothing here decides that a fix is a good idea; the user does. What this
// package guarantees is that they decided with the facts in front of them: a
// fix cannot be applied until its exact list of changes has been shown and
// confirmed back, and it cannot be applied at all if it was never described.
//
// The sequence is always the same. Plan describes the change, checks whether a
// restore point can be made, and runs the fix's own preflight. Apply refuses
// unless it is handed the token from that plan and an acknowledgement matching
// the changes it listed. Rollback undoes what was applied.
package remediate

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ZanOzair/SupportOne/internal/consent"
	"github.com/ZanOzair/SupportOne/internal/fixes"
	"github.com/ZanOzair/SupportOne/internal/platform"
	"github.com/ZanOzair/SupportOne/internal/restore"
)

// KeyDryRun is the outcome detail for a change that was described in full and
// deliberately not made.
const KeyDryRun = "fix.outcome.dry_run"

// Errors callers distinguish between.
var (
	// ErrNotConfirmed means Apply was called without a valid plan token, or
	// with an acknowledgement that did not match what the plan described.
	ErrNotConfirmed = errors.New("remediate: this change was not confirmed against what it would do")

	// ErrNeedsAdmin means the fix needs rights the agent does not hold.
	ErrNeedsAdmin = errors.New("remediate: this change needs administrator rights")

	// ErrNoRestorePoint means no restore point could be made and the user did
	// not accept going ahead without one.
	ErrNoRestorePoint = errors.New("remediate: no restore point is available and going ahead without one was not accepted")

	// ErrNotApplied means Rollback was asked to undo something this session
	// never did.
	ErrNotApplied = errors.New("remediate: this fix has not been applied in this session")
)

// Plan is what the user is shown before anything happens.
type Plan struct {
	FixID       string            `json:"fix_id"`
	Explanation fixes.Explanation `json:"explanation"`

	RequiresAdmin bool `json:"requires_admin"`

	// Elevated says whether the agent currently holds the rights the fix
	// needs. A fix that needs them and does not have them cannot be applied,
	// and the interface says so rather than offering a button that fails.
	Elevated bool `json:"elevated"`

	Reversible bool                 `json:"reversible"`
	Restore    restore.Availability `json:"restore"`

	// Blocked carries the fix's own refusal. A fix whose preflight says the
	// change is unsafe or pointless right now is described but not offered.
	Blocked string `json:"blocked,omitempty"`

	// Token proves this plan was shown. Apply will not act without it.
	Token string `json:"token"`

	// DryRun says the agent is in a mode where nothing will actually change.
	DryRun bool `json:"dry_run"`
}

// Applicable reports whether the plan can proceed to Apply.
func (p Plan) Applicable() bool {
	return p.Blocked == "" && (!p.RequiresAdmin || p.Elevated)
}

// Confirmation is the user's decision, carrying back what they were shown.
type Confirmation struct {
	Token string `json:"token"`

	// Acknowledged must repeat the plan's list of changes. A caller that
	// cannot reproduce the list did not show it, and the change is refused.
	Acknowledged []string `json:"acknowledged"`

	// AcceptWithoutRestorePoint is required when no restore point could be
	// made. Its whole purpose is that the user says it out loud.
	AcceptWithoutRestorePoint bool `json:"accept_without_restore_point"`
}

// Result is what happened.
type Result struct {
	Outcome fixes.Outcome `json:"outcome"`

	// RestorePoint is the point made before the change, when one was.
	RestorePoint *restore.Point `json:"restore_point,omitempty"`

	// Reversible repeats whether this change can be undone, so the record of
	// what happened carries it too.
	Reversible bool `json:"reversible"`
}

// Applier holds the registry, the audit log and the restore mechanism.
type Applier struct {
	Fixes   *fixes.Registry
	Audit   *consent.Log
	Restore restore.Maker
	OS      platform.OS

	// DryRun reports what would change without changing it. Preflight still
	// runs — that is read-only — but Apply never does.
	DryRun bool

	// Elevated reports whether the agent holds administrator rights. It is a
	// field rather than a direct call so tests can plan for a machine they are
	// not running on; production leaves it nil and platform.IsElevated answers.
	Elevated func() (bool, error)

	// Now is swappable in tests.
	Now func() time.Time

	mu      sync.Mutex
	plans   map[string]Plan
	applied map[string]Result
}

// New returns an applier over the given registry.
func New(registry *fixes.Registry, audit *consent.Log, maker restore.Maker, os platform.OS) *Applier {
	return &Applier{
		Fixes:   registry,
		Audit:   audit,
		Restore: maker,
		OS:      os,
		plans:   make(map[string]Plan),
		applied: make(map[string]Result),
	}
}

// elevated reports whether the agent holds administrator rights, treating an
// unanswerable question as "no": offering a change that will fail is worse
// than saying it needs rights we cannot confirm we have.
func (a *Applier) elevated() bool {
	ask := a.Elevated
	if ask == nil {
		ask = platform.IsElevated
	}
	ok, err := ask()
	return err == nil && ok
}

func (a *Applier) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Plan describes what applying a fix would do, without doing any of it.
//
// The fix is looked up by exact ID against the registry, which is the same
// whitelist the assistant's suggestions pass through: an ID that was never
// compiled in resolves to nothing here.
func (a *Applier) Plan(ctx context.Context, fixID string) (Plan, error) {
	fix, ok := a.Fixes.Get(fixID)
	if !ok || !fixes.RunsOn(fix, a.OS) {
		return Plan{}, fmt.Errorf("remediate: no fix with ID %q runs on %s", fixID, a.OS.Display())
	}

	elevated := a.elevated()

	plan := Plan{
		FixID:         fix.ID(),
		Explanation:   fix.Describe(),
		RequiresAdmin: fix.RequiresAdmin(),
		Elevated:      elevated,
		Reversible:    fix.Reversible(),
		DryRun:        a.DryRun,
	}
	if a.Restore != nil {
		plan.Restore = a.Restore.Check(ctx)
	}

	// Preflight is read-only: it decides whether the change makes sense right
	// now, and its refusal is part of what the user is shown.
	if err := fix.Preflight(ctx); err != nil {
		plan.Blocked = err.Error()
	}
	a.record(consent.EventFixPreflight, fix.ID(), map[string]string{
		"blocked":    fmt.Sprint(plan.Blocked != ""),
		"reversible": fmt.Sprint(plan.Reversible),
	})

	token, err := newToken()
	if err != nil {
		return Plan{}, err
	}
	plan.Token = token

	a.mu.Lock()
	a.plans[token] = plan
	a.mu.Unlock()

	a.record(consent.EventConsentAsked, fix.ID(), map[string]string{
		"changes":           fmt.Sprint(len(plan.Explanation.Changes)),
		"restore_available": fmt.Sprint(plan.Restore.Available),
	})
	return plan, nil
}

// Apply performs a change the user confirmed.
func (a *Applier) Apply(ctx context.Context, c Confirmation) (Result, error) {
	a.mu.Lock()
	plan, ok := a.plans[c.Token]
	if ok {
		// A plan is good for one decision. Reusing a token would let a second
		// change ride on a confirmation given for the first.
		delete(a.plans, c.Token)
	}
	a.mu.Unlock()

	if !ok {
		return Result{}, ErrNotConfirmed
	}
	if !acknowledges(plan.Explanation.Changes, c.Acknowledged) {
		a.record(consent.EventConsentDenied, plan.FixID, map[string]string{"reason": "acknowledgement did not match the described changes"})
		return Result{}, ErrNotConfirmed
	}
	if plan.Blocked != "" {
		return Result{}, fmt.Errorf("remediate: %s refused to run: %s", plan.FixID, plan.Blocked)
	}
	if plan.RequiresAdmin && !plan.Elevated {
		return Result{}, ErrNeedsAdmin
	}
	if !plan.Restore.Available && !c.AcceptWithoutRestorePoint {
		a.record(consent.EventConsentDenied, plan.FixID, map[string]string{"reason": "no restore point and none accepted"})
		return Result{}, ErrNoRestorePoint
	}

	fix, ok := a.Fixes.Get(plan.FixID)
	if !ok {
		return Result{}, fmt.Errorf("remediate: fix %q is no longer registered", plan.FixID)
	}

	a.record(consent.EventConsentGiven, plan.FixID, map[string]string{
		"dry_run":               fmt.Sprint(a.DryRun),
		"without_restore_point": fmt.Sprint(!plan.Restore.Available),
	})

	result := Result{Reversible: plan.Reversible}

	// A dry run stops here: the user has seen exactly what would happen, and
	// nothing has.
	if a.DryRun {
		result.Outcome = fixes.Outcome{
			FixID:     plan.FixID,
			Applied:   false,
			DryRun:    true,
			Detail:    KeyDryRun,
			StartedAt: a.now().UTC(),
		}
		return result, nil
	}

	if plan.Restore.Available && a.Restore != nil {
		point, err := a.Restore.Create(ctx, "SupportOne: before "+plan.FixID)
		if err != nil {
			// The restore point was offered and then could not be made. Going
			// ahead anyway would break the promise the user agreed to.
			a.record(consent.EventFixApplied, plan.FixID, map[string]string{
				"applied": "false",
				"reason":  "restore point could not be created",
			})
			return Result{}, fmt.Errorf("remediate: %s was not applied because its restore point could not be created: %w", plan.FixID, err)
		}
		result.RestorePoint = &point
		a.record(consent.EventFixApplied, "restore-point", map[string]string{
			"kind":      point.Kind,
			"reference": point.Reference,
		})
	}

	started := a.now().UTC()
	outcome, err := fix.Apply(ctx)
	outcome.FixID = plan.FixID
	outcome.StartedAt = started
	outcome.Duration = a.now().Sub(started)
	result.Outcome = outcome

	a.record(consent.EventFixApplied, plan.FixID, map[string]string{
		"applied":  fmt.Sprint(outcome.Applied),
		"duration": outcome.Duration.String(),
	})
	if err != nil {
		return result, fmt.Errorf("remediate: apply %s: %w", plan.FixID, err)
	}

	a.mu.Lock()
	a.applied[plan.FixID] = result
	a.mu.Unlock()
	return result, nil
}

// Rollback undoes a fix this session applied.
func (a *Applier) Rollback(ctx context.Context, fixID string) error {
	a.mu.Lock()
	_, wasApplied := a.applied[fixID]
	a.mu.Unlock()

	if !wasApplied {
		return ErrNotApplied
	}

	fix, ok := a.Fixes.Get(fixID)
	if !ok {
		return fmt.Errorf("remediate: fix %q is no longer registered", fixID)
	}

	if err := fix.Rollback(ctx); err != nil {
		a.record(consent.EventFixRolledBack, fixID, map[string]string{"undone": "false"})
		return fmt.Errorf("remediate: roll back %s: %w", fixID, err)
	}

	a.mu.Lock()
	delete(a.applied, fixID)
	a.mu.Unlock()

	a.record(consent.EventFixRolledBack, fixID, map[string]string{"undone": "true"})
	return nil
}

// Applied lists what this session changed and has not undone.
func (a *Applier) Applied() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := make([]string, 0, len(a.applied))
	for id := range a.applied {
		out = append(out, id)
	}
	return out
}

// acknowledges reports whether the user echoed back exactly the changes they
// were shown. Order and content must match: a caller that cannot reproduce the
// list did not display it.
func acknowledges(described, acknowledged []string) bool {
	if len(described) == 0 || len(described) != len(acknowledged) {
		return false
	}
	for i := range described {
		if described[i] != acknowledged[i] {
			return false
		}
	}
	return true
}

func newToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("remediate: generate plan token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (a *Applier) record(event consent.EventType, subject string, fields map[string]string) {
	if a.Audit == nil {
		return
	}
	_ = a.Audit.Append(consent.Event{Type: event, Subject: subject, Fields: fields})
}
