package assist

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/ZanOzair/SupportOne/internal/checks"
)

// systemPrompt is compiled in and never assembled from anything the machine
// reported. It states the one thing the model is allowed to do that has an
// effect, and the shape the answer must take.
//
// It is not a security control. The controls are that fix IDs are resolved
// against the registry and that prose is only ever displayed as prose — both
// of which hold whether or not the model pays this any attention.
const systemPrompt = `You are helping a non-technical person understand a diagnostic report from their own computer.

Reply with a single JSON object and nothing else:
{"notes": "<plain language, at most 150 words>", "fix_ids": ["<id>", ...]}

Rules:
- "notes" is for a person with no technical background. No jargon, no filenames, no commands.
- "fix_ids" may only contain IDs from the "available_fixes" list you were given. Do not invent IDs. An empty list is a good answer when nothing there applies.
- Do not suggest downloading anything, running any command, or editing any setting by hand.
- If the report shows nothing wrong, say so plainly. Do not manufacture concerns.`

// suggestion is the shape the model is asked for.
type suggestion struct {
	Notes  string   `json:"notes"`
	FixIDs []string `json:"fix_ids"`
}

// chatRequest is the OpenAI-shaped body. That shape is what Ollama,
// llama.cpp, LM Studio and most gateways already speak, so a local model needs
// no adapter and no key.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the part of the reply this reads. Everything else in it is
// ignored rather than trusted.
type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// request builds the body for a redacted snapshot.
//
// The snapshot goes in as data under a key, alongside the list of fix IDs this
// build actually carries — so a model that follows instructions has no reason
// to invent one, and a model that ignores them still cannot get an invented
// one past the registry.
func (a *Assistant) request(snap checks.Snapshot) ([]byte, error) {
	available := make([]string, 0)
	if a.fixes != nil {
		for _, f := range a.fixes.ForPlatform(a.os) {
			available = append(available, f.ID())
		}
	}

	content, err := json.MarshalIndent(map[string]any{
		"snapshot":        snap,
		"available_fixes": available,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("assist: build request content: %w", err)
	}

	body, err := json.MarshalIndent(chatRequest{
		Model: a.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(content)},
		},
		// Zero temperature: the same report should get the same answer, and a
		// diagnostic tool has no use for invention.
		Temperature: 0,
		MaxTokens:   800,
		Stream:      false,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("assist: build request: %w", err)
	}
	return body, nil
}

// completionContent pulls the assistant's message out of the response, and
// says plainly when there is nothing there rather than returning an empty
// answer that looks like a real one.
func completionContent(raw []byte) (content, model string, err error) {
	var res chatResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", "", fmt.Errorf("assist: the answer was not in the expected shape")
	}
	if len(res.Choices) == 0 {
		return "", res.Model, fmt.Errorf("assist: the answer contained no reply")
	}
	return res.Choices[0].Message.Content, res.Model, nil
}

// parseSuggestion reads the JSON object the model was asked for.
//
// Models wrap JSON in code fences, add a sentence before it, or return prose
// with no JSON at all. None of those is an error worth showing a user: the
// prose is kept as notes and the suggestion list is simply empty, which is a
// perfectly good answer.
func parseSuggestion(content string) (fixIDs []string, notes string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, ""
	}

	if object := firstJSONObject(trimmed); object != "" {
		var s suggestion
		if err := json.Unmarshal([]byte(object), &s); err == nil {
			return s.FixIDs, s.Notes
		}
	}

	// No parseable object: the whole reply is prose, and prose is displayable.
	return nil, trimmed
}

// firstJSONObject returns the outermost {...} span, so a fenced or prefaced
// object is still found. It counts braces outside string literals, which is
// enough for a response this small and avoids a dependency.
func firstJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}

	depth, inString, escaped := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// cleanNotes contains the one piece of model output shown to a person.
//
// It is stripped of control characters, which is what would otherwise let a
// reply repaint a terminal, and capped, because a model that answers with an
// essay does not get to fill the screen. It stays the model's words; nothing
// here tries to judge what they say.
func cleanNotes(notes string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, notes)

	cleaned = strings.TrimSpace(cleaned)
	if len(cleaned) > MaxNotes {
		// Cut on a rune boundary so the result is still valid text.
		cut := MaxNotes
		for cut > 0 && !utf8Boundary(cleaned, cut) {
			cut--
		}
		cleaned = strings.TrimSpace(cleaned[:cut]) + "…"
	}
	return cleaned
}

// utf8Boundary reports whether i starts a rune.
func utf8Boundary(s string, i int) bool {
	if i <= 0 || i >= len(s) {
		return true
	}
	return s[i]&0xC0 != 0x80
}
