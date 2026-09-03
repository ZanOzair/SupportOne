package wizard

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)+$`)

// Registry holds the compiled-in wizards.
type Registry struct {
	mu      sync.RWMutex
	wizards map[string]*Wizard
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{wizards: make(map[string]*Wizard)}
}

// Register adds w, rejecting anything that could not be run or explained.
func (r *Registry) Register(w *Wizard) error {
	if w == nil {
		return fmt.Errorf("wizard: register nil wizard")
	}
	if !idPattern.MatchString(w.ID) {
		return fmt.Errorf("wizard: invalid wizard ID %q", w.ID)
	}
	if w.Title == "" || w.Complaint == "" {
		return fmt.Errorf("wizard: wizard %q must say what it is for before it can be offered", w.ID)
	}
	if len(w.Platforms) == 0 {
		return fmt.Errorf("wizard: wizard %q declares no platforms", w.ID)
	}
	for _, p := range w.Platforms {
		if !p.Valid() {
			return fmt.Errorf("wizard: wizard %q declares unknown platform %q", w.ID, p)
		}
	}
	if len(w.Steps) == 0 {
		return fmt.Errorf("wizard: wizard %q has no steps", w.ID)
	}

	seen := make(map[string]bool, len(w.Steps))
	for _, step := range w.Steps {
		switch {
		case !idPattern.MatchString(step.ID):
			return fmt.Errorf("wizard: %q has an invalid step ID %q", w.ID, step.ID)
		case seen[step.ID]:
			return fmt.Errorf("wizard: %q repeats step ID %q", w.ID, step.ID)
		case step.Title == "":
			return fmt.Errorf("wizard: %q step %q has no title", w.ID, step.ID)
		case step.Ask == nil:
			return fmt.Errorf("wizard: %q step %q asks nothing", w.ID, step.ID)
		case step.FixID == "" && step.Advice == "":
			// A step that finds a problem and then says nothing leaves the
			// user worse off than not asking.
			return fmt.Errorf("wizard: %q step %q offers neither a fix nor advice", w.ID, step.ID)
		}
		seen[step.ID] = true
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.wizards[w.ID]; exists {
		return fmt.Errorf("wizard: duplicate wizard ID %q", w.ID)
	}
	r.wizards[w.ID] = w
	return nil
}

// Get returns the wizard with the given ID.
func (r *Registry) Get(id string) (*Wizard, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.wizards[id]
	return w, ok
}

// All returns every registered wizard, ordered by ID.
func (r *Registry) All() []*Wizard {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Wizard, 0, len(r.wizards))
	for _, w := range r.wizards {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ForPlatform returns the wizards that run on os, ordered by ID.
func (r *Registry) ForPlatform(os platform.OS) []*Wizard {
	var out []*Wizard
	for _, w := range r.All() {
		if w.RunsOn(os) {
			out = append(out, w)
		}
	}
	return out
}

// Default is the registry the agent offers wizards from.
var Default = NewRegistry()

// MustRegister adds w to Default and panics if it cannot. Called from init().
func MustRegister(w *Wizard) {
	if err := Default.Register(w); err != nil {
		panic(err)
	}
}
