package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestScopeOptInRewritesOnlyTheSelectedPosition(t *testing.T) {
	chat := []byte(`{"model":"gpt-test","messages":[{"role":"system","content":"You are Codex, a coding agent based on GPT-5. Keep guidance."},{"role":"user","content":"<system-reminder>\nYou are Codex, a coding agent based on GPT-5.\n</system-reminder>"},{"role":"assistant","content":"You are Codex, a coding agent based on GPT-5."},{"role":"system","content":"You are ZCode, an interactive agent. Late guidance."}],"tools":[{"type":"function","function":{"name":"mcp__codex_app__example","description":"You are OpenClaw, a personal assistant. Desc guidance. As Codex, stay.","parameters":{"type":"object","properties":{"path":{"type":"string","description":"You are Codex, a coding agent based on GPT-5."}}}}}]}`)
	responses := []byte(`{"model":"gpt-test","instructions":"You are Codex, a coding agent based on GPT-5.\r\nLater guidance.","input":[{"role":"developer","content":"You are Claude Code, Anthropic's official CLI for Claude. Second."},{"role":"user","content":"code sample: You are Codex, a coding agent based on GPT-5."}],"tools":[{"type":"function","name":"mcp__codex_app__example","description":"You are Hermes Agent, built by Nous Research. Tool guidance.","parameters":{"type":"object","properties":{"path":{"description":"You are Codex, a coding agent based on GPT-5."}}}}]}`)
	gemini := []byte(`{"systemInstruction":{"parts":[{"text":"You are OpenCode, the best coding agent on the planet. Stay."},{"text":"You are Codex, a coding agent based on GPT-5."}]},"tools":[{"functionDeclarations":[{"name":"codex_tool","description":"You are OpenClaw, a personal assistant. Keep.","parameters":{"type":"OBJECT","properties":{"path":{"type":"STRING","description":"You are ZCode, an interactive agent."}}}}]}]}`)

	t.Run("defaults", func(t *testing.T) {
		cfg := defaultConfig()
		assertChat(t, chat, cfg, chatExpect{
			first:     identityCodingBlock + " Keep guidance.",
			user:      "<system-reminder>\nYou are Codex, a coding agent based on GPT-5.\n</system-reminder>",
			assistant: "You are Codex, a coding agent based on GPT-5.",
			late:      "You are ZCode, an interactive agent. Late guidance.",
			toolName:  "mcp__codex_app__example",
			toolDesc:  "You are OpenClaw, a personal assistant. Desc guidance. As Codex, stay.",
			paramDesc: "You are Codex, a coding agent based on GPT-5.",
		})
		assertResponses(t, responses, cfg, responsesExpect{
			instructions: identityCodingBlock + "\r\nLater guidance.",
			developer:    "You are Claude Code, Anthropic's official CLI for Claude. Second.",
			user:         "code sample: You are Codex, a coding agent based on GPT-5.",
			toolName:     "mcp__codex_app__example",
			toolDesc:     "You are Hermes Agent, built by Nous Research. Tool guidance.",
			paramDesc:    "You are Codex, a coding agent based on GPT-5.",
		})
		got, changed := rewriteBody(gemini, cfg)
		if !changed {
			t.Fatal("expected gemini rewrite")
		}
		var parsed struct {
			SystemInstruction struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"systemInstruction"`
			Tools []struct {
				Decls []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"functionDeclarations"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(got, &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed.SystemInstruction.Parts[0].Text != identityCodingBlock+" Stay." {
			t.Fatalf("gemini first = %q", parsed.SystemInstruction.Parts[0].Text)
		}
		if parsed.SystemInstruction.Parts[1].Text != "You are Codex, a coding agent based on GPT-5." {
			t.Fatalf("sibling system part changed: %q", parsed.SystemInstruction.Parts[1].Text)
		}
		if parsed.Tools[0].Decls[0].Name != "codex_tool" || parsed.Tools[0].Decls[0].Description != "You are OpenClaw, a personal assistant. Keep." {
			t.Fatalf("gemini tool changed: %+v", parsed.Tools[0].Decls[0])
		}
	})

	t.Run("subsequent system", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.scopes.subsequentSystem = true
		assertChat(t, chat, cfg, chatExpect{
			first:     identityCodingBlock + " Keep guidance.",
			user:      "<system-reminder>\nYou are Codex, a coding agent based on GPT-5.\n</system-reminder>",
			assistant: "You are Codex, a coding agent based on GPT-5.",
			late:      identityCodingBlock + " Late guidance.",
			toolName:  "mcp__codex_app__example",
			toolDesc:  "You are OpenClaw, a personal assistant. Desc guidance. As Codex, stay.",
			paramDesc: "You are Codex, a coding agent based on GPT-5.",
		})
		assertResponses(t, responses, cfg, responsesExpect{
			instructions: identityCodingBlock + "\r\nLater guidance.",
			developer:    identityCodingBlock + " Second.",
			user:         "code sample: You are Codex, a coding agent based on GPT-5.",
			toolName:     "mcp__codex_app__example",
			toolDesc:     "You are Hermes Agent, built by Nous Research. Tool guidance.",
			paramDesc:    "You are Codex, a coding agent based on GPT-5.",
		})
		sibling := []byte(`{"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."},{"type":"text","text":"You are Codex, a coding agent based on GPT-5."}]}`)
		got, changed := rewriteBody(sibling, cfg)
		if !changed {
			t.Fatal("expected initial rewrite")
		}
		if !bytes.Contains(got, []byte("You are Codex, a coding agent based on GPT-5.")) {
			t.Fatalf("sibling system block was rewritten: %s", got)
		}
	})

	t.Run("tool descriptions", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.scopes.initial = false
		cfg.scopes.toolDescriptions = true
		assertChat(t, chat, cfg, chatExpect{
			first:     "You are Codex, a coding agent based on GPT-5. Keep guidance.",
			user:      "<system-reminder>\nYou are Codex, a coding agent based on GPT-5.\n</system-reminder>",
			assistant: "You are Codex, a coding agent based on GPT-5.",
			late:      "You are ZCode, an interactive agent. Late guidance.",
			toolName:  "mcp__codex_app__example",
			toolDesc:  identityCodingBlock + " Desc guidance. As Codex, stay.",
			paramDesc: "You are Codex, a coding agent based on GPT-5.",
		})
		assertResponses(t, responses, cfg, responsesExpect{
			instructions: "You are Codex, a coding agent based on GPT-5.\r\nLater guidance.",
			developer:    "You are Claude Code, Anthropic's official CLI for Claude. Second.",
			user:         "code sample: You are Codex, a coding agent based on GPT-5.",
			toolName:     "mcp__codex_app__example",
			toolDesc:     identityCodingBlock + " Tool guidance.",
			paramDesc:    "You are Codex, a coding agent based on GPT-5.",
		})
		got, changed := rewriteBody(gemini, cfg)
		if !changed {
			t.Fatal("expected gemini tool rewrite")
		}
		if !bytes.Contains(got, []byte(`"name":"codex_tool"`)) {
			t.Fatalf("tool name changed: %s", got)
		}
		if !bytes.Contains(got, []byte("You are ZCode, an interactive agent.")) {
			t.Fatalf("parameter description changed: %s", got)
		}
		var parsed struct {
			Tools []struct {
				Decls []struct {
					Description string `json:"description"`
				} `json:"functionDeclarations"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(got, &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed.Tools[0].Decls[0].Description != identityCodingBlock+" Keep." {
			t.Fatalf("declaration description = %q", parsed.Tools[0].Decls[0].Description)
		}
	})
}

