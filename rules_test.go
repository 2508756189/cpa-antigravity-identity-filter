package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestIdentityBlocksMatchExtractedText(t *testing.T) {
	const coding = "You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding.\nYou are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question."
	const generic = "You are Antigravity, a powerful agentic AI assistant designed by the Google Deepmind team.\nYou are working with a USER to solve their task."
	if identityCodingBlock != coding {
		t.Fatalf("coding block = %q", identityCodingBlock)
	}
	if identityGenericBlock != generic {
		t.Fatalf("generic block = %q", identityGenericBlock)
	}
	if strings.HasPrefix(identityCodingBlock, identityGenericBlock) || strings.HasPrefix(identityGenericBlock, identityCodingBlock) {
		t.Fatal("identity blocks must not be prefixes of each other")
	}
}

func TestBuiltinIdentityRules(t *testing.T) {
	cfg := defaultConfig()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "codex gpt-6 paragraph",
			in:   "You are Codex, an agent based on GPT-6. You and the user share one workspace, and your job is to collaborate with them until their intended goal is completely handled.\n\n# When to ask the user for permission\n\nKeep this.",
			want: identityCodingBlock + "\n\n# When to ask the user for permission\n\nKeep this.",
		},
		{
			name: "codex gpt-5 paragraph",
			in:   "You are Codex, a coding agent based on GPT-5.\n\nrest",
			want: identityCodingBlock + "\n\nrest",
		},
		{
			name: "openai codex",
			in:   "You are OpenAI Codex, a coding agent based on GPT-5.\n\nrest",
			want: identityCodingBlock + "\n\nrest",
		},
		{
			name: "codex personality phrase",
			in:   "You are Codex, a coding agent based on GPT-5.\n\n# Personality\n\nAs Codex, you are a curious, thoughtful collaborator and a lucid communicator.\n",
			want: identityCodingBlock + "\n\n# Personality\n\nAs Antigravity, you are a curious, thoughtful collaborator and a lucid communicator.\n",
		},
		{
			name: "later you are codex stays",
			in:   "You are Codex, a coding agent based on GPT-5.\n\n# Codex desktop context\n\nYou are Codex, mentioned again.\n.codex/$CODEX_HOME\nmcp__codex_app\n",
			want: identityCodingBlock + "\n\n# Codex desktop context\n\nYou are Codex, mentioned again.\n.codex/$CODEX_HOME\nmcp__codex_app\n",
		},
		{
			name: "claude code with following sentence",
			in:   "You are Claude Code, Anthropic's official CLI for Claude.\nYou are an interactive CLI tool that helps users with software engineering tasks.\n\n# Tools",
			want: identityCodingBlock + "\n\n# Tools",
		},
		{
			name: "claude code exact line",
			in:   "You are Claude Code, Anthropic's official CLI for Claude.",
			want: identityCodingBlock,
		},
		{
			name: "claude code inside agent sdk",
			in:   "You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK.\n\n# Tools",
			want: identityCodingBlock + "\n\n# Tools",
		},
		{
			name: "claude agent sdk",
			in:   "You are a Claude agent, built on Anthropic's Claude Agent SDK.\n\n# later",
			want: identityCodingBlock + "\n\n# later",
		},
		{
			name: "zcode coding",
			in:   "You are ZCode, an interactive coding agent.\n\n# Harness\nDo not touch safety rules.",
			want: identityCodingBlock + "\n\n# Harness\nDo not touch safety rules.",
		},
		{
			name: "zcode short",
			in:   "You are ZCode, an interactive agent.\n\n# Harness\nstay",
			want: identityCodingBlock + "\n\n# Harness\nstay",
		},
		{
			name: "hermes created",
			in:   "You are Hermes Agent, an intelligent AI assistant created by Nous Research.\n\n# later",
			want: identityCodingBlock + "\n\n# later",
		},
		{
			name: "hermes built",
			in:   "You are Hermes Agent, built by Nous Research.",
			want: identityCodingBlock,
		},
		{
			name: "opencode with following line",
			in:   "You are OpenCode, the best coding agent on the planet.\nYou are an interactive CLI tool that helps users.\n\n# later",
			want: identityCodingBlock + "\n\n# later",
		},
		{
			name: "opencode lowercase",
			in:   "You are opencode, the best coding agent on the planet.",
			want: identityCodingBlock,
		},
		{
			name: "openclaw",
			in:   "You are OpenClaw, a personal assistant.\n\n# later",
			want: identityCodingBlock + "\n\n# later",
		},
		{
			name: "leading whitespace",
			in:   "\n\nYou are Codex, a coding agent based on GPT-5.\n\nkeep",
			want: "\n\n" + identityCodingBlock + "\n\nkeep",
		},
		{
			name: "crlf blank line",
			in:   "You are Codex, a coding agent based on GPT-5.\r\n\r\nkeep",
			want: identityCodingBlock + "\r\n\r\nkeep",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := rewriteSystemText(t, tt.in, cfg)
			if !changed {
				t.Fatal("expected a rewrite")
			}
			if got != tt.want {
				t.Fatalf("got %q\nwant %q", got, tt.want)
			}
			again, changedAgain := rewriteSystemText(t, got, cfg)
			if changedAgain || again != got {
				t.Fatalf("not idempotent: changed=%v got %q", changedAgain, again)
			}
		})
	}
}

