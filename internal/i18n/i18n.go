// Package i18n resolves user-facing message keys to text in the user's
// language.
//
// Check results and fix explanations carry message keys, not English prose, so
// the same result renders in every supported language. Catalogs are embedded in
// the binary: translation works with no network and no files alongside the
// executable.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

//go:embed locales/*.json
var localeFS embed.FS

// Base is the language every catalog falls back to.
const Base = "en"

// Bundle resolves message keys for one language.
type Bundle struct {
	lang     string
	messages map[string]string
	fallback map[string]string
}

// Available returns the language tags with an embedded catalog, sorted.
func Available() []string {
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return []string{Base}
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
	}
	sort.Strings(out)
	return out
}

// Resolve maps a requested tag to an available language, falling back to the
// LANG/LC_ALL environment and finally to Base. A regional tag matches its base
// language, so "ms-MY" resolves to "ms".
func Resolve(requested string) string {
	for _, candidate := range []string{requested, os.Getenv("LC_ALL"), os.Getenv("LANG")} {
		if lang, ok := match(candidate); ok {
			return lang
		}
	}
	return Base
}

func match(tag string) (string, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return "", false
	}
	// Strip encoding ("ms_MY.UTF-8") and normalise separators.
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	tag = strings.ReplaceAll(tag, "_", "-")

	available := Available()
	for _, lang := range available {
		if tag == lang {
			return lang, true
		}
	}
	base, _, _ := strings.Cut(tag, "-")
	for _, lang := range available {
		if base == lang {
			return lang, true
		}
	}
	return "", false
}

// Load returns a bundle for lang, resolved through Resolve.
func Load(lang string) (*Bundle, error) {
	resolved := Resolve(lang)

	fallback, err := read(Base)
	if err != nil {
		return nil, err
	}
	messages := fallback
	if resolved != Base {
		if messages, err = read(resolved); err != nil {
			return nil, err
		}
	}
	return &Bundle{lang: resolved, messages: messages, fallback: fallback}, nil
}

func read(lang string) (map[string]string, error) {
	data, err := localeFS.ReadFile(path.Join("locales", lang+".json"))
	if err != nil {
		return nil, fmt.Errorf("i18n: read catalog %q: %w", lang, err)
	}
	var messages map[string]string
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("i18n: parse catalog %q: %w", lang, err)
	}
	return messages, nil
}

// Lang returns the language this bundle resolved to.
func (b *Bundle) Lang() string { return b.lang }

// T returns the message for key, formatted with args.
//
// An untranslated key falls back to English; a key missing everywhere returns
// the key itself, so a gap in the catalog is visible in the UI rather than
// silently rendering as empty space.
func (b *Bundle) T(key string, args ...any) string {
	msg, ok := b.messages[key]
	if !ok {
		if msg, ok = b.fallback[key]; !ok {
			return key
		}
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// Messages returns a copy of the catalog, with English filling any gap. The
// local interface renders check results in the browser, so it needs the same
// messages the Go side would use.
func (b *Bundle) Messages() map[string]string {
	out := make(map[string]string, len(b.fallback))
	for key, value := range b.fallback {
		out[key] = value
	}
	for key, value := range b.messages {
		out[key] = value
	}
	return out
}
