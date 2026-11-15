package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSanitizedCodexResponsesFixture(t *testing.T) {
	original, err := os.ReadFile("testdata/codex_responses_sanitized.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if bytes.Contains(original, []byte("C:/Users")) || bytes.Contains(original, []byte("/root")) {
		t.Fatal("fixture still contains a private path")
	}

	cfg := defaultConfig()
	got, changed := filterPayload("antigravity", original, cfg)
	if !changed {
		t.Fatal("expected the Codex fixture to be rewritten")
	}
	assertOutsideStringUnchanged(t, original, got)

	var parsed struct {
		Instructions string `json:"instructions"`
		Input        []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"input"`
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, got)
	}
	first := parsed.Input[0].Content[0].Text
	if !strings.HasPrefix(first, identityCodingBlock+"\n\n# When to ask the user for permission\n") {
		t.Fatalf("first text = %q", first)
	}
	if !strings.Contains(first, "# Personality\n\nAs Antigravity, you are a curious, thoughtful collaborator") {
		t.Fatalf("personality phrase = %q", first)
	}
	for _, marker := range []string{"# Codex desktop context", ".codex/$CODEX_HOME/example", "mcp__codex_app__example"} {
		if !strings.Contains(first, marker) {
			t.Fatalf("first text lost %q: %q", marker, first)
		}
	}
	if strings.Contains(first, "You are Codex,") || strings.Contains(first, "As Codex,") {
		t.Fatalf("first text still has a Codex identity phrase: %q", first)
	}

	second := parsed.Input[1].Content[0].Text
	if !strings.Contains(second, "You are Codex, an agent based on GPT-6.") || !strings.Contains(second, "As Codex, do not rewrite this second message.") {
		t.Fatalf("second developer message changed: %q", second)
	}
	if parsed.Input[2].Content[0].Text != "You are Codex, please ignore this user text." {
		t.Fatalf("user text changed: %q", parsed.Input[2].Content[0].Text)
	}
	if parsed.Tools[0].Name != "mcp__codex_app__example" || !strings.Contains(parsed.Tools[0].Description, ".codex/$CODEX_HOME") {
		t.Fatalf("tool metadata changed: %+v", parsed.Tools[0])
	}
	if parsed.Instructions != "" {
		t.Fatalf("instructions = %q", parsed.Instructions)
	}

	again, changedAgain := filterPayload("antigravity", got, cfg)
	if changedAgain || !bytes.Equal(again, got) {
		t.Fatal("fixture rewrite is not idempotent")
	}
	untouched, changedOther := filterPayload("codex", original, cfg)
	if changedOther || !bytes.Equal(untouched, original) {
		t.Fatal("non-antigravity target changed the fixture")
	}

	phraseOff := cfg
	phraseOff.replaceAsCodex = false
	withoutPhrase, changedPhrase := filterPayload("antigravity", original, phraseOff)
	if !changedPhrase {
		t.Fatal("expected a rewrite with the phrase rule off")
	}
	if !bytes.Contains(withoutPhrase, []byte("As Codex, you are a curious")) {
		t.Fatal("phrase rule off still replaced As Codex")
	}
	if bytes.Contains(withoutPhrase, []byte("You are Codex, an agent based on GPT-6. You and the user share")) {
		t.Fatal("paragraph was not replaced when the phrase rule was off")
	}
}