func TestNegativeIdentityText(t *testing.T) {
	cfg := defaultConfig()
	samples := []string{
		"you are codexify, an agent based on GPT-6.",
		"You are Codexify, an agent based on GPT-6.",
		"You are opencodeify, the best.",
		"You are Hermes, built by Nous Research.",
		"You are Codex desktop context",
		"Hello\n\nYou are Codex, a coding agent based on GPT-5.",
		"Temperature is K and the city is İstanbul.",
	}
	for _, sample := range samples {
		t.Run(sample, func(t *testing.T) {
			got, changed := rewriteSystemText(t, sample, cfg)
			if changed || got != sample {
				t.Fatalf("changed=%v got %q", changed, got)
			}
		})
	}
}

func TestAsCodexPhraseWithoutIdentityParagraph(t *testing.T) {
	cfg := defaultConfig()
	in := "As Codex, you are curious, but this is not an identity paragraph."
	got, changed := rewriteSystemText(t, in, cfg)
	if !changed || got != "As Antigravity, you are curious, but this is not an identity paragraph." {
		t.Fatalf("changed=%v got %q", changed, got)
	}
	again, changedAgain := rewriteSystemText(t, got, cfg)
	if changedAgain || again != got {
		t.Fatalf("phrase rule was not idempotent: %q", again)
	}
}

func TestAsCodexPhraseOnlyWhenEnabled(t *testing.T) {
	text := "You are Codex, a coding agent based on GPT-5.\n\n# Personality\n\nAs Codex, you are steady. As Codex, second stays."
	cfg := defaultConfig()
	cfg.replaceAsCodex = false
	got, changed := rewriteSystemText(t, text, cfg)
	if !changed {
		t.Fatal("paragraph should still be replaced")
	}
	if !strings.Contains(got, "As Codex, you are steady. As Codex, second stays.") {
		t.Fatalf("phrase rewritten while disabled: %q", got)
	}
	if !strings.HasPrefix(strings.TrimLeft(got, "\n"), identityCodingBlock) {
		t.Fatalf("identity missing: %q", got)
	}
}

