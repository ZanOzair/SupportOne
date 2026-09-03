// Package startup lists what the machine launches by itself when someone logs
// in.
//
// The check reports the inventory. It does not judge it: whether a startup item
// is slowing the machine down is a question for the performance analyser, and
// calling a list of programs a "problem" because it is long is exactly the kind
// of manufactured urgency this product refuses.
package startup

import (
	"context"

	"github.com/ZanOzair/supportone/internal/checks"
	"github.com/ZanOzair/supportone/internal/platform"
)

// item is one program or service configured to start on its own.
type item struct {
	Name     string `json:"name"`
	Command  string `json:"command,omitempty"`
	Location string `json:"location,omitempty"`

	// Scope is "user" for something that starts for this account only, and
	// "system" for something that starts for everyone.
	Scope string `json:"scope"`
}

const (
	scopeUser   = "user"
	scopeSystem = "system"
)

// Message keys for this package's results.
const (
	keyStartupOK   = "check.startup.items.ok"
	keyStartupNone = "check.startup.items.none"
)

type itemsCheck struct{ run platform.Runner }

func (itemsCheck) ID() string               { return "startup.items" }
func (itemsCheck) Platforms() []platform.OS { return platform.All() }
func (itemsCheck) RequiresAdmin() bool      { return false }

func (c itemsCheck) Run(ctx context.Context) (checks.Result, error) {
	items, err := collectStartupItems(ctx, c.run)
	if err != nil {
		return checks.UnknownFor(err), nil
	}
	if len(items) == 0 {
		return checks.OK(keyStartupNone), nil
	}
	return checks.OK(checks.PluralKey(keyStartupOK, len(items)), len(items)).With(map[string]any{"items": items}), nil
}

func init() {
	checks.MustRegister(itemsCheck{run: platform.RunRead})
}
