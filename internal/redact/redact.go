// Package redact removes identifying detail from a snapshot before the user
// sends or saves it.
//
// A diagnostic snapshot is useful precisely because it is specific, and that
// specificity is what makes it sensitive: hostnames, usernames, serial numbers
// and addresses describe a real machine and the person using it. The user
// decides what leaves their computer, and this package is how they take
// something out.
package redact

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

// Marker replaces a redacted value. It is deliberately visible: the reader of a
// report should see that something was removed, not wonder whether the field
// was ever there.
const Marker = "[redacted]"

// Policy says what to remove. The zero policy removes nothing, so redaction is
// always an explicit choice.
type Policy struct {
	Hostnames bool `json:"hostnames"`
	Usernames bool `json:"usernames"`
	Serials   bool `json:"serials"`
	Addresses bool `json:"addresses"`
}

// Everything is the policy the send flow offers first: the most protective
// starting point, which the user can then relax.
func Everything() Policy {
	return Policy{Hostnames: true, Usernames: true, Serials: true, Addresses: true}
}

// Nothing reports whether the policy would leave the snapshot untouched.
func (p Policy) Nothing() bool {
	return !p.Hostnames && !p.Usernames && !p.Serials && !p.Addresses
}

// Identity holds the machine-specific strings to look for. It is resolved from
// the running machine by CurrentIdentity, and set explicitly in tests.
type Identity struct {
	Hostname string
	Username string
	HomeDir  string
}

// CurrentIdentity reads the names this machine is known by. A name it cannot
// read is simply left out; redaction still applies to the patterns it can match
// on shape alone.
func CurrentIdentity() Identity {
	var id Identity
	if hostname, err := os.Hostname(); err == nil {
		id.Hostname = hostname
	}
	if home, err := os.UserHomeDir(); err == nil {
		id.HomeDir = home
		id.Username = filepath.Base(home)
	}
	if user := os.Getenv("USER"); user != "" {
		id.Username = user
	} else if user := os.Getenv("USERNAME"); user != "" {
		id.Username = user
	}
	return id
}

// Patterns that identify a value by its shape rather than by its field name.
var (
	macPattern  = regexp.MustCompile(`\b[0-9a-fA-F]{2}(?::[0-9a-fA-F]{2}){5}\b`)
	ipv4Pattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b(?:/\d{1,2})?`)
	ipv6Pattern = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{1,4}\b(?:/\d{1,3})?`)
)

// Field names whose values are identifying whatever they contain.
var (
	serialFields   = map[string]bool{"serial": true, "serial_number": true, "serialnumber": true, "uuid": true}
	hostnameFields = map[string]bool{"hostname": true, "host": true, "computer_name": true}
	usernameFields = map[string]bool{"user": true, "username": true, "owner": true}
	addressFields  = map[string]bool{"mac": true, "addresses": true, "gateway": true, "dns": true, "ip": true}
)

// Snapshot returns a copy of snap with the policy applied. The original is not
// modified, so the on-screen report can keep showing the full detail while the
// user decides what to send.
func (p Policy) Snapshot(snap checks.Snapshot, id Identity) (checks.Snapshot, error) {
	if p.Nothing() {
		return snap, nil
	}

	out := snap
	out.Results = make([]checks.Result, len(snap.Results))
	copy(out.Results, snap.Results)

	for i, res := range out.Results {
		if len(res.Detail) > 0 {
			detail, err := p.detail(res.Detail, id)
			if err != nil {
				return checks.Snapshot{}, fmt.Errorf("redact %s: %w", res.CheckID, err)
			}
			out.Results[i].Detail = detail
		}

		if len(res.Args) > 0 {
			args := make([]any, len(res.Args))
			for j, arg := range res.Args {
				if s, ok := arg.(string); ok {
					args[j] = p.text(s, id)
					continue
				}
				args[j] = arg
			}
			out.Results[i].Args = args
		}

		// An error message can carry a path, and a path carries a username.
		out.Results[i].Err = p.text(res.Err, id)
	}
	return out, nil
}

// detail applies the policy to one result's structured evidence. It walks the
// JSON shape of the value, so a check can put any struct in Detail without
// teaching this package about it.
func (p Policy) detail(detail map[string]any, id Identity) (map[string]any, error) {
	encoded, err := json.Marshal(detail)
	if err != nil {
		return nil, err
	}
	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return nil, err
	}

	walked, ok := p.walk(generic, "", id).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("redacted detail changed shape")
	}
	return walked, nil
}

// walk redacts a decoded JSON value, using the field name it arrived under to
// catch values whose shape alone would not give them away.
func (p Policy) walk(value any, field string, id Identity) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			out[key] = p.walk(child, strings.ToLower(key), id)
		}
		return out

	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = p.walk(child, field, id)
		}
		return out

	case string:
		if p.fieldIsRedacted(field) {
			return Marker
		}
		return p.text(v, id)

	default:
		return value
	}
}

func (p Policy) fieldIsRedacted(field string) bool {
	switch {
	case p.Serials && serialFields[field]:
		return true
	case p.Hostnames && hostnameFields[field]:
		return true
	case p.Usernames && usernameFields[field]:
		return true
	case p.Addresses && addressFields[field]:
		return true
	default:
		return false
	}
}

// text redacts the identifying substrings of a free-text value.
func (p Policy) text(s string, id Identity) string {
	if s == "" {
		return s
	}

	if p.Usernames {
		// The home directory is replaced before the username, so a path is
		// redacted as a whole rather than leaving a hollow prefix behind.
		s = replaceFold(s, id.HomeDir, Marker)
		s = replaceFold(s, id.Username, Marker)
	}
	if p.Hostnames {
		s = replaceFold(s, id.Hostname, Marker)
	}
	if p.Addresses {
		s = macPattern.ReplaceAllString(s, Marker)
		s = ipv6Pattern.ReplaceAllString(s, Marker)
		s = ipv4Pattern.ReplaceAllString(s, Marker)
	}
	return s
}

// replaceFold replaces every case-insensitive occurrence of old. Windows
// reports the same account name in several cases, and a report should not leak
// a username because it was capitalised differently.
func replaceFold(s, old, replacement string) string {
	// Two characters is not a name worth matching; replacing it would mangle
	// unrelated text.
	if len(old) < 3 {
		return s
	}

	var b strings.Builder
	lower, lowerOld := strings.ToLower(s), strings.ToLower(old)
	for {
		i := strings.Index(lower, lowerOld)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString(replacement)
		s, lower = s[i+len(old):], lower[i+len(lowerOld):]
	}
}

// jsonEncode writes a snapshot as JSON. It exists so tests can compare
// snapshots by their serialised form, which is the form that actually leaves
// the machine.
func jsonEncode(w io.Writer, snap checks.Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(snap)
}
