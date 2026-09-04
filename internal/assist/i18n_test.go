package assist

import (
	"strings"
	"testing"

	"github.com/ZanOzair/SupportOne/internal/i18n"
)

// TestTheEgressScreenIsTranslated covers the strings shown around the one
// outbound connection this agent makes. A bare key here would appear on the
// screen where a person decides whether to send their machine's report
// somewhere, which is the worst place for one.
func TestTheEgressScreenIsTranslated(t *testing.T) {
	bundle, err := i18n.Load(i18n.Base)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	keys := []string{
		"ui.assist.heading", "ui.assist.note", "ui.assist.prepare",
		"ui.assist.destination", "ui.assist.size", "ui.assist.redacted",
		"ui.assist.not_redacted", "ui.assist.payload", "ui.assist.send",
		"ui.assist.cancel", "ui.assist.answer_from", "ui.assist.suggested",
		"ui.assist.discarded", "ui.assist.caveat", "ui.assist.unnamed_model",

		"agent.assist.heading", "agent.assist.destination", "agent.assist.size",
		"agent.assist.redacted", "agent.assist.payload", "agent.assist.confirm",
		"agent.assist.not_sent", "agent.assist.answer_from", "agent.assist.suggested",
		"agent.assist.discarded", "agent.assist.caveat", "agent.assist.unnamed_model",
	}
	for _, key := range keys {
		if got := bundle.T(key); got == key {
			t.Errorf("message key %q has no translation", key)
		}
	}
}

// TestEveryCatalogCarriesTheEgressScreen keeps a language from falling back to
// English halfway through the one screen where precision matters most.
func TestEveryCatalogCarriesTheEgressScreen(t *testing.T) {
	base, err := i18n.Load(i18n.Base)
	if err != nil {
		t.Fatalf("load base catalog: %v", err)
	}

	for _, lang := range i18n.Available() {
		if lang == i18n.Base {
			continue
		}
		bundle, err := i18n.Load(lang)
		if err != nil {
			t.Fatalf("load %s: %v", lang, err)
		}

		messages := bundle.Messages()
		for key := range base.Messages() {
			if !strings.HasPrefix(key, "ui.assist.") && !strings.HasPrefix(key, "agent.assist.") {
				continue
			}
			if _, ok := messages[key]; !ok {
				t.Errorf("%s is missing %q", lang, key)
			}
		}
	}
}
