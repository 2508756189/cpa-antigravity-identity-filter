package main

import (
	"strings"
	"unicode/utf8"
)

const maxJSONDepth = 256

type rng struct {
	start int
	end   int
}

// stringSpan is the raw byte range inside a JSON string's quotes.
type stringSpan struct {
	contentStart int
	contentEnd   int
}

type parser struct {
	b     []byte
	i     int
	depth int
}

func findFirstSystemSpan(body []byte) (stringSpan, bool) {
	start := skipASCIISpace(body, 0)
	end := trimRightASCIISpace(body)
	if start >= end || body[start] != '{' {
		return stringSpan{}, false
	}
	span, ok, err := locateObject(body, start, end, 0)
	if err != nil || !ok {
		return stringSpan{}, false
	}
	return span, true
}

func locateObject(body []byte, start, end, depth int) (stringSpan, bool, error) {
	if depth > maxJSONDepth {
		return stringSpan{}, false, errJSON
	}
	p := &parser{b: body[:end], i: start}
	keys, err := p.scanObject()
	if err != nil {
		return stringSpan{}, false, err
	}
	p.skipASCII()
	if p.i != end {
		return stringSpan{}, false, errJSON
	}
	return choosePrompt(body, keys, depth)
}

// choosePrompt picks the first non-empty system/developer text in protocol order.
// instructions and input cover OpenAI Responses, system covers Anthropic,
// messages covers Chat Completions, and the Gemini names cover systemInstruction
// including the Antigravity {"request": ...} wrapper.
func choosePrompt(body []byte, keys map[string]rng, depth int) (stringSpan, bool, error) {
	order := []string{
		"instructions",
		"input",
		"system",
		"messages",
		"systemInstruction",
		"system_instruction",
		"request",
	}
	for _, key := range order {
		value, ok := keys[key]
		if !ok {
			continue
		}
		span, found, err := textForKey(body, key, value, depth)
		if err != nil {
			return stringSpan{}, false, err
		}
		if found {
			return span, true, nil
		}
	}
	return stringSpan{}, false, nil
}

func textForKey(body []byte, key string, value rng, depth int) (stringSpan, bool, error) {
	switch key {
	case "instructions":
		return nonEmptyString(body, value)
	case "input", "messages":
		return firstRoleText(body, value)
	case "system":
		return firstText(body, value)
	case "systemInstruction", "system_instruction":
		return firstGeminiText(body, value)
	case "request":
		if !isByte(body, value.start, '{') {
			return stringSpan{}, false, nil
		}
		return locateObject(body, value.start, value.end, depth+1)
	default:
		return stringSpan{}, false, nil
	}
}

func nonEmptyString(body []byte, value rng) (stringSpan, bool, error) {
	if !isByte(body, value.start, '"') || value.end < value.start+2 {
		return stringSpan{}, false, nil
	}
	span := stringSpan{contentStart: value.start + 1, contentEnd: value.end - 1}
	if span.contentStart > span.contentEnd || span.contentEnd > len(body) {
		return stringSpan{}, false, errJSON
	}
	if span.contentStart == span.contentEnd {
		return stringSpan{}, false, nil
	}
	decoded, _, err := decodeJSONString(body[span.contentStart:span.contentEnd])
	if err != nil {
		return stringSpan{}, false, err
	}
	// Whitespace-only values are not a prompt. Empty instructions ("") already
	// falls through; " ", "\n", and "\t" must do the same so a later
	// developer/system identity is still eligible.
	if strings.TrimSpace(decoded) == "" {
		return stringSpan{}, false, nil
	}
	return span, true, nil
}

func firstText(body []byte, value rng) (stringSpan, bool, error) {
	if value.start >= value.end || value.start >= len(body) {
		return stringSpan{}, false, nil
	}
	switch body[value.start] {
	case '"':
		return nonEmptyString(body, value)
	case '[':
		return firstTextInArray(body, value)
	case '{':
		return firstBlockText(body, value)
	default:
		return stringSpan{}, false, nil
	}
}

func firstTextInArray(body []byte, value rng) (stringSpan, bool, error) {
	elements, err := arrayElements(body, value)
	if err != nil {
		return stringSpan{}, false, err
	}
	for _, element := range elements {
		if element.start >= len(body) {
			continue
		}
		switch body[element.start] {
		case '"':
			span, found, err := nonEmptyString(body, element)
			if err != nil || found {
				return span, found, err
			}
		case '{':
			span, found, err := firstBlockText(body, element)
			if err != nil || found {
				return span, found, err
			}
		}
	}
	return stringSpan{}, false, nil
}

