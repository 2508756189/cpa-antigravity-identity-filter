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

// rewriteBody replaces the identity paragraph inside the first system or
// developer text. Every byte outside that JSON string is copied through, and
// bytes inside the string that the rules do not cover keep their original
// JSON encoding.
func rewriteBody(body []byte, cfg config) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	span, ok := findFirstSystemSpan(body)
	if !ok {
		return body, false
	}
	decoded, rawIndex, err := decodeJSONString(body[span.contentStart:span.contentEnd])
	if err != nil || len(rawIndex) != len(decoded)+1 {
		return body, false
	}
	edits := editsForText(decoded, cfg)
	if len(edits) == 0 {
		return body, false
	}

	rawEdits := make([]rawEdit, 0, len(edits))
	for _, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(decoded) {
			return body, false
		}
		absStart := span.contentStart + rawIndex[edit.start]
		absEnd := span.contentStart + rawIndex[edit.end]
		if absStart < span.contentStart || absEnd > span.contentEnd || absEnd < absStart {
			return body, false
		}
		repl, err := encodeJSONStringContent(edit.replacement)
		if err != nil {
			return body, false
		}
		rawEdits = append(rawEdits, rawEdit{start: absStart, end: absEnd, repl: repl})
	}
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
	return out, true
}
