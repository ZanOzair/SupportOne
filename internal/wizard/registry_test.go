package wizard

import (
	"context"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/platform"
)

func ok(context.Context) (Finding, error) { return Finding{OK: true}, nil }

func valid() *Wizard {
	return &Wizard{
		ID: "wizard.stub", Title: "title.key", Complaint: "complaint.key",
		Platforms: platform.All(),
		Steps:     []Step{{ID: "stub.one", Title: "one.key", Ask: ok, Advice: "advice.key"}},
	}
}

func TestRegisterAcceptsAWizardThatCanBeRunAndExplained(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(valid()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := r.Get("wizard.stub"); !ok {
		t.Error("the wizard is not reachable by its ID")
	}
	if len(r.All()) != 1 {
		t.Errorf("All returned %d wizards, want 1", len(r.All()))
	}
	if len(r.ForPlatform(platform.Linux)) != 1 {
		t.Error("the wizard is not offered on a platform it declares")
	}
}

func TestRegisterRefusesWhatCannotBeRunOrExplained(t *testing.T) {
	cases := map[string]func(*Wizard){
		"no ID":                    func(w *Wizard) { w.ID = "" },
		"an ID that is not dotted": func(w *Wizard) { w.ID = "wizard" },
		"an ID with a space":       func(w *Wizard) { w.ID = "wizard. stub" },
		"no title":                 func(w *Wizard) { w.Title = "" },
		"no complaint":             func(w *Wizard) { w.Complaint = "" },
		"no platforms":             func(w *Wizard) { w.Platforms = nil },
		"an unknown platform":      func(w *Wizard) { w.Platforms = []platform.OS{"plan9"} },
		"no steps":                 func(w *Wizard) { w.Steps = nil },
		"a step with no ID":        func(w *Wizard) { w.Steps[0].ID = "" },
		"a step with no title":     func(w *Wizard) { w.Steps[0].Title = "" },
		"a step that asks nothing": func(w *Wizard) { w.Steps[0].Ask = nil },
		"a step that offers nothing": func(w *Wizard) {
			w.Steps[0].Advice, w.Steps[0].FixID = "", ""
		},
		"a repeated step ID": func(w *Wizard) {
			w.Steps = append(w.Steps, w.Steps[0])
		},
	}

	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			w := valid()
			break_(w)
			if err := NewRegistry().Register(w); err == nil {
				t.Error("Register accepted it")
			}
		})
	}

	if err := NewRegistry().Register(nil); err == nil {
		t.Error("Register accepted a nil wizard")
	}
}

func TestRegisterRefusesADuplicateID(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(valid()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(valid()); err == nil {
		t.Error("Register accepted a second wizard with the same ID")
	}
}

func TestForPlatformOffersOnlyWhatRunsThere(t *testing.T) {
	r := NewRegistry()

	windowsOnly := valid()
	windowsOnly.ID = "wizard.windows"
	windowsOnly.Platforms = []platform.OS{platform.Windows}

	if err := r.Register(valid()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(windowsOnly); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, w := range r.ForPlatform(platform.Linux) {
		if w.ID == "wizard.windows" {
			t.Error("a Windows-only wizard is offered on Linux")
		}
	}
	if len(r.ForPlatform(platform.Windows)) != 2 {
		t.Error("a wizard that runs on Windows is missing from the Windows list")
	}
}