func TestOptionalNeutralizeRulesStayInsideTrustedText(t *testing.T) {
	system := "You are Codex, a coding agent based on GPT-5.3.\n\n# Codex desktop context\nWorkspace: .codex/$CODEX_HOME/example\n# Codex desktop context extra\nYou are powered by the model named gpt-5.3-codex.\nPath: .zcode/keep\n"
	user := "# Codex desktop context\nYou are powered by the model named gpt-5.3-codex.\n.codex/keep"
	body := []byte(`{"messages":[{"role":"system","content":` + jsonString(system) + `},{"role":"user","content":` + jsonString(user) + `}]}`)

	t.Run("default leaves phrases", func(t *testing.T) {
		got, changed := rewriteBody(body, defaultConfig())
		if !changed {
			t.Fatal("expected identity rewrite")
		}
		decoded := decodeMessages(t, got)
		if !strings.Contains(decoded[0], "# Codex desktop context\n") || !strings.Contains(decoded[0], "You are powered by the model named gpt-5.3-codex.") {
			t.Fatalf("default neutralized optional phrases: %q", decoded[0])
		}
		if !strings.Contains(decoded[0], ".codex/$CODEX_HOME/example") || !strings.Contains(decoded[0], ".zcode/keep") {
			t.Fatalf("path changed: %q", decoded[0])
		}
		if decoded[1] != user {
			t.Fatalf("user text changed: %q", decoded[1])
		}
		if !strings.HasPrefix(decoded[0], identityCodingBlock+"\n\n# Codex desktop context\n") {
			t.Fatalf("versioned sentence was not bounded: %q", decoded[0])
		}
		if strings.Contains(decoded[0], "GPT-5.3.") {
			t.Fatalf("model version was left in the identity sentence: %q", decoded[0])
		}
	})

	t.Run("opt in", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.neutralize.desktopContext = true
		cfg.neutralize.poweredByModel = true
		got, changed := rewriteBody(body, cfg)
		if !changed {
			t.Fatal("expected rewrite")
		}
		decoded := decodeMessages(t, got)
		if strings.Contains(decoded[0], "# Codex desktop context\n") || strings.Contains(decoded[0], "gpt-5.3-codex") {
			t.Fatalf("optional phrases remained: %q", decoded[0])
		}
		if !strings.Contains(decoded[0], "# Desktop context\n") || !strings.Contains(decoded[0], "You are powered by the model named Antigravity.") {
			t.Fatalf("neutral form missing: %q", decoded[0])
		}
		if !strings.Contains(decoded[0], "# Codex desktop context extra") || !strings.Contains(decoded[0], ".codex/$CODEX_HOME/example") || !strings.Contains(decoded[0], ".zcode/keep") {
			t.Fatalf("unrelated line or path changed: %q", decoded[0])
		}
		if decoded[1] != user {
			t.Fatalf("user text changed: %q", decoded[1])
		}
		again, changedAgain := rewriteBody(got, cfg)
		if changedAgain || !bytes.Equal(again, got) {
			t.Fatal("neutralize rules were not idempotent")
		}
	})

	t.Run("unbounded powered-by stays", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.neutralize.poweredByModel = true
		text := "You are powered by the model named gpt-5.3-codex\nkeep"
		got, changed := rewriteSystemText(t, text, cfg)
		if changed || got != text {
			t.Fatalf("changed=%v got %q", changed, got)
		}
	})
}

