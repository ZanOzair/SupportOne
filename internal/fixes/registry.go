package fixes

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/ZanOzair/supportone/internal/platform"
)

var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)+$`)

// Registry holds the compiled-in fixes. Lookup is by exact ID only: this is
// the whitelist that keeps a suggested fix from ever being an arbitrary
// action.
type Registry struct {
	mu    sync.RWMutex
	fixes map[string]Fix
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{fixes: make(map[string]Fix)}
}

// Register adds f, rejecting malformed IDs, empty platform lists and
// duplicates.
func (r *Registry) Register(f Fix) error {
	if f == nil {
		return fmt.Errorf("fixes: register nil fix")
	}
	id := f.ID()
	if !idPattern.MatchString(id) {
		return fmt.Errorf("fixes: invalid fix ID %q", id)
	}
	if len(f.Platforms()) == 0 {
		return fmt.Errorf("fixes: fix %q declares no platforms", id)
	}
	for _, p := range f.Platforms() {
		if !p.Valid() {
			return fmt.Errorf("fixes: fix %q declares unknown platform %q", id, p)
		}
	}
	if f.Describe().Summary == "" || len(f.Describe().Changes) == 0 {
		return fmt.Errorf("fixes: fix %q must describe what it changes before it can be offered", id)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.fixes[id]; exists {
		return fmt.Errorf("fixes: duplicate fix ID %q", id)
	}
	r.fixes[id] = f
	return nil
}

// Get returns the fix with the given ID. Callers offer a fix to the user only
// via this lookup, so an ID that was never compiled in resolves to nothing.
func (r *Registry) Get(id string) (Fix, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.fixes[id]
	return f, ok
}

// All returns every registered fix, ordered by ID.
func (r *Registry) All() []Fix {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Fix, 0, len(r.fixes))
	for _, f := range r.fixes {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// ForPlatform returns the fixes that run on os, ordered by ID.
func (r *Registry) ForPlatform(os platform.OS) []Fix {
	var out []Fix
	for _, f := range r.All() {
		if RunsOn(f, os) {
			out = append(out, f)
		}
	}
	return out
}

// Resolve maps candidate IDs to registered fixes for the given platform and
// reports which candidates were discarded.
//
// This is the gate every suggestion passes through, including suggestions from
// the optional assistant: an ID that is not in the registry is dropped here and
// is never shown to the user, let alone run.
func (r *Registry) Resolve(candidates []string, os platform.OS) (known []Fix, discarded []string) {
	for _, id := range candidates {
		f, ok := r.Get(id)
		if !ok || !RunsOn(f, os) {
			discarded = append(discarded, id)
			continue
		}
		known = append(known, f)
	}
	return known, discarded
}

// Default is the registry the agent offers fixes from.
var Default = NewRegistry()

// MustRegister adds f to Default and panics if it cannot.
func MustRegister(f Fix) {
	if err := Default.Register(f); err != nil {
		panic(err)
	}
}
