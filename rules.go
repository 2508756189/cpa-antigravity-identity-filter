package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	asCodexPhrase       = "As Codex,"
	asAntigravityPhrase = "As Antigravity,"

	// maxIdentitySentenceBytes bounds a declaration so a missing terminator
	// cannot consume the rest of the prompt. Known client sentences are shorter.
	maxIdentitySentenceBytes = 320

	desktopContextHeader  = "# Codex desktop context"
	desktopContextNeutral = "# Desktop context"
	poweredByPrefix       = "You are powered by the model named "
	poweredByNeutral      = "You are powered by the model named Antigravity."
)

// identityRule matches one client declaration. The match is the single
// sentence that begins with prefix; it does not continue through a blank line.
type identityRule struct {
	id           string
	prefix       string
	wordBoundary bool
	applies      func(clientSet) bool
}

// Built-in rules are case-sensitive and anchored at the start of an eligible
// text (after Unicode whitespace). Order is fixed: a more specific Codex
// prefix is tried before the shorter one.
var builtinIdentityRules = []identityRule{
	{id: "openai-codex", prefix: "You are OpenAI Codex,", applies: func(c clientSet) bool { return c.codex }},
	{id: "codex", prefix: "You are Codex,", applies: func(c clientSet) bool { return c.codex }},
	{id: "claude-code", prefix: "You are Claude Code,", applies: func(c clientSet) bool { return c.claudeCode }},
	{id: "claude-agent", prefix: "You are a Claude agent,", applies: func(c clientSet) bool { return c.claudeAgentSDK }},
	{id: "zcode", prefix: "You are ZCode,", applies: func(c clientSet) bool { return c.zcode }},
	{id: "hermes", prefix: "You are Hermes Agent,", applies: func(c clientSet) bool { return c.hermes }},
	{id: "opencode", prefix: "You are OpenCode,", applies: func(c clientSet) bool { return c.opencode }},
	{id: "opencode-lower", prefix: "You are opencode", wordBoundary: true, applies: func(c clientSet) bool { return c.opencode }},
	{id: "openclaw", prefix: "You are OpenClaw,", applies: func(c clientSet) bool { return c.openclaw }},
}

type textEdit struct {
	start       int
	end         int
	replacement string
	ruleID      string
}

// editsForText returns non-overlapping replacements inside one eligible
// prompt string. Offsets refer to the original string. Identity matching runs
// at most once and stops at the end of that sentence. Optional neutralization
// and the "As Codex," phrase are separate rules and do not walk other messages.
func editsForText(text string, cfg config, scope textScope) []textEdit {
	lead := leadingSpaceLen(text)
	rest := text[lead:]
	var edits []textEdit
	if !hasOfficialIdentity(rest) {
		if edit, ok := anchoredIdentityEdit(rest, cfg); ok {
			edit.start += lead
			edit.end += lead
			if edit.replacement == cfg.block() && strings.Contains(text, cfg.block()) {
				// The selected official block is already present. Drop the
				// third-party sentence instead of inserting a second copy.
				edit.replacement = ""
			}
			if text[edit.start:edit.end] != edit.replacement {
				edits = append(edits, edit)
			}
		}
	}
	if cfg.neutralize.desktopContext {
		edits = appendNonOverlapping(text, edits, desktopHeaderEdits(text))
	}
	if cfg.neutralize.poweredByModel {
		edits = appendNonOverlapping(text, edits, poweredByEdits(text))
	}
	// "As Codex," stays on the initial system text only. Other scopes do not
	// scan for it, so a tool description or later message is left alone.
	if scope == scopeInitial && cfg.replaceAsCodex {
		if start := indexPhrase(text, asCodexPhrase); start >= 0 {
			end := start + len(asCodexPhrase)
			if !overlaps(edits, start, end) && text[start:end] != asAntigravityPhrase {
				edits = append(edits, textEdit{
					start:       start,
					end:         end,
					replacement: asAntigravityPhrase,
					ruleID:      "as-codex",
				})
			}
		}
	}
	return edits
}

func appendNonOverlapping(text string, edits, extra []textEdit) []textEdit {
	for _, edit := range extra {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(text) {
			continue
		}
		if overlaps(edits, edit.start, edit.end) {
			continue
		}
		if text[edit.start:edit.end] == edit.replacement {
			continue
		}
		edits = append(edits, edit)
	}
	return edits
}

func hasOfficialIdentity(rest string) bool {
	return strings.HasPrefix(rest, identityCodingBlock) || strings.HasPrefix(rest, identityGenericBlock)
}

func anchoredIdentityEdit(rest string, cfg config) (textEdit, bool) {
	for _, rule := range builtinIdentityRules {
		if rule.applies != nil && !rule.applies(cfg.clients) {
			continue
		}
		end, ok := identitySentenceEnd(rest, rule.prefix, rule.wordBoundary)
		if !ok {
			continue
		}
		return textEdit{start: 0, end: end, replacement: cfg.block(), ruleID: rule.id}, true
	}
	for index, rule := range cfg.custom {
		replacement := rule.replacement
		if rule.useIdentity {
			replacement = cfg.block()
		}
		loc := rule.re.FindStringIndex(rest)
		if loc == nil || loc[0] != 0 || loc[1] == loc[0] {
			continue
		}
		end := loc[1]
		if rule.paragraph {
			// paragraph:true used to consume through the next blank line.
			// It now means "this sentence": clamp to the terminator, and do
			// not apply when that bound is missing.
			sentenceEnd, ok := sentenceTerminator(rest)
			if !ok {
				continue
			}
			end = sentenceEnd
		}
		return textEdit{
			start:       0,
			end:         end,
			replacement: replacement,
			ruleID:      customRuleID(index),
		}, true
	}
	return textEdit{}, false
}