func TestNoDuplicateIdentityBlock(t *testing.T) {
	cfg := defaultConfig()
	in := "You are Codex, a coding agent based on GPT-5.\n\n" + identityCodingBlock
	got, changed := rewriteSystemText(t, in, cfg)
	want := "\n\n" + identityCodingBlock
	if !changed || got != want {
		t.Fatalf("changed=%v got %q", changed, got)
	}
	if strings.Count(got, identityCodingBlock) != 1 {
		t.Fatalf("block count = %d", strings.Count(got, identityCodingBlock))
	}
	again, changedAgain := rewriteSystemText(t, got, cfg)
	if changedAgain || again != got {
		t.Fatalf("second pass changed: %q", again)
	}
}

func TestAsCodexInsideIdentitySentenceIsNotPartial(t *testing.T) {
	cfg := defaultConfig()
	got, changed := rewriteSystemText(t, "You are Codex, As Codex, a coding agent based on GPT-5. After.", cfg)
	if !changed || got != identityCodingBlock+" After." {
		t.Fatalf("changed=%v got %q", changed, got)
	}
	if strings.Contains(got, "As Codex,") {
		t.Fatalf("phrase survived inside the replaced sentence: %q", got)
	}
}

func TestDisabledScopesAreExactPassthrough(t *testing.T) {
	cfg := defaultConfig()
	cfg.scopes = scopeSet{}
	body := []byte(`{"system":"You are Codex, a coding agent based on GPT-5. Keep.","tools":[{"type":"function","function":{"name":"codex","description":"You are Codex, a coding agent based on GPT-5."}}]}`)
	got, changed := filterPayload("antigravity", body, cfg)
	if changed || !bytes.Equal(got, body) || !sameSlice(got, body) {
		t.Fatal("disabled scopes changed the payload")
	}
	got, changed = filterPayload("openai", body, defaultConfig())
	if changed || !bytes.Equal(got, body) || !sameSlice(got, body) {
		t.Fatal("non-antigravity target changed the payload")
	}
}

