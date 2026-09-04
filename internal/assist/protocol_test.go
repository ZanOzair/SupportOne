package assist

import (
	"strings"
	"testing"
)

func TestCheckEndpoint(t *testing.T) {
	allowed := []string{
		"https://api.example.com/v1/chat/completions",
		"https://127.0.0.1:8443/v1/chat/completions",
		// A local model server over plain HTTP: the traffic never leaves the
		// computer, and refusing it would push people to a hosted service.
		"http://127.0.0.1:11434/v1/chat/completions",
		"http://localhost:1234/v1/chat/completions",
		"http://[::1]:8080/v1/chat/completions",
	}
	for _, endpoint := range allowed {
		if err := CheckEndpoint(endpoint); err != nil {
			t.Errorf("CheckEndpoint(%q) = %v, want allowed", endpoint, err)
		}
	}

	refused := []string{
		"",
		"   ",
		"http://api.example.com/v1/chat/completions",
		"http://192.168.1.50:11434/v1/chat/completions",
		"ftp://example.com/",
		"file:///etc/passwd",
		"ws://example.com/",
	}
	for _, endpoint := range refused {
		if err := CheckEndpoint(endpoint); err == nil {
			t.Errorf("CheckEndpoint(%q) was allowed", endpoint)
		}
	}
}

func TestParseSuggestion(t *testing.T) {
	cases := []struct {
		name    string
		content string
		fixes   []string
		notes   string
	}{
		{
			"a clean object",
			`{"notes":"Disk is filling up.","fix_ids":["temp.clear"]}`,
			[]string{"temp.clear"}, "Disk is filling up.",
		},
		{
			"wrapped in a code fence, which models do constantly",
			"```json\n{\"notes\":\"Hello.\",\"fix_ids\":[]}\n```",
			nil, "Hello.",
		},
		{
			"prefaced with a sentence",
			"Sure! Here you go:\n{\"notes\":\"Hello.\",\"fix_ids\":[\"temp.clear\"]}",
			[]string{"temp.clear"}, "Hello.",
		},
		{
			// Prose with no JSON is a perfectly usable answer; it just names
			// no repairs.
			"prose only",
			"I could not find anything wrong with this computer.",
			nil, "I could not find anything wrong with this computer.",
		},
		{"nothing at all", "", nil, ""},
		{"only whitespace", "   \n\t ", nil, ""},
		{
			"a brace inside a string does not end the object",
			`{"notes":"a } brace","fix_ids":[]}`,
			nil, "a } brace",
		},
		{
			"an unterminated object falls back to prose",
			`{"notes":"oops"`,
			nil, `{"notes":"oops"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixIDs, notes := parseSuggestion(tc.content)
			if notes != tc.notes {
				t.Errorf("notes = %q, want %q", notes, tc.notes)
			}
			if len(fixIDs) != len(tc.fixes) {
				t.Fatalf("fixIDs = %v, want %v", fixIDs, tc.fixes)
			}
			for i := range tc.fixes {
				if fixIDs[i] != tc.fixes[i] {
					t.Errorf("fixIDs[%d] = %q, want %q", i, fixIDs[i], tc.fixes[i])
				}
			}
		})
	}
}

func TestFirstJSONObject(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                     `{"a":1}`,
		`prefix {"a":{"b":2}} suffix`: `{"a":{"b":2}}`,
		`{"a":"}"}`:                   `{"a":"}"}`,
		`{"a":"\""}`:                  `{"a":"\""}`,
		`no object here`:              ``,
		`{"unterminated":`:            ``,
	}

	for input, want := range cases {
		if got := firstJSONObject(input); got != want {
			t.Errorf("firstJSONObject(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestCleanNotesContainsTheOnePieceOfModelOutputWeDisplay: control characters
// are what would let a reply repaint a terminal.
func TestCleanNotesContainsTheOnePieceOfModelOutputWeDisplay(t *testing.T) {
	got := cleanNotes("Hello\x1b[31m red\x00 there\r\n")
	if strings.ContainsAny(got, "\x1b\x00\r") {
		t.Errorf("control characters survived: %q", got)
	}
	// Newlines and tabs are legitimate formatting and are kept.
	if got := cleanNotes("one\ntwo\tthree"); got != "one\ntwo\tthree" {
		t.Errorf("cleanNotes stripped legitimate whitespace: %q", got)
	}
}

func TestCleanNotesCapsAnEssay(t *testing.T) {
	got := cleanNotes(strings.Repeat("a", MaxNotes*2))
	if len(got) > MaxNotes+8 {
		t.Errorf("notes are %d bytes, want about %d", len(got), MaxNotes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("a truncated note does not say it was truncated")
	}
}

func TestCleanNotesCutsOnARuneBoundary(t *testing.T) {
	// A multi-byte run cut mid-rune would produce invalid text.
	got := cleanNotes(strings.Repeat("é", MaxNotes))
	for _, r := range got {
		if r == '�' {
			t.Fatalf("cleanNotes cut mid-rune: %q", got)
		}
	}
}

func TestCompletionContent(t *testing.T) {
	content, model, err := completionContent([]byte(`{"model":"m","choices":[{"message":{"content":"hi"}}]}`))
	if err != nil {
		t.Fatalf("completionContent: %v", err)
	}
	if content != "hi" || model != "m" {
		t.Errorf("content, model = %q, %q", content, model)
	}

	if _, _, err := completionContent([]byte(`{"choices":[]}`)); err == nil {
		t.Error("a reply with no choices was accepted")
	}
	if _, _, err := completionContent([]byte(`not json`)); err == nil {
		t.Error("a reply that is not JSON was accepted")
	}
}

// TestTheSystemPromptSaysWhatItMustSay is a reminder in test form: the prompt
// is not a security control, but it should still be honest about the shape of
// the answer and about not telling people to run things.
func TestTheSystemPromptSaysWhatItMustSay(t *testing.T) {
	for _, phrase := range []string{"fix_ids", "available_fixes", "Do not invent IDs", "Do not manufacture concerns"} {
		if !strings.Contains(systemPrompt, phrase) {
			t.Errorf("the system prompt no longer says %q", phrase)
		}
	}
}