func customRuleID(index int) string {
	return "custom-" + itoa(index)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// identitySentenceEnd reports the byte length of one declaration sentence.
// The sentence must begin with prefix, stay on one line, and end at the first
// period that is followed by whitespace or the end of the text. A missing
// terminator, a newline, or a path-like token makes the declaration ambiguous
// and therefore not a match.
func identitySentenceEnd(rest, prefix string, wordBoundary bool) (int, bool) {
	if prefix == "" || !strings.HasPrefix(rest, prefix) {
		return 0, false
	}
	if wordBoundary && len(rest) > len(prefix) && isASCIIWord(rest[len(prefix)]) {
		return 0, false
	}
	end, ok := sentenceTerminator(rest)
	if !ok || end < len(prefix) {
		return 0, false
	}
	return end, true
}

func sentenceTerminator(s string) (int, bool) {
	limit := len(s)
	if limit > maxIdentitySentenceBytes {
		limit = maxIdentitySentenceBytes
	}
	for j := 0; j < limit; j++ {
		switch s[j] {
		case '\n', '\r', '/', '\\', '<', '>':
			return 0, false
		case '.':
			if j+1 == len(s) || isSentenceTrail(s[j+1]) {
				sentence := s[:j]
				if strings.Contains(sentence, ".codex") || strings.Contains(sentence, ".zcode") || strings.Contains(sentence, "://") {
					return 0, false
				}
				return j + 1, true
			}
		}
	}
	return 0, false
}

func isSentenceTrail(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func desktopHeaderEdits(text string) []textEdit {
	var edits []textEdit
	lineStart := 0
	for lineStart <= len(text) {
		lineEnd := lineStart
		for lineEnd < len(text) && text[lineEnd] != '\n' && text[lineEnd] != '\r' {
			lineEnd++
		}
		if text[lineStart:lineEnd] == desktopContextHeader {
			edits = append(edits, textEdit{
				start:       lineStart,
				end:         lineEnd,
				replacement: desktopContextNeutral,
				ruleID:      "desktop-context-header",
			})
		}
		if lineEnd >= len(text) {
			break
		}
		if text[lineEnd] == '\r' && lineEnd+1 < len(text) && text[lineEnd+1] == '\n' {
			lineStart = lineEnd + 2
			continue
		}
		lineStart = lineEnd + 1
	}
	return edits
}

func poweredByEdits(text string) []textEdit {
	var edits []textEdit
	from := 0
	for from < len(text) {
		rel := strings.Index(text[from:], poweredByPrefix)
		if rel < 0 {
			break
		}
		start := from + rel
		end, ok := poweredBySentenceEnd(text[start:])
		if !ok {
			next := start + len(poweredByPrefix)
			if next <= from {
				break
			}
			from = next
			continue
		}
		edits = append(edits, textEdit{
			start:       start,
			end:         start + end,
			replacement: poweredByNeutral,
			ruleID:      "powered-by-model",
		})
		from = start + end
	}
	return edits
}

func poweredBySentenceEnd(sentence string) (int, bool) {
	if !strings.HasPrefix(sentence, poweredByPrefix) {
		return 0, false
	}
	i := len(poweredByPrefix)
	if i >= len(sentence) || !isModelChar(sentence[i]) {
		return 0, false
	}
	limit := len(sentence)
	if limit > maxIdentitySentenceBytes {
		limit = maxIdentitySentenceBytes
	}
	j := i
	for j < limit {
		c := sentence[j]
		if c == '\n' || c == '\r' || c == '/' || c == '\\' || c == '<' || c == '>' || c == ' ' || c == '\t' {
			return 0, false
		}
		if c == '.' {
			if j+1 == len(sentence) || isSentenceTrail(sentence[j+1]) {
				return j + 1, true
			}
			if j+1 < limit && isModelChar(sentence[j+1]) {
				j++
				continue
			}
			return 0, false
		}
		if !isModelChar(c) {
			return 0, false
		}
		j++
	}
	return 0, false
}

func isModelChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' || b == ':' || b == '-'
}

func leadingSpaceLen(s string) int {
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			break
		}
		if !unicode.IsSpace(r) {
			break
		}
		i += size
	}
	return i
}

func indexPhrase(s, phrase string) int {
	if phrase == "" {
		return -1
	}
	from := 0
	for {
		rel := strings.Index(s[from:], phrase)
		if rel < 0 {
			return -1
		}
		start := from + rel
		if start == 0 || !isASCIIWord(s[start-1]) {
			return start
		}
		from = start + 1
		if from > len(s) {
			return -1
		}
	}
}

func isASCIIWord(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

func overlaps(edits []textEdit, start, end int) bool {
	for _, edit := range edits {
		if start < edit.end && edit.start < end {
			return true
		}
	}
	return false
}
