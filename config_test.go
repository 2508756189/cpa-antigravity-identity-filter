package main

import (
	"strings"
	"testing"
)

func TestParseConfigDefaultsAndHostKeys(t *testing.T) {
	cfg, err := parseConfigYAML([]byte("enabled: true\npriority: 1\nstore:\n  version: 1.0.0\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.identity != identityCoding || !cfg.replaceAsCodex || !cfg.clients.codex || !cfg.clients.openclaw {
		t.Fatalf("defaults changed by host keys: %+v", cfg)
	}
	if !cfg.scopes.initial || cfg.scopes.subsequentSystem || cfg.scopes.toolDescriptions {
		t.Fatalf("scope defaults = %+v", cfg.scopes)
	}
	if cfg.neutralize.desktopContext || cfg.neutralize.poweredByModel || cfg.diagnostics {
		t.Fatalf("optional rules defaulted on: %+v diagnostics=%v", cfg.neutralize, cfg.diagnostics)
	}
	if len(cfg.custom) != 0 {
		t.Fatalf("custom rules = %#v", cfg.custom)
	}
}

func TestParseConfigClientsAndIdentity(t *testing.T) {
	cfg, err := parseConfigYAML([]byte(`
identity: Generic
replace_as_codex: false
clients:
  codex: false
  claude_code: true
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.identity != identityGeneric || cfg.replaceAsCodex || cfg.clients.codex || !cfg.clients.claudeCode || !cfg.clients.hermes {
		t.Fatalf("parsed = %+v", cfg)
	}

	if _, err := parseConfigYAML([]byte("identity: other\n")); err == nil {
		t.Fatal("invalid identity was accepted")
	}
	if _, err := parseConfigYAML([]byte("clients:\n  codxe: false\n")); err == nil || !strings.Contains(err.Error(), "unknown client") {
		t.Fatalf("unknown client error = %v", err)
	}
}

func TestParseCustomRules(t *testing.T) {
	cfg, err := parseConfigYAML([]byte(`
custom_rules:
  - pattern: '^You are Foo,'
    paragraph: true
  - pattern: '^You are Bar'
    replacement: 'You are Baz'
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.custom) != 2 || !cfg.custom[0].useIdentity || !cfg.custom[0].paragraph || cfg.custom[1].useIdentity || cfg.custom[1].replacement != "You are Baz" {
		t.Fatalf("rules = %#v", cfg.custom)
	}
	if _, err := parseConfigYAML([]byte("custom_rules:\n  - pattern: 'a*'\n")); err == nil {
		t.Fatal("empty-matching pattern was accepted")
	}
	if _, err := parseConfigYAML([]byte("custom_rules:\n  - pattern: '('\n")); err == nil {
		t.Fatal("invalid pattern was accepted")
	}
}

func TestParseScopesNeutralizeAndDiagnostics(t *testing.T) {
	cfg, err := parseConfigYAML([]byte(`
diagnostics: true
scopes:
  initial: false
  subsequent_system: true
  tool_descriptions: true
neutralize:
  desktop_context: true
  powered_by_model: true
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.scopes.initial || !cfg.scopes.subsequentSystem || !cfg.scopes.toolDescriptions {
		t.Fatalf("scopes = %+v", cfg.scopes)
	}
	if !cfg.neutralize.desktopContext || !cfg.neutralize.poweredByModel || !cfg.diagnostics {
		t.Fatalf("neutralize = %+v diagnostics=%v", cfg.neutralize, cfg.diagnostics)
	}

	if _, err := parseConfigYAML([]byte("scopes:\n  reminder_tags: true\n")); err == nil || !strings.Contains(err.Error(), "unknown scope") {
		t.Fatalf("unknown scope error = %v", err)
	}
	if _, err := parseConfigYAML([]byte("neutralize:\n  codex: true\n")); err == nil || !strings.Contains(err.Error(), "unknown neutralize") {
		t.Fatalf("unknown neutralize error = %v", err)
	}
	if _, err := parseConfigYAML([]byte("diagnostics: yes\n")); err == nil {
		t.Fatal("non-boolean diagnostics was accepted")
	}
	if _, err := parseConfigYAML([]byte("scopes: []\n")); err == nil {
		t.Fatal("scope list was accepted")
	}
}
