package i18n

import (
	"encoding/json"
	"path"
	"testing"
)

func TestEveryCatalogCoversBaseKeys(t *testing.T) {
	base, err := read(Base)
	if err != nil {
		t.Fatalf("read base catalog: %v", err)
	}

	for _, lang := range Available() {
		if lang == Base {
			continue
		}
		t.Run(lang, func(t *testing.T) {
			messages, err := read(lang)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			for key := range base {
				if _, ok := messages[key]; !ok {
					t.Errorf("missing key %q", key)
				}
			}
			for key := range messages {
				if _, ok := base[key]; !ok {
					t.Errorf("key %q is not in the %s catalog", key, Base)
				}
			}
		})
	}
}

func TestCatalogsAreValidJSON(t *testing.T) {
	for _, lang := range Available() {
		data, err := localeFS.ReadFile(path.Join("locales", lang+".json"))
		if err != nil {
			t.Fatalf("read %s: %v", lang, err)
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			t.Errorf("%s: %v", lang, err)
		}
	}
}

func TestResolve(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "")

	tests := []struct {
		requested string
		want      string
	}{
		{"en", "en"},
		{"ms", "ms"},
		{"ms-MY", "ms"},
		{"ms_MY.UTF-8", "ms"},
		{"MS", "ms"},
		{"fr", "en"},
		{"", "en"},
	}
	for _, tc := range tests {
		if got := Resolve(tc.requested); got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.requested, got, tc.want)
		}
	}
}

func TestResolveFallsBackToEnvironment(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LANG", "ms_MY.UTF-8")

	if got := Resolve(""); got != "ms" {
		t.Errorf("Resolve(\"\") with LANG=ms_MY.UTF-8 = %q, want ms", got)
	}
}

func TestTranslateFallsBackThenSurfacesMissingKeys(t *testing.T) {
	b, err := Load("ms")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := b.T("severity.urgent"); got != "Mendesak" {
		t.Errorf("T(severity.urgent) = %q, want Mendesak", got)
	}
	if got := b.T("checks.does.not.exist"); got != "checks.does.not.exist" {
		t.Errorf("missing key = %q, want the key echoed back", got)
	}
	if got := b.T("agent.checks.available", 12, "Windows"); got == "" {
		t.Error("formatted message is empty")
	}
}