func TestToolScopeDoesNotBlockAStringToolsField(t *testing.T) {
	cfg := defaultConfig()
	cfg.scopes.toolDescriptions = true
	body := []byte(`{"system":"You are Codex, a coding agent based on GPT-5.","tools":"codex"}`)
	got, changed := rewriteBody(body, cfg)
	if !changed || !bytes.Contains(got, []byte(`"tools":"codex"`)) {
		t.Fatalf("changed=%v body %s", changed, got)
	}
}

func TestMalformedToolsLeaveTheBodyUntouched(t *testing.T) {
	cfg := defaultConfig()
	cfg.scopes.toolDescriptions = true
	body := []byte(`{"system":"You are Codex, a coding agent based on GPT-5.","tools":[`)
	original := append([]byte(nil), body...)
	got, changed := rewriteBody(body, cfg)
	if changed || !bytes.Equal(got, original) || !bytes.Equal(body, original) {
		t.Fatalf("malformed body changed: %s", got)
	}
}

func TestDiagnosticsRecordOnlyRuleScopeAndCount(t *testing.T) {
	const marker = "SECRET_PROMPT_MARKER"
	text := "You are Codex, a coding agent based on GPT-5.\n" + marker + "\nPath: .zcode/private .codex/$CODEX_HOME C:/Users/someone sk-test-credential user@example.com Authorization: Bearer token\n<system-reminder>\n"
	body := []byte(`{"system":` + jsonString(text) + `}`)

	var records []diagRecord
	setDiagHook(func(rec diagRecord) { records = append(records, rec) })
	t.Cleanup(func() { setDiagHook(nil) })

	cfg := defaultConfig()
	got, changed := filterPayload("antigravity", body, cfg)
	if !changed {
		t.Fatal("expected rewrite")
	}
	if len(records) != 0 {
		t.Fatalf("diagnostics default emitted %#v", records)
	}
	if !bytes.Contains(got, []byte(marker)) {
		t.Fatal("guidance marker was removed from the body")
	}

	records = nil
	cfg.diagnostics = true
	got, changed = filterPayload("codex", body, cfg)
	if changed || len(records) != 0 {
		t.Fatalf("non-antigravity diagnostics changed=%v records=%#v", changed, records)
	}

	got, changed = filterPayload("antigravity", body, cfg)
	if !changed {
		t.Fatal("expected rewrite")
	}
	if len(records) != 1 || records[0] != (diagRecord{RuleID: "codex", Scope: "initial", Count: 1}) {
		t.Fatalf("records = %#v", records)
	}
	assertDiagPayloadSafe(t, records, []string{
		marker,
		"You are Codex, a coding agent based on GPT-5.",
		"You are Antigravity",
		".zcode",
		".codex",
		"C:/Users",
		"sk-test-credential",
		"user@example.com",
		"Authorization",
		"Bearer",
		"<system-reminder>",
		text,
	})

	records = nil
	cfg.neutralize.poweredByModel = true
	cfg.replaceAsCodex = true
	phraseBody := []byte(`{"system":"You are Codex, a coding agent based on GPT-5.\n\n# Codex desktop context\nYou are powered by the model named gpt-test.\nAs Codex, stay. ` + marker + `"}`)
	cfg.neutralize.desktopContext = true
	_, changed = filterPayload("antigravity", phraseBody, cfg)
	if !changed {
		t.Fatal("expected combined rewrite")
	}
	seen := map[string]int{}
	for _, rec := range records {
		seen[rec.RuleID+"/"+rec.Scope] = rec.Count
	}
	for _, id := range []string{"codex/initial", "desktop-context-header/initial", "powered-by-model/initial", "as-codex/initial"} {
		if seen[id] != 1 {
			t.Fatalf("missing %s in %#v", id, records)
		}
	}
	assertDiagPayloadSafe(t, records, []string{marker, "You are Codex", "gpt-test", "# Codex desktop context", "As Codex", "As Antigravity", ".codex"})
}

