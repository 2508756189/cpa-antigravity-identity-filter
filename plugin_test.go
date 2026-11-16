package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestRegisterDeclaresIdentityFilter(t *testing.T) {
	raw, code := handlePluginCall(pluginabi.MethodPluginRegister, nil)
	if code != 0 {
		t.Fatalf("code = %d, body = %s", code, raw)
	}
	var envelope struct {
		OK     bool `json:"ok"`
		Result struct {
			SchemaVersion uint32 `json:"schema_version"`
			Metadata      struct {
				Name             string `json:"Name"`
				Version          string `json:"Version"`
				Author           string `json:"Author"`
				GitHubRepository string `json:"GitHubRepository"`
				ConfigFields     []struct {
					Name string `json:"Name"`
					Type string `json:"Type"`
				} `json:"ConfigFields"`
			} `json:"metadata"`
			Capabilities map[string]any `json:"capabilities"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if !envelope.OK {
		t.Fatal("ok = false")
	}
	if envelope.Result.SchemaVersion != pluginabi.SchemaVersion {
		t.Fatalf("schema = %d, want %d", envelope.Result.SchemaVersion, pluginabi.SchemaVersion)
	}
	meta := envelope.Result.Metadata
	if meta.Name != pluginName || meta.Version != "1.0.0" || meta.Author != pluginAuthor || meta.GitHubRepository != pluginRepository {
		t.Fatalf("metadata = %+v", meta)
	}
	if envelope.Result.Capabilities["request_interceptor"] != true {
		t.Fatalf("capabilities = %#v", envelope.Result.Capabilities)
	}
	if _, ok := envelope.Result.Capabilities["model_router"]; ok {
		t.Fatal("model_router must not be declared")
	}
	if _, ok := envelope.Result.Capabilities["executor"]; ok {
		t.Fatal("executor must not be declared")
	}
	seen := map[string]bool{}
	for _, field := range meta.ConfigFields {
		seen[field.Name] = true
	}
	for _, name := range []string{"identity", "replace_as_codex", "clients", "custom_rules", "scopes", "neutralize", "diagnostics"} {
		if !seen[name] {
			t.Fatalf("missing config field %s in %#v", name, meta.ConfigFields)
		}
	}
}

func TestReconfigureKeepsPreviousConfigOnInvalidInput(t *testing.T) {
	t.Cleanup(func() { setConfig(defaultConfig()) })

	raw, code := handlePluginCall(pluginabi.MethodPluginReconfigure, lifecycleJSON(t, []byte("identity: generic\n")))
	if code != 0 || !bytes.Contains(raw, []byte(`"ok":true`)) {
		t.Fatalf("reconfigure = %d %s", code, raw)
	}
	raw, code = handlePluginCall(pluginabi.MethodPluginReconfigure, lifecycleJSON(t, []byte("clients:\n  nope: false\n")))
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.OK || envelope.Error.Code != "invalid_config" {
		t.Fatalf("envelope = %+v", envelope)
	}
	got, changed := rewriteSystemText(t, "You are Codex, a coding agent based on GPT-5.", activeConfig())
	if !changed || got != identityGenericBlock {
		t.Fatalf("previous generic identity was not retained: changed=%v got %q", changed, got)
	}
}

func TestInterceptBeforeNeverRewrites(t *testing.T) {
	body := []byte(`{"system":"You are Codex, a coding agent based on GPT-5."}`)
	raw, code := handlePluginCall(pluginabi.MethodRequestInterceptBefore, interceptJSON(t, "antigravity", body))
	if code != 0 {
		t.Fatalf("code = %d body %s", code, raw)
	}
	resp := decodeIntercept(t, raw)
	if len(resp.Body) != 0 {
		t.Fatalf("before-auth body = %s", resp.Body)
	}
}

func TestInterceptAfterRewritesOnlyAntigravity(t *testing.T) {
	t.Cleanup(func() { setConfig(defaultConfig()) })
	setConfig(defaultConfig())
	body := []byte(`{"system":"You are Codex, a coding agent based on GPT-5.\n\nkeep"}`)

	raw, code := handlePluginCall(pluginabi.MethodRequestInterceptAfter, interceptJSON(t, "codex", body))
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if len(decodeIntercept(t, raw).Body) != 0 {
		t.Fatalf("non-antigravity response = %s", raw)
	}

	raw, code = handlePluginCall(pluginabi.MethodRequestInterceptAfter, interceptJSON(t, "antigravity", body))
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	resp := decodeIntercept(t, raw)
	want, changed := rewriteBody(body, defaultConfig())
	if !changed || !bytes.Equal(resp.Body, want) {
		t.Fatalf("after body = %s\nwant %s", resp.Body, want)
	}
}

func TestCustomRuleStillAppliesWhenBuiltInIsDisabled(t *testing.T) {
	t.Cleanup(func() { setConfig(defaultConfig()) })
	raw, code := handlePluginCall(pluginabi.MethodPluginReconfigure, lifecycleJSON(t, []byte(`
clients:
  codex: false
custom_rules:
  - pattern: '^You are Codex,'
    paragraph: true
`)))
	if code != 0 || !bytes.Contains(raw, []byte(`"ok":true`)) {
		t.Fatalf("reconfigure = %d %s", code, raw)
	}
	got, changed := rewriteSystemText(t, "You are Codex, a coding agent based on GPT-5.\n\nkeep", activeConfig())
	if !changed || got != identityCodingBlock+"\n\nkeep" {
		t.Fatalf("custom rule did not run: changed=%v got %q", changed, got)
	}
}

func TestUnknownMethod(t *testing.T) {
	raw, code := handlePluginCall("executor.execute", nil)
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.OK || envelope.Error.Code != "unknown_method" {
		t.Fatalf("envelope = %+v body %s", envelope, raw)
	}
}

func lifecycleJSON(t *testing.T, yaml []byte) []byte {
	t.Helper()
	raw, err := json.Marshal(lifecycleRequest{ConfigYAML: yaml})
	if err != nil {
		t.Fatalf("marshal lifecycle: %v", err)
	}
	return raw
}

func interceptJSON(t *testing.T, toFormat string, body []byte) []byte {
	t.Helper()
	raw, err := json.Marshal(pluginapi.RequestInterceptRequest{
		SourceFormat: "openai-response",
		ToFormat:     toFormat,
		Body:         body,
	})
	if err != nil {
		t.Fatalf("marshal intercept: %v", err)
	}
	return raw
}

func decodeIntercept(t *testing.T, raw []byte) pluginapi.RequestInterceptResponse {
	t.Helper()
	var envelope struct {
		OK     bool                               `json:"ok"`
		Result pluginapi.RequestInterceptResponse `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal intercept: %v\n%s", err, raw)
	}
	if !envelope.OK {
		t.Fatalf("intercept failed: %s", raw)
	}
	return envelope.Result
}
