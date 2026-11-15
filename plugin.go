package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	pluginName       = "antigravity-identity-filter"
	pluginVersion    = "1.0.0"
	pluginAuthor     = "wangsh"
	pluginRepository = "https://github.com/2508756189/cpa-antigravity-identity-filter"
)

var (
	cfgMu      sync.RWMutex
	currentCfg = defaultConfig()
)

func setConfig(cfg config) {
	cfgMu.Lock()
	currentCfg = cfg.snapshot()
	cfgMu.Unlock()
}

func activeConfig() config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return currentCfg.snapshot()
}

func handlePluginCall(method string, request []byte) ([]byte, int) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return handleLifecycle(request), 0
	case pluginabi.MethodPluginQuiesce, pluginabi.MethodPluginShutdown:
		return mustEnvelope(struct{}{}), 0
	case pluginabi.MethodRequestInterceptBefore:
		return handleRequestInterceptBefore(request), 0
	case pluginabi.MethodRequestInterceptAfter:
		return handleRequestInterceptAfter(request), 0
	default:
		return mustErrorEnvelope("unknown_method", fmt.Sprintf("unknown method %q", method)), 0
	}
}

func handleLifecycle(request []byte) []byte {
	if len(request) == 0 {
		return mustEnvelope(registrationResponse())
	}
	cfg, err := configFromLifecycle(request)
	if err != nil {
		return mustErrorEnvelope("invalid_config", err.Error())
	}
	setConfig(cfg)
	return mustEnvelope(registrationResponse())
}

func registrationResponse() any {
	return struct {
		SchemaVersion uint32             `json:"schema_version"`
		Metadata      pluginapi.Metadata `json:"metadata"`
		Capabilities  struct {
			RequestInterceptor bool `json:"request_interceptor"`
		} `json:"capabilities"`
	}{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           pluginAuthor,
			GitHubRepository: pluginRepository,
			ConfigFields:     configFields(),
		},
		Capabilities: struct {
			RequestInterceptor bool `json:"request_interceptor"`
		}{RequestInterceptor: true},
	}
}

func configFields() []pluginapi.ConfigField {
	return []pluginapi.ConfigField{
		{
			Name:        "identity",
			Type:        pluginapi.ConfigFieldTypeEnum,
			EnumValues:  []string{identityCoding, identityGeneric},
			Description: "Official identity block to insert. coding is the default agentic-coding paragraph; generic is the shorter assistant paragraph.",
		},
		{
			Name:        "replace_as_codex",
			Type:        pluginapi.ConfigFieldTypeBoolean,
			Description: "Replace the first \"As Codex,\" in the first system/developer text with \"As Antigravity,\". Defaults to true.",
		},
		{
			Name:        "clients",
			Type:        pluginapi.ConfigFieldTypeObject,
			Description: "Per-client toggles: codex, claude_code, claude_agent_sdk, zcode, hermes, opencode, openclaw. Omitted keys stay enabled.",
		},
		{
			Name:        "custom_rules",
			Type:        pluginapi.ConfigFieldTypeArray,
			Description: "Extra anchored rules. Each item has pattern, optional replacement (omit to use the selected identity block), and optional paragraph.",
		},
	}
}

func handleRequestInterceptBefore(request []byte) []byte {
	var req pluginapi.RequestInterceptRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return mustErrorEnvelope("invalid_request", fmt.Sprintf("decode request.intercept_before request: %v", err))
	}
	// ToFormat is empty before credential selection. This hook never rewrites.
	return mustEnvelope(pluginapi.RequestInterceptResponse{})
}

func handleRequestInterceptAfter(request []byte) []byte {
	var req pluginapi.RequestInterceptRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return mustErrorEnvelope("invalid_request", fmt.Sprintf("decode request.intercept_after request: %v", err))
	}
	rewritten, changed := filterPayload(req.ToFormat, req.Body, activeConfig())
	if !changed {
		return mustEnvelope(pluginapi.RequestInterceptResponse{})
	}
	return mustEnvelope(pluginapi.RequestInterceptResponse{Body: rewritten})
}

func mustEnvelope(result any) []byte {
	raw, err := json.Marshal(pluginabi.Envelope{OK: true, Result: mustRawMessage(result)})
	if err != nil {
		return mustErrorEnvelope("marshal_error", err.Error())
	}
	return raw
}

func mustErrorEnvelope(code, message string) []byte {
	raw, err := json.Marshal(pluginabi.Envelope{OK: false, Error: &pluginabi.Error{
		Code:    code,
		Message: message,
	}})
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"marshal_error","message":"failed to encode plugin response"}}`)
	}
	return raw
}

func mustRawMessage(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}
