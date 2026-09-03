// Package security reports the machine's baseline protections: whether the
// disk is encrypted, whether the firewall is on, and what antivirus the system
// says is running.
//
// It reports state. It does not scan for malware, and it is not an antivirus.
package security

import (
	"context"

	"github.com/ZanOzair/SupportOne/internal/checks"
	"github.com/ZanOzair/SupportOne/internal/platform"
)

// state is a tri-state answer plus a fourth value for protections that do not
// exist on a platform. Unknown is never collapsed into off: "we could not tell"
// and "it is switched off" are different findings.
type state string

const (
	stateOn            state = "on"
	stateOff           state = "off"
	stateUnknown       state = "unknown"
	stateNotApplicable state = "not_applicable"
)

// postureFacts is what every platform's collector produces.
type postureFacts struct {
	DiskEncryption state  `json:"disk_encryption"`
	Firewall       state  `json:"firewall"`
	Antivirus      state  `json:"antivirus"`
	AntivirusName  string `json:"antivirus_name,omitempty"`

	// Notes explain why something is unknown, in the collector's own words.
	Notes map[string]string `json:"notes,omitempty"`
}

// Message keys for this package's results.
const (
	keyPostureOK              = "check.security.posture.ok"
	keyPostureNoEncryption    = "check.security.posture.no_encryption"
	keyPostureNoFirewall      = "check.security.posture.no_firewall"
	keyPostureNoAntivirus     = "check.security.posture.no_antivirus"
	keyPostureSeveralOff      = "check.security.posture.several_off"
	keyPostureNothingReadable = "check.security.posture.unreadable"
)

type postureCheck struct{ run platform.Runner }

func (postureCheck) ID() string               { return "security.posture" }
func (postureCheck) Platforms() []platform.OS { return platform.All() }

// RequiresAdmin is false on purpose. Some of these answers need elevation on
// some platforms; the check reports what it can read and says plainly which
// answers it could not get, rather than demanding rights up front.
func (postureCheck) RequiresAdmin() bool { return false }

func (c postureCheck) Run(ctx context.Context) (checks.Result, error) {
	facts, err := collectPosture(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	return postureVerdict(facts), nil
}

// postureVerdict weighs the three protections. It is separate from collection
// so the rules can be tested without a machine in each state.
func postureVerdict(facts postureFacts) checks.Result {
	detail := map[string]any{
		"disk_encryption": string(facts.DiskEncryption),
		"firewall":        string(facts.Firewall),
		"antivirus":       string(facts.Antivirus),
	}
	if facts.AntivirusName != "" {
		detail["antivirus_name"] = facts.AntivirusName
	}
	if len(facts.Notes) > 0 {
		detail["notes"] = facts.Notes
	}

	var off []state
	if facts.DiskEncryption == stateOff {
		off = append(off, facts.DiskEncryption)
	}
	if facts.Firewall == stateOff {
		off = append(off, facts.Firewall)
	}
	if facts.Antivirus == stateOff {
		off = append(off, facts.Antivirus)
	}

	if len(off) > 1 {
		return checks.Attention(keyPostureSeveralOff, len(off)).With(detail)
	}
	switch {
	case facts.DiskEncryption == stateOff:
		return checks.Attention(keyPostureNoEncryption).With(detail)
	case facts.Firewall == stateOff:
		return checks.Attention(keyPostureNoFirewall).With(detail)
	case facts.Antivirus == stateOff:
		return checks.Attention(keyPostureNoAntivirus).With(detail)
	}

	// Nothing is switched off, but that is only reassuring if something was
	// actually readable.
	if facts.DiskEncryption == stateUnknown && facts.Firewall == stateUnknown &&
		(facts.Antivirus == stateUnknown || facts.Antivirus == stateNotApplicable) {
		return checks.Unknown(keyPostureNothingReadable).With(detail)
	}
	return checks.OK(keyPostureOK).With(detail)
}

func init() {
	checks.MustRegister(postureCheck{run: platform.RunRead})
}