func TestDiagnosticsCountToolDescriptionRepeats(t *testing.T) {
	body := []byte(`{"tools":[{"type":"function","function":{"name":"one","description":"You are Codex, a coding agent based on GPT-5. A.","parameters":{"type":"object"}}},{"type":"function","function":{"name":"two","description":"You are Codex, a coding agent based on GPT-5. B.","parameters":{"type":"object"}}}]}`)
	cfg := defaultConfig()
	cfg.scopes.initial = false
	cfg.scopes.toolDescriptions = true
	cfg.diagnostics = true
	var records []diagRecord
	setDiagHook(func(rec diagRecord) { records = append(records, rec) })
	t.Cleanup(func() { setDiagHook(nil) })

	got, changed := rewriteBody(body, cfg)
	if !changed {
		t.Fatal("expected tool rewrites")
	}
	if !bytes.Contains(got, []byte(`"name":"one"`)) || !bytes.Contains(got, []byte(`"name":"two"`)) {
		t.Fatalf("tool names changed: %s", got)
	}
	if len(records) != 1 || records[0] != (diagRecord{RuleID: "codex", Scope: "tool_description", Count: 2}) {
		t.Fatalf("records = %#v", records)
	}
	again, changedAgain := rewriteBody(got, cfg)
	if changedAgain || !bytes.Equal(again, got) {
		t.Fatal("repeated tool rewrite was not stable")
	}
}