func firstBlockText(body []byte, value rng) (stringSpan, bool, error) {
	keys, err := objectKeys(body, value)
	if err != nil {
		return stringSpan{}, false, err
	}
	text, ok := keys["text"]
	if !ok {
		return stringSpan{}, false, nil
	}
	if typeValue, exists := keys["type"]; exists {
		if !isByte(body, typeValue.start, '"') {
			return stringSpan{}, false, nil
		}
		kind, err := decodeRangeString(body, typeValue)
		if err != nil {
			return stringSpan{}, false, err
		}
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "", "text", "input_text":
		default:
			return stringSpan{}, false, nil
		}
	}
	return nonEmptyString(body, text)
}

func firstRoleText(body []byte, value rng) (stringSpan, bool, error) {
	if !isByte(body, value.start, '[') {
		return stringSpan{}, false, nil
	}
	elements, err := arrayElements(body, value)
	if err != nil {
		return stringSpan{}, false, err
	}
	for _, element := range elements {
		if !isByte(body, element.start, '{') {
			continue
		}
		keys, err := objectKeys(body, element)
		if err != nil {
			return stringSpan{}, false, err
		}
		roleRange, ok := keys["role"]
		if !ok || !isByte(body, roleRange.start, '"') {
			continue
		}
		role, err := decodeRangeString(body, roleRange)
		if err != nil {
			return stringSpan{}, false, err
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
		if err != nil || found {
			return span, found, err
		}
	}
	return stringSpan{}, false, nil
}

func firstGeminiText(body []byte, value rng) (stringSpan, bool, error) {
	if value.start >= len(body) {
		return stringSpan{}, false, nil
	}
	switch body[value.start] {
	case '"':
		return nonEmptyString(body, value)
	case '{':
		keys, err := objectKeys(body, value)
		if err != nil {
			return stringSpan{}, false, err
		}
		if parts, ok := keys["parts"]; ok {
			return firstPartText(body, parts)
		}
		if text, ok := keys["text"]; ok {
			return nonEmptyString(body, text)
		}
		return stringSpan{}, false, nil
	default:
		return stringSpan{}, false, nil
	}
}

func firstPartText(body []byte, value rng) (stringSpan, bool, error) {
	if !isByte(body, value.start, '[') {
		return stringSpan{}, false, nil
	}
	elements, err := arrayElements(body, value)
	if err != nil {
		return stringSpan{}, false, err
	}
	for _, element := range elements {
		if !isByte(body, element.start, '{') {
			continue
		}
		keys, err := objectKeys(body, element)
		if err != nil {
			return stringSpan{}, false, err
		}
		text, ok := keys["text"]
		if !ok {
			continue
		}
		span, found, err := nonEmptyString(body, text)
		if err != nil || found {
			return span, found, err
		}
	}
	return stringSpan{}, false, nil
}

func objectKeys(body []byte, value rng) (map[string]rng, error) {
	if !isByte(body, value.start, '{') {
		return nil, errJSON
	}
	p := &parser{b: body[:value.end], i: value.start}
	keys, err := p.scanObject()
	if err != nil {
		return nil, err
	}
	p.skipASCII()
	if p.i != value.end {
		return nil, errJSON
	}
	return keys, nil
}

func arrayElements(body []byte, value rng) ([]rng, error) {
	if !isByte(body, value.start, '[') {
		return nil, errJSON
	}
	p := &parser{b: body[:value.end], i: value.start}
	elements, err := p.scanArray()
	if err != nil {
		return nil, err
	}
	p.skipASCII()
	if p.i != value.end {
		return nil, errJSON
	}
	return elements, nil
}

func decodeRangeString(body []byte, value rng) (string, error) {
	if !isByte(body, value.start, '"') || value.end < value.start+2 {
		return "", errJSON
	}
	decoded, _, err := decodeJSONString(body[value.start+1 : value.end-1])
	return decoded, err
}

func (p *parser) scanObject() (map[string]rng, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()
	if p.i >= len(p.b) || p.b[p.i] != '{' {
		return nil, errJSON
	}
	p.i++
	keys := make(map[string]rng)
	p.skipASCII()
	if p.i < len(p.b) && p.b[p.i] == '}' {
		p.i++
		return keys, nil
	}
	for {
		p.skipASCII()
		key, err := p.readString()
		if err != nil {
			return nil, err
		}
		p.skipASCII()
		if err := p.expect(':'); err != nil {
			return nil, err
		}
		p.skipASCII()
		start := p.i
		if err := p.skipValue(); err != nil {
			return nil, err
		}
		if _, exists := keys[key]; !exists {
			keys[key] = rng{start: start, end: p.i}
		}
		p.skipASCII()
		if p.i >= len(p.b) {
			return nil, errJSON
		}
		switch p.b[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return keys, nil
		default:
			return nil, errJSON
		}
	}
}

func (p *parser) scanArray() ([]rng, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()
	if p.i >= len(p.b) || p.b[p.i] != '[' {
		return nil, errJSON
	}
	p.i++
	var elements []rng
	p.skipASCII()
	if p.i < len(p.b) && p.b[p.i] == ']' {
		p.i++
		return elements, nil
	}
	for {
		p.skipASCII()
		start := p.i
		if err := p.skipValue(); err != nil {
			return nil, err
		}
		elements = append(elements, rng{start: start, end: p.i})
		p.skipASCII()
		if p.i >= len(p.b) {
			return nil, errJSON
		}
		switch p.b[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return elements, nil
		default:
			return nil, errJSON
		}
	}
}

func (p *parser) skipValue() error {
	p.skipASCII()
	if p.i >= len(p.b) {
		return errJSON
	}
	switch p.b[p.i] {
	case '{':
		_, err := p.scanObject()
		return err
	case '[':
		_, err := p.scanArray()
		return err
	case '"':
		return p.skipString()
	case 't':
		return p.skipLiteral("true")
	case 'f':
		return p.skipLiteral("false")
	case 'n':
		return p.skipLiteral("null")
	default:
		if p.b[p.i] == '-' || isDigit(p.b[p.i]) {
			return p.skipNumber()
		}
		return errJSON
	}
}

func (p *parser) readString() (string, error) {
	start := p.i
	if err := p.skipString(); err != nil {
		return "", err
	}
	if start >= len(p.b) || p.b[start] != '"' || p.i < start+2 {
		return "", errJSON
	}
	decoded, _, err := decodeJSONString(p.b[start+1 : p.i-1])
	return decoded, err
}

func (p *parser) skipString() error {
	if p.i >= len(p.b) || p.b[p.i] != '"' {
		return errJSON
	}
	p.i++
	for p.i < len(p.b) {
		c := p.b[p.i]
		if c == '"' {
			p.i++
			return nil
		}
		if c == '\\' {
			p.i++
			if p.i >= len(p.b) {
				return errJSON
			}
			esc := p.b[p.i]
			p.i++
			if esc == 'u' {
				if p.i+4 > len(p.b) {
					return errJSON
				}
				for _, hex := range p.b[p.i : p.i+4] {
					if !isHex(hex) {
						return errJSON
					}
				}
				p.i += 4
			}
			continue
		}
		if c < 0x20 {
			return errJSON
		}
		if c < 0x80 {
			p.i++
			continue
		}
		r, size := utf8.DecodeRune(p.b[p.i:])
		if r == utf8.RuneError && size == 1 {
			return errJSON
		}
		p.i += size
	}
	return errJSON
}

func (p *parser) skipNumber() error {
	if p.i < len(p.b) && p.b[p.i] == '-' {
		p.i++
	}
	if p.i >= len(p.b) || !isDigit(p.b[p.i]) {
		return errJSON
	}
	if p.b[p.i] == '0' {
		p.i++
	} else {
		for p.i < len(p.b) && isDigit(p.b[p.i]) {
			p.i++
		}
	}
	if p.i < len(p.b) && p.b[p.i] == '.' {
		p.i++
		if p.i >= len(p.b) || !isDigit(p.b[p.i]) {
			return errJSON
		}
		for p.i < len(p.b) && isDigit(p.b[p.i]) {
			p.i++
		}
	}
	if p.i < len(p.b) && (p.b[p.i] == 'e' || p.b[p.i] == 'E') {
		p.i++
		if p.i < len(p.b) && (p.b[p.i] == '+' || p.b[p.i] == '-') {
			p.i++
		}
		if p.i >= len(p.b) || !isDigit(p.b[p.i]) {
			return errJSON
		}
		for p.i < len(p.b) && isDigit(p.b[p.i]) {
			p.i++
		}
	}
	return nil
}

func (p *parser) skipLiteral(lit string) error {
	if p.i+len(lit) > len(p.b) || string(p.b[p.i:p.i+len(lit)]) != lit {
		return errJSON
	}
	end := p.i + len(lit)
	if end < len(p.b) {
		c := p.b[end]
		if isDigit(c) || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' {
			return errJSON
		}
	}
	p.i = end
	return nil
}

func (p *parser) expect(c byte) error {
	if p.i >= len(p.b) || p.b[p.i] != c {
		return errJSON
	}
	p.i++
	return nil
}

func (p *parser) skipASCII() {
	for p.i < len(p.b) {
		switch p.b[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) enter() error {
	p.depth++
	if p.depth > maxJSONDepth {
		return errJSON
	}
	return nil
}

func (p *parser) leave() {
	if p.depth > 0 {
		p.depth--
	}
}

func isByte(body []byte, index int, want byte) bool {
	return index >= 0 && index < len(body) && body[index] == want
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func skipASCIISpace(body []byte, i int) int {
	for i < len(body) {
		switch body[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func trimRightASCIISpace(body []byte) int {
	end := len(body)
	for end > 0 {
		switch body[end-1] {
		case ' ', '\t', '\n', '\r':
			end--
		default:
			return end
		}
	}
	return end
}
