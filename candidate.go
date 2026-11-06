package rocketwind

import (
	"strings"
	"unicode/utf8"
)

func validNamed(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !asciiLetter(c) && !digit(c) && c != '_' && c != '.' && c != '%' && c != '-' {
			return false
		}
	}
	return true
}

func parseArbitrary(s string) (value, dataType string, ok bool) {
	if len(s) < 3 {
		return
	}
	// The candidate parser has already selected brackets and expanded the
	// parenthesized custom-property shorthand before calling this helper.
	if !validArbitrary(s[1 : len(s)-1]) {
		return "", "", false
	}
	value = DecodeArbitraryValue(s[1 : len(s)-1])
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == ':' {
			dataType, value = value[:i], value[i+1:]
			if dataType == "" {
				return "", "", false
			}
			break
		}
		if c != '-' && !(c >= 'a' && c <= 'z') {
			break
		}
	}
	ok = strings.TrimSpace(value) != "" && validArbitrary(value)
	return
}

func validArbitrary(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	var local [32]byte
	stack := local[:0]
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			i++
		case '\'', '"':
			quote := c
			i++
			for ; i < len(s); i++ {
				if s[i] == '\\' {
					i++
				} else if s[i] == quote {
					break
				}
			}
		case '(':
			stack = append(stack, ')')
		case '[':
			stack = append(stack, ']')
		case '{':
			// A brace never opens a valid arbitrary-value context.
		case ')', ']', '}':
			if len(stack) == 0 {
				return false
			}
			if stack[len(stack)-1] == c {
				stack = stack[:len(stack)-1]
			} else if c != '}' {
				return false
			}
		case ';':
			if len(stack) == 0 {
				return false
			}
		}
	}
	return len(stack) == 0
}