func TestAlreadyOfficialIdentityIsLeftAlone(t *testing.T) {
	cfg := defaultConfig()
	plain := identityCodingBlock + "\n\n# Personality\n\nStay."
	got, changed := rewriteSystemText(t, plain, cfg)
	if changed || got != plain {
		t.Fatalf("official coding block changed: %q", got)
	}
	generic := identityGenericBlock + "\n\nStay."
	got, changed = rewriteSystemText(t, generic, cfg)
	if changed || got != generic {
		t.Fatalf("official generic block changed: %q", got)
	}

	withPhrase := identityCodingBlock + "\n\n# Personality\n\nAs Codex, you are steady."
	got, changed = rewriteSystemText(t, withPhrase, cfg)
	if !changed {
		t.Fatal("phrase should still be rewritten")
	}
	want := identityCodingBlock + "\n\n# Personality\n\nAs Antigravity, you are steady."
	if got != want {
		t.Fatalf("got %q", got)
	}
	again, changedAgain := rewriteSystemText(t, got, cfg)
	if changedAgain || again != got {
		t.Fatalf("second pass changed official text: %q", again)
	}
}

func TestGenericIdentityVariant(t *testing.T) {
	cfg := defaultConfig()
	cfg.identity = identityGeneric
	got, changed := rewriteSystemText(t, "You are Codex, a coding agent based on GPT-5.\n\nkeep", cfg)
	if !changed {
		t.Fatal("expected rewrite")
	}
	want := identityGenericBlock + "\n\nkeep"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestClientTogglesAndCustomRules(t *testing.T) {
	cfg := defaultConfig()
	cfg.clients.codex = false
	got, changed := rewriteSystemText(t, "You are Codex, a coding agent based on GPT-5.\n\nkeep", cfg)
	if changed || got != "You are Codex, a coding agent based on GPT-5.\n\nkeep" {
		t.Fatalf("disabled codex still changed: %q", got)
	}
	got, changed = rewriteSystemText(t, "You are Claude Code, Anthropic's official CLI for Claude.", cfg)
	if !changed || got != identityCodingBlock {
		t.Fatalf("claude toggle leaked: changed=%v got %q", changed, got)
	}

	cfg = defaultConfig()
	cfg.clients.codex = false
	cfg.custom = []customRule{{
		re:          regexp.MustCompile(`^You are Codex,`),
		paragraph:   true,
		useIdentity: true,
	}}
	got, changed = rewriteSystemText(t, "You are Codex, a coding agent based on GPT-5.\n\nkeep", cfg)
	if !changed || got != identityCodingBlock+"\n\nkeep" {
		t.Fatalf("custom rule ignored while built-in disabled: %q", got)
	}

	cfg = defaultConfig()
	cfg.custom = []customRule{{
		re:          regexp.MustCompile(`^You are Codex,`),
		paragraph:   true,
		replacement: "CUSTOM",
		useIdentity: false,
	}}
	got, changed = rewriteSystemText(t, "You are Codex, a coding agent based on GPT-5.\n\nkeep", cfg)
	if !changed || !strings.HasPrefix(got, identityCodingBlock) {
		t.Fatalf("custom rule overrode an enabled built-in: %q", got)
	}

	cfg = defaultConfig()
	cfg.custom = []customRule{{
		re:          regexp.MustCompile(`^You are Foo,`),
		replacement: "You are Bar,",
		useIdentity: false,
	}}
	got, changed = rewriteSystemText(t, "You are Foo, keep this sentence.", cfg)
	if !changed || got != "You are Bar, keep this sentence." {
		t.Fatalf("exact custom rule = %q", got)
	}
}

func TestParagraphEnd(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{in: "abc\n\nrest", want: 3},
		{in: "abc\r\n\r\nrest", want: 3},
		{in: "abc\n \nrest", want: 3},
		{in: "abc\nrest", want: len("abc\nrest")},
		{in: "abc", want: 3},
		{in: "", want: 0},
	}
	for _, tt := range tests {
		if got := paragraphEnd(tt.in); got != tt.want {
			t.Fatalf("paragraphEnd(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func rewriteSystemText(t *testing.T, text string, cfg config) (string, bool) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"system": text})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, changed := rewriteBody(body, cfg)
	if !changed {
		return text, false
	}
	var out struct {
		System string `json:"system"`
	}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal rewritten: %v\n%s", err, got)
	}
	return out.System, true
}
