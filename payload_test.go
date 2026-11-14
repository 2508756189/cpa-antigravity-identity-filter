package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPayloadShapesReplaceOnlyTheFirstSystemText(t *testing.T) {
	identity := "You are Codex, a coding agent based on GPT-5."
	cfg := defaultConfig()
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "responses instructions",
			body: `{"instructions":"You are Codex, a coding agent based on GPT-5.\n\nkeep","input":[{"role":"developer","content":"You are Claude Code, Anthropic's official CLI for Claude."}]}`,
			want: identityCodingBlock + "\n\nkeep",
		},
		{
			name: "responses empty instructions falls through",
			body: `{"instructions":"","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"You are Codex, a coding agent based on GPT-5.\n\nkeep"}]}]}`,
			want: identityCodingBlock + "\n\nkeep",
		},
		{
			name: "chat string",
			body: `{"messages":[{"role":"system","content":"You are Hermes Agent, built by Nous Research."},{"role":"user","content":"You are Codex, hi"}]}`,
			want: identityCodingBlock,
		},
		{
			name: "chat developer after user",
			body: `{"messages":[{"role":"user","content":"You are Codex, hi"},{"role":"developer","content":"You are ZCode, an interactive agent.\n\n# Harness\nstay"}]}`,
			want: identityCodingBlock + "\n\n# Harness\nstay",
		},
		{
			name: "chat text parts",
			body: `{"messages":[{"role":"system","content":[{"type":"text","text":"You are OpenCode, the best coding agent on the planet."},{"type":"text","text":"second block You are OpenCode, stay"}]},{"role":"user","content":"hi"}]}`,
			want: identityCodingBlock,
		},
		{
			name: "anthropic string",
			body: `{"system":"You are a Claude agent, built on Anthropic's Claude Agent SDK.","messages":[{"role":"user","content":"hi"}]}`,
			want: identityCodingBlock,
		},
		{
			name: "anthropic blocks",
			body: `{"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude.","cache_control":{"type":"ephemeral"}},{"type":"text","text":"second You are Claude Code, stay"}],"messages":[{"role":"user","content":"hi"}]}`,
			want: identityCodingBlock,
		},
		{
			name: "gemini systemInstruction",
			body: `{"systemInstruction":{"parts":[{"text":"You are ZCode, an interactive coding agent.\n\n# Harness\nstay"},{"text":"second part"}]},"contents":[{"role":"user","parts":[{"text":"You are Codex, hi"}]}]}`,
			want: identityCodingBlock + "\n\n# Harness\nstay",
		},
		{
			name: "gemini snake case",
			body: `{"system_instruction":{"parts":[{"text":"You are OpenClaw, a personal assistant."}]}}`,
			want: identityCodingBlock,
		},
		{
			name: "antigravity request wrapper",
			body: `{"request":{"systemInstruction":{"parts":[{"text":"You are Hermes Agent, an intelligent AI assistant created by Nous Research."}]},"contents":[{"role":"user","parts":[{"text":"You are Codex, hi"}]}]}}`,
			want: identityCodingBlock,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := []byte(tt.body)
			got, changed := rewriteBody(before, cfg)
			if !changed {
				t.Fatalf("expected rewrite: %s", tt.body)
			}
			assertOutsideStringUnchanged(t, before, got)
			if decodeFirst(t, got) != tt.want {
				t.Fatalf("decoded = %q\nwant %q", decodeFirst(t, got), tt.want)
			}
			if bytes.Contains(before, []byte(identity)) && strings.Contains(tt.want, identity) {
				t.Fatal("test expected the identity sentence to be removed")
			}
			again, changedAgain := rewriteBody(got, cfg)
			if changedAgain || !bytes.Equal(again, got) {
				t.Fatal("rewrite is not idempotent")
			}
		})
	}
}

func TestUntouchedRegionsStayByteIdentical(t *testing.T) {
	cfg := defaultConfig()
	body := []byte(`{ "n" : -1.5e+10 , "ok" : true , "no" : false , "z" : null , "arr" : [1,{"a":"Codex"}] , "system" : "You are Codex, a coding agent based on GPT-5.\n\nkeep \u212a and \u0130 and \"q\" and .codex/$CODEX_HOME and mcp__codex_app" , "obj" : {"system":"You are Codex, hidden"} , "tail" : "You are Codex," }`)
	got, changed := rewriteBody(body, cfg)
	if !changed {
		t.Fatal("expected rewrite")
	}
	assertOutsideStringUnchanged(t, body, got)
	for _, marker := range []string{`\u212a`, `\u0130`, `\"q\"`, `.codex/$CODEX_HOME`, `mcp__codex_app`, `"obj" : {"system":"You are Codex, hidden"}`, `"tail" : "You are Codex,"`} {
		if !bytes.Contains(got, []byte(marker)) {
			t.Fatalf("missing preserved marker %s\n%s", marker, got)
		}
	}
	if !utf8.Valid(got) {
		t.Fatal("rewritten JSON is not valid UTF-8")
	}
	decoded := decodeFirst(t, got)
	if !strings.HasPrefix(decoded, identityCodingBlock) {
		t.Fatalf("decoded prefix = %q", decoded)
	}
	if !strings.Contains(decoded, "keep K and İ and \"q\" and .codex/$CODEX_HOME and mcp__codex_app") {
		t.Fatalf("decoded suffix changed: %q", decoded)
	}
}

