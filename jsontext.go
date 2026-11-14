package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

var errJSON = errors.New("invalid json")

// decodeJSONString decodes the raw contents of a JSON string (without the
// surrounding quotes). rawIndex[i] is the raw offset of decoded byte i, and
// rawIndex[len(decoded)] is len(raw). Splicing at those offsets keeps the
// encoding of every untouched byte.
func decodeJSONString(raw []byte) (string, []int, error) {
	var b strings.Builder
	b.Grow(len(raw))
	index := make([]int, 0, len(raw)+1)
	for i := 0; i < len(raw); {
		if raw[i] != '\\' {
			r, size := utf8.DecodeRune(raw[i:])
			if r == utf8.RuneError && size == 1 {
				return "", nil, fmt.Errorf("invalid UTF-8 in JSON string")
			}
			for k := 0; k < size; k++ {
				index = append(index, i+k)
			}
			b.Write(raw[i : i+size])
			i += size
			continue
		}
		if i+1 >= len(raw) {
			return "", nil, fmt.Errorf("truncated JSON escape")
		}
		start := i
		switch raw[i+1] {
		case '"', '\\', '/':
			index = append(index, start)
			b.WriteByte(raw[i+1])
			i += 2
		case 'b':
			index = append(index, start)
			b.WriteByte('\b')
			i += 2
		case 'f':
			index = append(index, start)
			b.WriteByte('\f')
			i += 2
		case 'n':
			index = append(index, start)
			b.WriteByte('\n')
			i += 2
		case 'r':
			index = append(index, start)
			b.WriteByte('\r')
			i += 2
		case 't':
			index = append(index, start)
			b.WriteByte('\t')
			i += 2
		case 'u':
			r, next, err := decodeUnicodeEscape(raw, i)
			if err != nil {
				return "", nil, err
			}
			var buf [utf8.UTFMax]byte
			n := utf8.EncodeRune(buf[:], r)
			for k := 0; k < n; k++ {
				index = append(index, start)
			}
			b.Write(buf[:n])
			i = next
		default:
			return "", nil, fmt.Errorf("unknown JSON escape")
		}
	}
	index = append(index, len(raw))
	return b.String(), index, nil
}

func decodeUnicodeEscape(raw []byte, i int) (rune, int, error) {
	r, next, err := readHex4(raw, i)
	if err != nil {
		return 0, 0, err
	}
	if utf16.IsSurrogate(r) {
		if next+1 < len(raw) && raw[next] == '\\' && raw[next+1] == 'u' {
			r2, next2, err2 := readHex4(raw, next)
			if err2 == nil {
				decoded := utf16.DecodeRune(r, r2)
				if decoded != utf8.RuneError {
					return decoded, next2, nil
				}
			}
		}
		return utf8.RuneError, next, nil
	}
	return r, next, nil
}

func readHex4(raw []byte, i int) (rune, int, error) {
	// i points at the backslash of \uXXXX.
	if i+6 > len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
		return 0, 0, fmt.Errorf("truncated unicode escape")
	}
	var value rune
	for _, c := range raw[i+2 : i+6] {
		value <<= 4
		switch {
		case c >= '0' && c <= '9':
			value += rune(c - '0')
		case c >= 'a' && c <= 'f':
			value += rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			value += rune(c-'A') + 10
		default:
			return 0, 0, fmt.Errorf("invalid unicode escape")
		}
	}
	return value, i + 6, nil
}

// encodeJSONStringContent returns the JSON encoding of s without quotes.
// HTML characters are left as themselves so a custom replacement is not
// rewritten into \u003c sequences.
func encodeJSONStringContent(s string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	raw := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, fmt.Errorf("unexpected JSON string encoding")
	}
	return raw[1 : len(raw)-1], nil
}
