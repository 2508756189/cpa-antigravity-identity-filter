package main

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	asCodexPhrase       = "As Codex,"
	asAntigravityPhrase = "As Antigravity,"
)

// Built-in rules are case-sensitive and anchored at the start of the first
// system/developer text (after Unicode whitespace). A match consumes the
// rest of that first paragraph, through the next blank line.
var builtinPatterns = []struct {
	applies func(clientSet) bool
	re      *regexp.Regexp
}{
	{func(c clientSet) bool { return c.codex }, regexp.MustCompile(`^You are OpenAI Codex,`)},
	{func(c clientSet) bool { return c.codex }, regexp.MustCompile(`^You are Codex,`)},
	{func(c clientSet) bool { return c.claudeCode }, regexp.MustCompile(`^You are Claude Code,`)},
	{func(c clientSet) bool { return c.claudeAgentSDK }, regexp.MustCompile(`^You are a Claude agent,`)},
	{func(c clientSet) bool { return c.zcode }, regexp.MustCompile(`^You are ZCode,`)},
	{func(c clientSet) bool { return c.hermes }, regexp.MustCompile(`^You are Hermes Agent,`)},
	{func(c clientSet) bool { return c.opencode }, regexp.MustCompile(`^You are OpenCode,`)},
	{func(c clientSet) bool { return c.opencode }, regexp.MustCompile(`^You are opencode\b`)},
	{func(c clientSet) bool { return c.openclaw }, regexp.MustCompile(`^You are OpenClaw,`)},
}

type textEdit struct {
	start       int
	end         int
	replacement string
}

// editsForText returns non-overlapping replacements inside one prompt string.
// Offsets refer to the original string. The anchored paragraph rule runs at
// most once. The optional "As Codex," phrase also runs at most once, and only
// outside the paragraph that was replaced.
func editsForText(text string, cfg config) []textEdit {
	lead := leadingSpaceLen(text)
	rest := text[lead:]
	var edits []textEdit
	if !hasOfficialIdentity(rest) {
		if edit, ok := anchoredEdit(rest, cfg); ok {
			edit.start += lead
			edit.end += lead
			if text[edit.start:edit.end] != edit.replacement {
				edits = append(edits, edit)
			}
		}
	}
	if cfg.replaceAsCodex {
		if start := indexPhrase(text, asCodexPhrase); start >= 0 {
			end := start + len(asCodexPhrase)
			if !overlaps(edits, start, end) {
				edits = append(edits, textEdit{start: start, end: end, replacement: asAntigravityPhrase})
			}
		}
	}
	return edits
}

func hasOfficialIdentity(rest string) bool {
	return strings.HasPrefix(rest, identityCodingBlock) || strings.HasPrefix(rest, identityGenericBlock)
}

func anchoredEdit(rest string, cfg config) (textEdit, bool) {
	for _, pattern := range builtinPatterns {
		if !pattern.applies(cfg.clients) {
			continue
		}
		if edit, ok := paragraphEdit(rest, pattern.re, cfg.block()); ok {
			return edit, true
		}
	}
	for _, rule := range cfg.custom {
		replacement := rule.replacement
		if rule.useIdentity {
			replacement = cfg.block()
		}
		if rule.paragraph {
			if edit, ok := paragraphEdit(rest, rule.re, replacement); ok {
				return edit, true
			}
			continue
		}
		loc := rule.re.FindStringIndex(rest)
		if loc == nil || loc[0] != 0 || loc[1] == loc[0] {
			continue
		}
		return textEdit{start: 0, end: loc[1], replacement: replacement}, true
	}
	return textEdit{}, false
}

func paragraphEdit(rest string, re *regexp.Regexp, replacement string) (textEdit, bool) {
	loc := re.FindStringIndex(rest)
	if loc == nil || loc[0] != 0 || loc[1] == loc[0] {
		return textEdit{}, false
	}
	end := paragraphEnd(rest)
	if end < loc[1] {
		end = loc[1]
	}
	return textEdit{start: 0, end: end, replacement: replacement}, true
}

// paragraphEnd is the index of the blank-line separator, which is not part of
// the paragraph. A blank line is a newline, optional spaces or tabs, and
// another newline. Without one, the paragraph runs to the end of the string.
func paragraphEnd(s string) int {
	for i := 0; i < len(s); {
		if s[i] != '\n' && s[i] != '\r' {
			i++
			continue
		}
		width := 1
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			width = 2
		}
		j := i + width
		k := j
		for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
			k++
		}
		if k < len(s) && (s[k] == '\n' || s[k] == '\r') {
			return i
		}
		i = j
	}
	return len(s)
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
