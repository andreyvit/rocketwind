package rocketwind

import (
	"iter"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Segments splits on a separator outside brackets, braces, parentheses and quotes.
// Returned strings share input storage. Iteration uses no heap allocations.
func Segments(input string, separator byte) iter.Seq[string] {
	return func(yield func(string) bool) {
		var local [64]byte
		stack := local[:0]
		start := 0
		for i := 0; i < len(input); i++ {
			c := input[i]
			if len(stack) == 0 && c == separator {
				if !yield(input[start:i]) {
					return
				}
				start = i + 1
				continue
			}
			switch c {
			case '\\':
				i++
			case '\'', '"':
				for i++; i < len(input); i++ {
					if input[i] == '\\' {
						i++
					} else if input[i] == c {
						break
					}
				}
			case '(':
				stack = append(stack, ')')
			case '[':
				stack = append(stack, ']')
			case '{':
				stack = append(stack, '}')
			case ')', ']', '}':
				if len(stack) > 0 && stack[len(stack)-1] == c {
					stack = stack[:len(stack)-1]
				}
			}
		}
		yield(input[start:])
	}
}

// Stack buffers avoid allocating a collector for short candidate components.
func appendSegments(dst []string, input string, separator byte) []string {
	for part := range Segments(input, separator) {
		dst = append(dst, part)
	}
	return dst
}

// Escape serializes a CSS identifier according to CSSOM.
func Escape(s string) string {
	if s == "-" {
		return `\-`
	}
	var b strings.Builder
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		hex := c > 0 && c < 32 || c == 127 || c >= '0' && c <= '9' && (i == 0 || i == 1 && s[0] == '-')
		if !hex && (c >= 128 || c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			continue
		}
		if b.Cap() == 0 {
			b.Grow(len(s) + 8)
		}
		b.WriteString(s[start:i])
		switch {
		case c == 0:
			b.WriteRune(utf8.RuneError)
		case hex:
			b.WriteByte('\\')
			b.WriteString(strconv.FormatUint(uint64(c), 16))
			b.WriteByte(' ')
		default:
			b.WriteByte('\\')
			b.WriteByte(c)
		}
		start = i + 1
	}
	if start == 0 {
		return s
	}
	b.WriteString(s[start:])
	return b.String()
}

// Unescape decodes CSS escapes, replacing invalid code points with U+FFFD.
func Unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		start := i
		var r rune
		for i < len(s) && i-start < 6 {
			c := s[i]
			var n byte
			switch {
			case c >= '0' && c <= '9':
				n = c - '0'
			case c >= 'a' && c <= 'f':
				n = c - 'a' + 10
			case c >= 'A' && c <= 'F':
				n = c - 'A' + 10
			default:
				goto endHex
			}
			r = r*16 + rune(n)
			i++
		}
	endHex:
		if i == start {
			b.WriteByte(s[i])
			continue
		}
		if r == 0 || !utf8.ValidRune(r) {
			r = utf8.RuneError
		}
		b.WriteRune(r)
		if i < len(s) && cssSpace(s[i]) {
			i++
		}
		i--
	}
	return b.String()
}

func cssSpace(c byte) bool { return c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == '\f' }