func TestDiagnosticPayloadRejectsUnsafeTokens(t *testing.T) {
	ok, err := diagnosticPayload(diagRecord{RuleID: "codex", Scope: "initial", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(ok, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 3 || decoded["message"] != "identity_filter" || decoded["level"] != "info" {
		t.Fatalf("payload = %s", ok)
	}
	fields, _ := decoded["fields"].(map[string]any)
	if len(fields) != 3 || fields["rule_id"] != "codex" || fields["scope"] != "initial" || fields["count"] != float64(1) {
		t.Fatalf("fields = %#v", fields)
	}
	for _, rec := range []diagRecord{
		{RuleID: "You are Codex", Scope: "initial", Count: 1},
		{RuleID: ".codex", Scope: "initial", Count: 1},
		{RuleID: "codex", Scope: "user", Count: 1},
		{RuleID: "codex", Scope: "initial", Count: 0},
		{RuleID: "", Scope: "initial", Count: 1},
	} {
		if _, err := diagnosticPayload(rec); err == nil {
			t.Fatalf("accepted %#v", rec)
		}
	}
}

func TestRewriteConcurrent(t *testing.T) {
	cfg := defaultConfig()
	body := []byte(`{"system":"You are Codex, a coding agent based on GPT-5.\nkeep"}`)
	var wg sync.WaitGroup
	errCh := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 40; n++ {
				got, changed := rewriteBody(append([]byte(nil), body...), cfg)
				if !changed {
					errCh <- "expected change"
					return
				}
				again, changedAgain := rewriteBody(got, cfg)
				if changedAgain || !bytes.Equal(again, got) {
					errCh <- "not idempotent"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	if errMsg := <-errCh; errMsg != "" {
		t.Fatal(errMsg)
	}
}

func FuzzBoundedIdentityIdempotent(f *testing.F) {
	f.Add("You are ZCode, an interactive agent. You are an agent working with the user toward their goals, using your own judgment along the way.")
	f.Add("You are Codex, a coding agent based on GPT-5.\r\n\r\nkeep")
	f.Add("You are Codex, a coding agent based on GPT-5.\nYou are an agent working with the user toward their goals, using your own judgment along the way.")
	f.Add("no identity here")
	f.Add("")
	f.Add("As Codex, only a phrase.")
	f.Add("You are Codex, see .codex/config.")
	f.Add("# Codex desktop context\n.codex/keep")
	f.Add("You are opencodeify, the best.")
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 2048 || !utf8.ValidString(text) {
			return
		}
		cfg := defaultConfig()
		got, _ := rewriteSystemText(t, text, cfg)
		if strings.Count(got, identityCodingBlock) > max(strings.Count(text, identityCodingBlock), 1) {
			t.Fatalf("duplicate identity block")
		}
		again, changed := rewriteSystemText(t, got, cfg)
		if changed || again != got {
			t.Fatalf("not idempotent: changed=%v", changed)
		}
		if !containsIdentityTrigger(text) && got != text {
			t.Fatalf("rewrote text without a supported trigger: %q -> %q", text, got)
		}
	})
}

func containsIdentityTrigger(text string) bool {
	triggers := []string{
		"You are OpenAI Codex,",
		"You are Codex,",
		"You are Claude Code,",
		"You are a Claude agent,",
		"You are ZCode,",
		"You are Hermes Agent,",
		"You are OpenCode,",
		"You are opencode",
		"You are OpenClaw,",
		"As Codex,",
	}
	for _, trigger := range triggers {
		if strings.Contains(text, trigger) {
			return true
		}
	}
	return false
}

type chatExpect struct {
	first     string
	user      string
	assistant string
	late      string
	toolName  string
	toolDesc  string
	paramDesc string
}

func assertChat(t *testing.T, body []byte, cfg config, want chatExpect) {
	t.Helper()
	got, changed := rewriteBody(append([]byte(nil), body...), cfg)
	if !changed && (want.first != "You are Codex, a coding agent based on GPT-5. Keep guidance." || want.toolDesc != "You are OpenClaw, a personal assistant. Desc guidance. As Codex, stay.") {
		t.Fatal("expected a rewrite")
	}
	if !changed {
		got = body
	}
	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Function struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Parameters  struct {
					Properties struct {
						Path struct {
							Description string `json:"description"`
						} `json:"path"`
					} `json:"properties"`
				} `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, got)
	}
	if parsed.Messages[0].Content != want.first || parsed.Messages[1].Content != want.user || parsed.Messages[2].Content != want.assistant || parsed.Messages[3].Content != want.late {
		t.Fatalf("messages = %#v", parsed.Messages)
	}
	fn := parsed.Tools[0].Function
	if fn.Name != want.toolName || fn.Description != want.toolDesc || fn.Parameters.Properties.Path.Description != want.paramDesc {
		t.Fatalf("tool = %#v", fn)
	}
	again, changedAgain := rewriteBody(got, cfg)
	if changedAgain || !bytes.Equal(again, got) {
		t.Fatal("chat rewrite was not idempotent")
	}
}

type responsesExpect struct {
	instructions string
	developer    string
	user         string
	toolName     string
	toolDesc     string
	paramDesc    string
}

func assertResponses(t *testing.T, body []byte, cfg config, want responsesExpect) {
	t.Helper()
	got, changed := rewriteBody(append([]byte(nil), body...), cfg)
	if !changed && !strings.HasPrefix(want.instructions, "You are Codex,") {
		t.Fatal("expected a rewrite")
	}
	if !changed {
		got = body
	}
	var parsed struct {
		Instructions string `json:"instructions"`
		Input        []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"input"`
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Parameters  struct {
				Properties struct {
					Path struct {
						Description string `json:"description"`
					} `json:"path"`
				} `json:"properties"`
			} `json:"parameters"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, got)
	}
	if parsed.Instructions != want.instructions || parsed.Input[0].Content != want.developer || parsed.Input[1].Content != want.user {
		t.Fatalf("instructions=%q developer=%q user=%q", parsed.Instructions, parsed.Input[0].Content, parsed.Input[1].Content)
	}
	tool := parsed.Tools[0]
	if tool.Name != want.toolName || tool.Description != want.toolDesc || tool.Parameters.Properties.Path.Description != want.paramDesc {
		t.Fatalf("tool = %#v", tool)
	}
	again, changedAgain := rewriteBody(got, cfg)
	if changedAgain || !bytes.Equal(again, got) {
		t.Fatal("responses rewrite was not idempotent")
	}
}

func assertDiagPayloadSafe(t *testing.T, records []diagRecord, forbidden []string) {
	t.Helper()
	for _, rec := range records {
		raw, err := diagnosticPayload(rec)
		if err != nil {
			t.Fatalf("payload: %v", err)
		}
		for _, marker := range forbidden {
			if strings.Contains(string(raw), marker) || strings.Contains(rec.RuleID, marker) || strings.Contains(rec.Scope, marker) {
				t.Fatalf("diagnostic leaked %q in %s", marker, raw)
			}
		}
	}
}

func decodeMessages(t *testing.T, body []byte) []string {
	t.Helper()
	var parsed struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(parsed.Messages))
	for i, message := range parsed.Messages {
		out[i] = message.Content
	}
	return out
}

func jsonString(text string) string {
	raw, err := json.Marshal(text)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func sameSlice(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	return &a[0] == &b[0]
}
