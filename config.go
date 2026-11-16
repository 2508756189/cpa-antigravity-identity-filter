package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// clientSet toggles each built-in identity rule. Missing keys keep the default.
type clientSet struct {
	codex          bool
	claudeCode     bool
	claudeAgentSDK bool
	zcode          bool
	hermes         bool
	opencode       bool
	openclaw       bool
}

type customRule struct {
	pattern     string
	re          *regexp.Regexp
	paragraph   bool
	replacement string
	useIdentity bool
}

// scopeSet selects which protocol positions may be rewritten.
// Only initial is on by default. The other scopes are exact opt-ins.
type scopeSet struct {
	initial          bool
	subsequentSystem bool
	toolDescriptions bool
}

// neutralizeSet holds optional exact-phrase rules. Both default off.
// They run only inside a scope that is already enabled.
type neutralizeSet struct {
	desktopContext bool
	poweredByModel bool
}

type config struct {
	identity       string
	replaceAsCodex bool
	clients        clientSet
	custom         []customRule
	scopes         scopeSet
	neutralize     neutralizeSet
	diagnostics    bool
}

func defaultConfig() config {
	return config{
		identity:       identityCoding,
		replaceAsCodex: true,
		clients: clientSet{
			codex:          true,
			claudeCode:     true,
			claudeAgentSDK: true,
			zcode:          true,
			hermes:         true,
			opencode:       true,
			openclaw:       true,
		},
		scopes: scopeSet{initial: true},
	}
}

func (c config) snapshot() config {
	out := c
	out.custom = append([]customRule(nil), c.custom...)
	return out
}

func (c config) block() string {
	return identityBlock(c.identity)
}

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

func configFromLifecycle(request []byte) (config, error) {
	var req lifecycleRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return config{}, fmt.Errorf("decode lifecycle request: %w", err)
	}
	return parseConfigYAML(req.ConfigYAML)
}

// parseConfigYAML reads the plugin config block. Host-injected keys such as
// enabled, priority, and store are ignored. An empty document is the default.
func parseConfigYAML(raw []byte) (config, error) {
	cfg := defaultConfig()
	if len(strings.TrimSpace(string(raw))) == 0 {
		return cfg, nil
	}

	var values map[string]any
	if err := yaml.Unmarshal(raw, &values); err != nil {
		return config{}, fmt.Errorf("decode config yaml: %w", err)
	}
	if values == nil {
		return cfg, nil
	}

	if value, exists := values["identity"]; exists {
		text, ok := value.(string)
		if !ok {
			return config{}, fmt.Errorf("identity must be a string")
		}
		switch strings.ToLower(strings.TrimSpace(text)) {
		case identityCoding:
			cfg.identity = identityCoding
		case identityGeneric:
			cfg.identity = identityGeneric
		default:
			return config{}, fmt.Errorf("identity must be one of: coding, generic")
		}
	}
	if value, exists := values["replace_as_codex"]; exists {
		parsed, err := parseBool(value)
		if err != nil {
			return config{}, fmt.Errorf("replace_as_codex %w", err)
		}
		cfg.replaceAsCodex = parsed
	}
	if value, exists := values["clients"]; exists {
		clients, err := parseClients(value, cfg.clients)
		if err != nil {
			return config{}, err
		}
		cfg.clients = clients
	}
	if value, exists := values["custom_rules"]; exists {
		rules, err := parseCustomRules(value)
		if err != nil {
			return config{}, err
		}
		cfg.custom = rules
	}
	if value, exists := values["scopes"]; exists {
		if err := parseScopes(value, &cfg.scopes); err != nil {
			return config{}, err
		}
	}
	if value, exists := values["neutralize"]; exists {
		if err := parseNeutralize(value, &cfg.neutralize); err != nil {
			return config{}, err
		}
	}
	if value, exists := values["diagnostics"]; exists {
		parsed, err := parseBool(value)
		if err != nil {
			return config{}, fmt.Errorf("diagnostics %w", err)
		}
		cfg.diagnostics = parsed
	}
	return cfg, nil
}

func parseScopes(value any, dest *scopeSet) error {
	return applyBoolFlags(value, map[string]*bool{
		"initial":           &dest.initial,
		"subsequent_system": &dest.subsequentSystem,
		"tool_descriptions": &dest.toolDescriptions,
	}, "scopes", "scope")
}

func parseNeutralize(value any, dest *neutralizeSet) error {
	return applyBoolFlags(value, map[string]*bool{
		"desktop_context":  &dest.desktopContext,
		"powered_by_model": &dest.poweredByModel,
	}, "neutralize", "neutralize")
}

func applyBoolFlags(value any, known map[string]*bool, mappingName, itemName string) error {
	if value == nil {
		return nil
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be a mapping", mappingName)
	}
	for key, raw := range fields {
		dest, exists := known[key]
		if !exists {
			return fmt.Errorf("unknown %s %q", itemName, key)
		}
		parsed, err := parseBool(raw)
		if err != nil {
			return fmt.Errorf("%s.%s %w", mappingName, key, err)
		}
		*dest = parsed
	}
	return nil
}

func parseBool(value any) (bool, error) {
	parsed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("must be a boolean")
	}
	return parsed, nil
}

func parseClients(value any, base clientSet) (clientSet, error) {
	fields, ok := value.(map[string]any)
	if !ok {
		if value == nil {
			return base, nil
		}
		return clientSet{}, fmt.Errorf("clients must be a mapping")
	}
	known := map[string]*bool{
		"codex":            &base.codex,
		"claude_code":      &base.claudeCode,
		"claude_agent_sdk": &base.claudeAgentSDK,
		"zcode":            &base.zcode,
		"hermes":           &base.hermes,
		"opencode":         &base.opencode,
		"openclaw":         &base.openclaw,
	}
	for key, raw := range fields {
		dest, exists := known[key]
		if !exists {
			return clientSet{}, fmt.Errorf("unknown client %q", key)
		}
		parsed, err := parseBool(raw)
		if err != nil {
			return clientSet{}, fmt.Errorf("clients.%s %w", key, err)
		}
		*dest = parsed
	}
	return base, nil
}

func parseCustomRules(value any) ([]customRule, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("custom_rules must be a list")
	}
	rules := make([]customRule, 0, len(items))
	for index, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("custom_rules[%d] must be a mapping", index)
		}
		pattern, ok := fields["pattern"].(string)
		if !ok || strings.TrimSpace(pattern) == "" {
			return nil, fmt.Errorf("custom_rules[%d].pattern must be a non-empty string", index)
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("custom_rules[%d].pattern: %w", index, err)
		}
		if loc := re.FindStringIndex(""); loc != nil && loc[1] == 0 {
			return nil, fmt.Errorf("custom_rules[%d].pattern must not match empty text", index)
		}
		rule := customRule{pattern: pattern, re: re, useIdentity: true}
		if raw, exists := fields["replacement"]; exists {
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("custom_rules[%d].replacement must be a string", index)
			}
			rule.replacement = text
			rule.useIdentity = false
		}
		if raw, exists := fields["paragraph"]; exists {
			parsed, err := parseBool(raw)
			if err != nil {
				return nil, fmt.Errorf("custom_rules[%d].paragraph %w", index, err)
			}
			rule.paragraph = parsed
		}
		rules = append(rules, rule)
	}
	return rules, nil
}
