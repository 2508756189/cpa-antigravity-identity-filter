package main

import (
	"bytes"
	"sort"
	"strings"
)

type rawEdit struct {
	start int
	end   int
	repl  []byte
}

// filterPayload rewrites body only when the selected upstream format is
// Antigravity. Any other target, including the empty format seen before
// credential selection, is returned unchanged.
func filterPayload(toFormat string, body []byte, cfg config) ([]byte, bool) {
	if !targetsAntigravity(toFormat) {
		return body, false
	}
	return rewriteBody(body, cfg)
}

func targetsAntigravity(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), "antigravity")
}

// rewriteBody applies the enabled scopes. Bytes outside a selected JSON
// string are copied through. Bytes inside a selected string that no rule
// covers keep their original JSON encoding. A decode or overlap error leaves
// the whole body unchanged.
func rewriteBody(body []byte, cfg config) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	spans, err := collectScopedSpans(body, cfg)
	if err != nil || len(spans) == 0 {
		return body, false
	}

	var rawEdits []rawEdit
	var diag []diagRecord
	for _, scoped := range spans {
		edits, raws, ok := editsForSpan(body, scoped, cfg)
		if !ok {
			return body, false
		}
		if len(raws) == 0 {
			continue
		}
		rawEdits = append(rawEdits, raws...)
		diag = append(diag, aggregateDiag(edits, scoped.scope)...)
	}
	if len(rawEdits) == 0 {
		return body, false
	}
	diag = mergeDiag(diag)
	sort.Slice(rawEdits, func(i, j int) bool { return rawEdits[i].start > rawEdits[j].start })
	for i := 1; i < len(rawEdits); i++ {
		if rawEdits[i].end > rawEdits[i-1].start {
			return body, false
		}
	}

	out := body
	for _, edit := range rawEdits {
		next := make([]byte, 0, len(out)-(edit.end-edit.start)+len(edit.repl))
		next = append(next, out[:edit.start]...)
		next = append(next, edit.repl...)
		next = append(next, out[edit.end:]...)
		out = next
	}
	if bytes.Equal(out, body) {
		return body, false
	}
	emitDiagnostics(cfg, diag)
	return out, true
}

func editsForSpan(body []byte, scoped scopedSpan, cfg config) ([]textEdit, []rawEdit, bool) {
	span := scoped.span
	if span.contentStart < 0 || span.contentEnd < span.contentStart || span.contentEnd > len(body) {
		return nil, nil, false
	}
	decoded, rawIndex, err := decodeJSONString(body[span.contentStart:span.contentEnd])
	if err != nil || len(rawIndex) != len(decoded)+1 {
		return nil, nil, false
	}
	edits := editsForText(decoded, cfg, scoped.scope)
	if len(edits) == 0 {
		return nil, nil, true
	}
	raws := make([]rawEdit, 0, len(edits))
	for _, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(decoded) {
			return nil, nil, false
		}
		absStart := span.contentStart + rawIndex[edit.start]
		absEnd := span.contentStart + rawIndex[edit.end]
		if absStart < span.contentStart || absEnd > span.contentEnd || absEnd < absStart {
			return nil, nil, false
		}
		repl, err := encodeJSONStringContent(edit.replacement)
		if err != nil {
			return nil, nil, false
		}
		raws = append(raws, rawEdit{start: absStart, end: absEnd, repl: repl})
	}
	return edits, raws, true
}
