package main

import "strings"

// textScope is the protocol position of one eligible string.
// initial is the first system or developer text. subsequent_system is a later
// message whose role is exactly system or developer. tool_description is a
// tool's own description field. User text, assistant text, tool names, and
// parameter schemas are never scopes, even when they contain a reminder tag.
type textScope string

const (
	scopeInitial          textScope = "initial"
	scopeSubsequentSystem textScope = "subsequent_system"
	scopeToolDescription  textScope = "tool_description"
)

type scopedSpan struct {
	span  stringSpan
	scope textScope
}

// collectScopedSpans selects the strings the enabled scopes may rewrite.
// It follows the known prompt and tool fields. It does not search every
// string in the payload, and a user message is not treated as a system
// reminder because it contains the characters "<system-reminder>".
func collectScopedSpans(body []byte, cfg config) ([]scopedSpan, error) {
	if len(body) == 0 || !cfg.scopes.any() {
		return nil, nil
	}
	start := skipASCIISpace(body, 0)
	end := trimRightASCIISpace(body)
	if start >= end || body[start] != '{' {
		return nil, nil
	}
	keys, err := objectKeys(body, rng{start: start, end: end})
	if err != nil {
		return nil, err
	}

	var spans []scopedSpan
	initial, hasInitial := findFirstSystemSpan(body)
	if cfg.scopes.initial && hasInitial {
		spans = append(spans, scopedSpan{span: initial, scope: scopeInitial})
	}
	if cfg.scopes.subsequentSystem {
		later, err := subsequentSystemSpans(body, keys, initial, hasInitial, 0)
		if err != nil {
			return nil, err
		}
		spans = append(spans, later...)
	}
	if cfg.scopes.toolDescriptions {
		tools, err := toolDescriptionSpans(body, keys, 0)
		if err != nil {
			return nil, err
		}
		spans = append(spans, tools...)
	}
	return spans, nil
}

func (s scopeSet) any() bool {
	return s.initial || s.subsequentSystem || s.toolDescriptions
}

func subsequentSystemSpans(body []byte, keys map[string]rng, initial stringSpan, hasInitial bool, depth int) ([]scopedSpan, error) {
	if depth > maxJSONDepth {
		return nil, errJSON
	}
	var spans []scopedSpan
	for _, key := range []string{"input", "messages"} {
		value, ok := keys[key]
		if !ok {
			continue
		}
		found, err := systemRoleSpans(body, value)
		if err != nil {
			return nil, err
		}
		for _, span := range found {
			if hasInitial && sameSpan(span, initial) {
				continue
			}
			spans = append(spans, scopedSpan{span: span, scope: scopeSubsequentSystem})
		}
	}
	if req, ok := keys["request"]; ok && isByte(body, req.start, '{') {
		sub, err := objectKeys(body, req)
		if err != nil {
			return nil, err
		}
		nested, err := subsequentSystemSpans(body, sub, initial, hasInitial, depth+1)
		if err != nil {
			return nil, err
		}
		spans = append(spans, nested...)
	}
	return spans, nil
}

func systemRoleSpans(body []byte, value rng) ([]stringSpan, error) {
	if !isByte(body, value.start, '[') {
		return nil, nil
	}
	elements, err := arrayElements(body, value)
	if err != nil {
		return nil, err
	}
	var spans []stringSpan
	for _, element := range elements {
		if !isByte(body, element.start, '{') {
			continue
		}
		keys, err := objectKeys(body, element)
		if err != nil {
			return nil, err
		}
		roleRange, ok := keys["role"]
		if !ok || !isByte(body, roleRange.start, '"') {
			continue
		}
		role, err := decodeRangeString(body, roleRange)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "system", "developer":
		default:
			continue
		}
		content, ok := keys["content"]
		if !ok {
			continue
		}
		span, found, err := firstText(body, content)
		if err != nil {
			return nil, err
		}
		if found {
			spans = append(spans, span)
		}
	}
	return spans, nil
}

func toolDescriptionSpans(body []byte, keys map[string]rng, depth int) ([]scopedSpan, error) {
	if depth > maxJSONDepth {
		return nil, errJSON
	}
	var spans []scopedSpan
	if tools, ok := keys["tools"]; ok {
		found, err := descriptionsInTools(body, tools)
		if err != nil {
			return nil, err
		}
		for _, span := range found {
			spans = append(spans, scopedSpan{span: span, scope: scopeToolDescription})
		}
	}
	if req, ok := keys["request"]; ok && isByte(body, req.start, '{') {
		sub, err := objectKeys(body, req)
		if err != nil {
			return nil, err
		}
		nested, err := toolDescriptionSpans(body, sub, depth+1)
		if err != nil {
			return nil, err
		}
		spans = append(spans, nested...)
	}
	return spans, nil
}

// descriptionsInTools reads only description strings. Nested parameter
// schemas and tool names are not returned.
func descriptionsInTools(body []byte, value rng) ([]stringSpan, error) {
	if !isByte(body, value.start, '[') {
		return nil, nil
	}
	elements, err := arrayElements(body, value)
	if err != nil {
		return nil, err
	}
	var spans []stringSpan
	for _, element := range elements {
		if !isByte(body, element.start, '{') {
			continue
		}
		keys, err := objectKeys(body, element)
		if err != nil {
			return nil, err
		}
		if fn, ok := keys["function"]; ok && isByte(body, fn.start, '{') {
			span, found, err := objectDescription(body, fn)
			if err != nil {
				return nil, err
			}
			if found {
				spans = append(spans, span)
			}
			continue
		}
		if decls, ok := keys["functionDeclarations"]; ok {
			found, err := declarationDescriptions(body, decls)
			if err != nil {
				return nil, err
			}
			spans = append(spans, found...)
			continue
		}
		if desc, ok := keys["description"]; ok {
			span, found, err := nonEmptyString(body, desc)
			if err != nil {
				return nil, err
			}
			if found {
				spans = append(spans, span)
			}
		}
	}
	return spans, nil
}

func declarationDescriptions(body []byte, value rng) ([]stringSpan, error) {
	if !isByte(body, value.start, '[') {
		return nil, nil
	}
	elements, err := arrayElements(body, value)
	if err != nil {
		return nil, err
	}
	var spans []stringSpan
	for _, element := range elements {
		if !isByte(body, element.start, '{') {
			continue
		}
		span, found, err := objectDescription(body, element)
		if err != nil {
			return nil, err
		}
		if found {
			spans = append(spans, span)
		}
	}
	return spans, nil
}

func objectDescription(body []byte, value rng) (stringSpan, bool, error) {
	keys, err := objectKeys(body, value)
	if err != nil {
		return stringSpan{}, false, err
	}
	desc, ok := keys["description"]
	if !ok {
		return stringSpan{}, false, nil
	}
	return nonEmptyString(body, desc)
}

func sameSpan(a, b stringSpan) bool {
	return a.contentStart == b.contentStart && a.contentEnd == b.contentEnd
}
