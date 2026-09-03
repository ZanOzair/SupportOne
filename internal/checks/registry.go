package checks

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

// idPattern constrains check IDs to lowercase dotted segments, e.g.
// "disk.smart" or "network.config". Stable IDs are what the report, the
// explainer and the audit log join on.
var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)+$`)

// Registry holds the compiled-in checks. Adding a check means registering one
// more implementation from its own file; it never means editing this file.
type Registry struct {
	mu     sync.RWMutex
	checks map[string]Check
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{checks: make(map[string]Check)}
}

// Register adds c. It fails on a malformed ID, an empty platform list, or an
// ID already taken — all of which are programming errors caught by tests.
func (r *Registry) Register(c Check) error {
	if c == nil {
		return fmt.Errorf("checks: register nil check")
	}
	id := c.ID()
	if !idPattern.MatchString(id) {
		return fmt.Errorf("checks: invalid check ID %q", id)
	}
	if len(c.Platforms()) == 0 {
		return fmt.Errorf("checks: check %q declares no platforms", id)
	}
	for _, p := range c.Platforms() {
		if !p.Valid() {
			return fmt.Errorf("checks: check %q declares unknown platform %q", id, p)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.checks[id]; exists {
		return fmt.Errorf("checks: duplicate check ID %q", id)
	}
	r.checks[id] = c
	return nil
}

// Get returns the check with the given ID.
func (r *Registry) Get(id string) (Check, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.checks[id]
	return c, ok
}

// All returns every registered check, ordered by ID.
func (r *Registry) All() []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Check, 0, len(r.checks))
	for _, c := range r.checks {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// ForPlatform returns the checks that run on os, ordered by ID.
func (r *Registry) ForPlatform(os platform.OS) []Check {
	var out []Check
	for _, c := range r.All() {
		if RunsOn(c, os) {
			out = append(out, c)
		}
	}
	return out
}

// Default is the registry the agent runs. Platform packages add themselves to
// it from init(), so a build for one OS carries only the checks that OS can
// honestly answer.
var Default = NewRegistry()

// MustRegister adds c to Default and panics if it cannot. Called from init();
// a failure here is a build-time mistake, not a runtime condition.
func MustRegister(c Check) {
	if err := Default.Register(c); err != nil {
		panic(err)
	}
}