func TestRawUTF8KelvinAndDottedI(t *testing.T) {
	cfg := defaultConfig()
	const kelvin = "\u212A"
	const dotted = "\u0130"
	unchanged := []byte("{\"system\":\"you are codexify " + kelvin + dotted + "\"}")
	got, changed := rewriteBody(append([]byte(nil), unchanged...), cfg)
	if changed || !bytes.Equal(got, unchanged) {
		t.Fatalf("non-matching UTF-8 text changed: %q", got)
	}
	if !utf8.Valid(got) {
		t.Fatal("invalid UTF-8")
	}

	matching := []byte("{\"system\":\"You are Codex, a coding agent based on GPT-5.\\n\\nkeep " + kelvin + dotted + "\"}")
	got, changed = rewriteBody(append([]byte(nil), matching...), cfg)
	if !changed {
		t.Fatal("expected rewrite")
	}
	assertOutsideStringUnchanged(t, matching, got)
	if !bytes.Contains(got, []byte(kelvin+dotted)) {
		t.Fatalf("raw UTF-8 suffix was re-encoded: %q", got)
	}
	if !utf8.Valid(got) {
		t.Fatal("rewritten JSON is not valid UTF-8")
	}
}

func TestSecondDeveloperMessageAndUserTextStay(t *testing.T) {
	cfg := defaultConfig()
	body := []byte(`{"instructions":"","input":[{"role":"developer","content":"You are Codex, a coding agent based on GPT-5.\n\n# Personality\n\nAs Codex, first."},{"role":"developer","content":"You are Codex, a coding agent based on GPT-5.\nAs Codex, second."},{"role":"user","content":"You are Codex, user."}]}`)
	got, changed := rewriteBody(body, cfg)
	if !changed {
		t.Fatal("expected rewrite")
	}
	assertOutsideStringUnchanged(t, body, got)
	var parsed struct {
		Input []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Input[0].Content != identityCodingBlock+"\n\n# Personality\n\nAs Antigravity, first." {
		t.Fatalf("first = %q", parsed.Input[0].Content)
	}
	if parsed.Input[1].Content != "You are Codex, a coding agent based on GPT-5.\nAs Codex, second." {
		t.Fatalf("second = %q", parsed.Input[1].Content)
	}
	if parsed.Input[2].Content != "You are Codex, user." {
		t.Fatalf("user = %q", parsed.Input[2].Content)
	}
}

func TestAnthropicSystemWinsOverLaterDeveloperMessage(t *testing.T) {
	cfg := defaultConfig()
	body := []byte(`{"system":"You are Claude Code, Anthropic's official CLI for Claude.","messages":[{"role":"user","content":"hi"},{"role":"developer","content":"You are Codex, a coding agent based on GPT-5."}]}`)
	got, changed := rewriteBody(body, cfg)
	if !changed {
		t.Fatal("expected rewrite")
	}
	assertOutsideStringUnchanged(t, body, got)
	if !bytes.Contains(got, []byte("You are Codex, a coding agent based on GPT-5.")) {
		t.Fatalf("later developer message changed: %s", got)
	}
	if decodeFirst(t, got) != identityCodingBlock {
		t.Fatalf("system = %q", decodeFirst(t, got))
	}
}

func TestNonObjectAndInvalidJSONAreUnchanged(t *testing.T) {
	cfg := defaultConfig()
	samples := [][]byte{
		nil,
		[]byte(""),
		[]byte("{"),
		[]byte("[]"),
		[]byte(`[{"system":"You are Codex, a coding agent based on GPT-5."}]`),
		[]byte(`"You are Codex, a coding agent based on GPT-5."`),
		[]byte(`{"system":`),
	}
	for _, sample := range samples {
		original := append([]byte(nil), sample...)
		got, changed := rewriteBody(sample, cfg)
		if changed || !bytes.Equal(got, original) || !bytes.Equal(sample, original) {
			t.Fatalf("sample %q changed to %q", sample, got)
		}
	}
}

func TestNonAntigravityTargetIsByteIdentical(t *testing.T) {
	cfg := defaultConfig()
	body := []byte(`{"system":"You are Codex, a coding agent based on GPT-5.\n\nkeep"}`)
	for _, format := range []string{"", "codex", "claude", "gemini", "openai", "openai-response", "interactions"} {
		original := append([]byte(nil), body...)
		got, changed := filterPayload(format, body, cfg)
		if changed || !bytes.Equal(got, original) || !bytes.Equal(body, original) {
			t.Fatalf("format %q changed the payload", format)
		}
		if len(got) > 0 && &got[0] != &body[0] {
			t.Fatalf("format %q copied the payload", format)
		}
	}
	got, changed := filterPayload("  Antigravity  ", body, cfg)
	if !changed || !bytes.Contains(got, []byte("You are Antigravity,")) {
		t.Fatal("trimmed mixed-case antigravity target should rewrite")
	}
}

func assertOutsideStringUnchanged(t *testing.T, before, after []byte) {
	t.Helper()
	span, ok := findFirstSystemSpan(before)
	if !ok {
		t.Fatal("first system span not found")
	}
	if !bytes.Equal(before[:span.contentStart], after[:span.contentStart]) {
		t.Fatalf("bytes before the system text changed\nbefore %q\nafter  %q", before[:span.contentStart], after[:span.contentStart])
	}
	delta := len(after) - len(before)
	suffixAt := span.contentEnd + delta
	if suffixAt < 0 || suffixAt > len(after) || !bytes.Equal(before[span.contentEnd:], after[suffixAt:]) {
		t.Fatalf("bytes after the system text changed\nbefore %q\nafter  %q", before[span.contentEnd:], after[suffixAt:])
	}
}

func decodeFirst(t *testing.T, body []byte) string {
	t.Helper()
	span, ok := findFirstSystemSpan(body)
	if !ok {
		t.Fatal("first system span not found")
	}
	decoded, _, err := decodeJSONString(body[span.contentStart:span.contentEnd])
	if err != nil {
		t.Fatalf("decode system text: %v", err)
	}
	return decoded
}
