package rocketwind

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Preprocess makes template shorthand readable by Extract. It preserves byte
// offsets and length. The extension may have a leading dot; unsupported formats
// pass through unchanged. No source code is evaluated.
func Preprocess(input, extension string) string {
	switch strings.TrimPrefix(extension, ".") {
	case "clj", "cljs", "cljc":
		return preprocessClojure(input)
	case "heex", "eex", "ex", "exs":
		return preprocessElixir(input)
	case "cshtml", "razor":
		return strings.ReplaceAll(strings.ReplaceAll(input, "@@", " @"), `@("@")`, "     @")
	case "haml":
		return preprocessHaml(input)
	case "json", "jsonl", "ndjson":
		return preprocessJSON(input)
	case "md", "mdx":
		return preprocessMarkdown(input)
	case "pug":
		return preprocessPug(input)
	case "rb", "erb":
		return preprocessRuby(input)
	case "slim", "slang":
		return preprocessSlim(input)
	case "svelte":
		return preprocessClassDirective(input)
	case "rs":
		return preprocessRust(input)
	case "tt", "tt2", "tx":
		return preprocessToolkit(input)
	case "twig":
		return preprocessTwig(input)
	case "vue":
		return preprocessVue(input)
	}
	return input
}

type sourceEdits struct {
	input string
	buf   []byte
}

func (e *sourceEdits) set(pos int, ch byte) {
	if e.buf == nil {
		if e.input[pos] == ch {
			return
		}
		e.buf = []byte(e.input)
	}
	e.buf[pos] = ch
}
func (e *sourceEdits) replace(start, end int, value string) {
	if e.buf == nil {
		if e.input[start:end] == value {
			return
		}
		e.buf = []byte(e.input)
	}
	copy(e.buf[start:end], value)
}
func (e *sourceEdits) result() string {
	if e.buf == nil {
		return e.input
	}
	return string(e.buf)
}

type scanBrackets struct {
	values [32]byte
	n      int
}

func (s *scanBrackets) push(ch byte) bool {
	if s.n == len(s.values) {
		return false
	}
	if ch == '(' {
		ch = ')'
	} else if ch == '<' {
		ch = '>'
	} else {
		ch += 2
	}
	s.values[s.n] = ch
	s.n++
	return true
}
func (s *scanBrackets) pop(ch byte) bool {
	// All callers check n before consuming a closing bracket.
	s.n--
	return s.values[s.n] == ch
}

func preprocessQuoted(c *scanCursor, e *sourceEdits, erase bool) {
	quote := c.curr()
	if erase {
		e.set(c.pos, ' ')
	}
	c.pos++
	for c.pos < len(c.input) {
		switch c.curr() {
		case '\\':
			c.pos += 2
		case quote:
			if erase {
				e.set(c.pos, ' ')
			}
			return
		default:
			c.pos++
		}
	}
}

func preprocessDot(c *scanCursor) bool {
	if !digit(c.prev()) || !digit(c.next()) {
		return true
	}
	next := *c
	next.pos++
	_, ok := (&scanVariantMachine{}).next(&next)
	return ok
}

func preprocessClassDirective(input string) string {
	for _, prefix := range []string{" ", "\t", "\n"} {
		input = strings.ReplaceAll(input, prefix+"class:", " class ")
	}
	return input
}

func preprocessJSON(input string) string {
	e := sourceEdits{input: input}
	c := scanCursor{input: input}
	for c.pos < len(input) {
		switch c.curr() {
		case '"':
			preprocessQuoted(&c, &e, false)
		case '[', '{', ']', '}':
			e.set(c.pos, ' ')
		}
		c.pos++
	}
	return e.result()
}

func preprocessToolkit(input string) string {
	e := sourceEdits{input: input}
	for i := 0; i+1 < len(input); i++ {
		if input[i] == '[' && input[i+1] == '%' {
			e.set(i, ' ')
		} else if input[i] == '%' && input[i+1] == ']' {
			e.set(i+1, ' ')
		}
	}
	return e.result()
}

func preprocessTwig(input string) string {
	e := sourceEdits{input: input}
	depth := 0
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '(':
			if depth > 0 {
				depth++
			} else if strings.HasSuffix(input[:i], "addClass") || strings.HasSuffix(input[:i], "removeClass") {
				depth++
				e.set(i, ' ')
			}
		case ')':
			if depth > 0 {
				depth--
				if depth == 0 {
					e.set(i, ' ')
				}
			}
		}
	}
	return e.result()
}

func preprocessMarkdown(input string) string {
	e := sourceEdits{input: input}
	depth := 0
	directive := false
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if !directive {
			if ch == '{' {
				e.set(i, ' ')
				directive = true
			}
			continue
		}
		switch ch {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if depth > 0 {
				depth--
			} else if ch == '}' {
				e.set(i, ' ')
				directive = false
			}
		case '.':
			if depth == 0 {
				e.set(i, ' ')
			}
		}
	}
	return e.result()
}

func preprocessElixir(input string) string {
	e := sourceEdits{input: input}
	c := scanCursor{input: input}
	var brackets scanBrackets
	for c.pos < len(input) {
		if c.curr() != '~' || !strings.ContainsRune("cCsSwW", rune(c.next())) {
			c.pos++
			continue
		}
		c.pos += 2
		if c.curr() != '(' && c.curr() != '[' && c.curr() != '{' {
			continue
		}
		e.set(c.pos, ' ')
		brackets.push(c.curr())
		for c.pos < len(input) {
			c.pos++
			switch c.curr() {
			case '\\':
				c.pos += 2
			case '(', '[', '{':
				brackets.push(c.curr())
			case ')', ']', '}':
				if brackets.n > 0 {
					brackets.pop(c.curr())
					if brackets.n == 0 {
						e.set(c.pos, ' ')
						goto closed
					}
				}
			}
		}
	closed:
	}
	return e.result()
}

func clojureKeyword(ch byte) bool { return scanAlnum(ch) || strings.IndexByte("!#%*+-./:_", ch) >= 0 }
func preprocessClojure(input string) string {
	input = strings.ReplaceAll(strings.ReplaceAll(input, ":class", "      "), ":className", "          ")
	e := sourceEdits{input: input}
	c := scanCursor{input: input}
	for c.pos < len(input) {
		switch ch := c.curr(); {
		case ch == '"':
			preprocessQuoted(&c, &e, true)
		case ch == ';' && c.next() == ';':
			for c.pos < len(input) && c.curr() != '\n' {
				e.set(c.pos, ' ')
				c.pos++
			}
		case ch == ':':
			e.set(c.pos, ' ')
			c.pos++
			for c.pos < len(input) {
				if c.curr() == '.' {
					if !digit(c.prev()) || !digit(c.next()) {
						e.set(c.pos, ' ')
					}
				} else if !clojureKeyword(c.curr()) {
					e.set(c.pos, ' ')
					break
				}
				c.pos++
			}
		case ch == '\'' && (c.next() == '[' || c.next() == '('):
			e.set(c.pos, ' ')
			c.pos++
			e.set(c.pos, ' ')
			end := byte(']')
			if c.curr() == '(' {
				end = ')'
			}
			for c.pos < len(input) {
				if c.curr() == end {
					e.set(c.pos, ' ')
					break
				}
				if c.curr() == '"' {
					preprocessQuoted(&c, &e, true)
				}
				c.pos++
			}
		case ch == '\'' && !cssSpace(c.next()):
			e.set(c.pos, ' ')
			c.pos++
			for c.pos < len(input) {
				if !clojureKeyword(c.curr()) {
					e.set(c.pos, ' ')
					break
				}
				c.pos++
			}
		default:
			e.set(c.pos, ' ')
		}
		c.pos++
	}
	return e.result()
}

func preprocessPug(input string) string {
	e := sourceEdits{input: input}
	c := scanCursor{input: input}
	var brackets scanBrackets
	attributes, lineStart, tag, expansion := false, true, false, false
	quote, quoteStart, quotesAreText := byte(0), 0, false
	for c.pos < len(input) {
		ch := c.curr()
		if quote != 0 {
			switch {
			case ch == '\\':
				c.pos++
			case ch == quote:
				quote = 0
			case quote != '`' && (ch == '\n' || c.pos+1 == len(input)):
				quote = 0
				quotesAreText = true
				c.move(quoteStart + 1)
				continue
			}
			c.pos++
			continue
		}
		if lineStart && ch != ' ' && ch != '\t' && ch != '\r' && ch != '\n' {
			lineStart = false
			tag = asciiLetter(ch) || ch == '.' || ch == '#' || ch == '+'
		}
		switch {
		case ch == '\n' && !attributes:
			brackets.n = 0
			lineStart = true
			tag = false
			expansion = false
		case scanQuote(ch) && attributes && !quotesAreText:
			quote = ch
			quoteStart = c.pos
		case (ch == ' ' || ch == '\t') && brackets.n == 0 && !expansion:
			tag = false
		case ch == ':' && brackets.n == 0:
			expansion = true
		case ch == '.':
			if brackets.n > 0 {
				c.pos++
				continue
			}
			if preprocessDot(&c) {
				e.set(c.pos, ' ')
			}
		case ch == '(' && brackets.n == 0 && c.prev() != '-' && c.prev() != '/':
			e.set(c.pos, ' ')
			brackets.push(ch)
			attributes = tag
		case ch == '(' || ch == '[' || ch == '{':
			brackets.push(ch)
		case (ch == ')' || ch == ']' || ch == '}') && brackets.n > 0:
			brackets.pop(ch)
			if brackets.n == 0 {
				attributes = false
			}
		}
		if ch != ' ' && ch != '\t' && ch != ':' {
			expansion = false
		}
		if ch == '\n' {
			quotesAreText = false
		}
		c.pos++
	}
	return e.result()
}

func preprocessRust(input string) string {
	input = preprocessClassDirective(input)
	if !strings.Contains(input, "html!") {
		return input
	}
	e := sourceEdits{input: input}
	c := scanCursor{input: input}
	var brackets scanBrackets
	for c.pos < len(input) {
		switch c.curr() {
		case '\\':
			c.pos += 2
			continue
		case '"':
			preprocessQuoted(&c, &e, true)
		case '.':
			if brackets.n > 0 {
				c.pos++
				continue
			}
			if preprocessDot(&c) {
				e.set(c.pos, ' ')
			}
		case '[':
			brackets.push('[')
			if c.prev() != '-' {
				e.set(c.pos, ' ')
			}
		case ']':
			if brackets.n > 0 {
				brackets.pop(']')
			}
		}
		c.pos++
	}
	return e.result()
}

var vueTemplatePattern = regexp.MustCompile(`<template lang=['"]([^"']*)['"]>([\s\S]*)</template>`)

func preprocessVue(input string) string {
	if !utf8.ValidString(input) || !strings.Contains(input, "<template lang=") {
		return input
	}
	result := input
	for _, match := range vueTemplatePattern.FindAllStringSubmatch(input, -1) {
		result = strings.ReplaceAll(result, match[2], Preprocess(match[2], match[1]))
	}
	return result
}

var rubyTemplateStart = regexp.MustCompile(`(?m)[\t\n\v\f\r\x{0085}\p{Z}]*([a-z0-9_-]+)_template[\t\n\v\f\r\x{0085}\p{Z}]*<<[-~]?([A-Z]+)$`)
var rubyTemplateEnd = regexp.MustCompile(`(?m)^[\t\n\v\f\r\x{0085}\p{Z}]*([A-Z]+)`)

func scanPunctuation(ch byte) bool {
	return ch >= '!' && ch <= '/' || ch >= ':' && ch <= '@' || ch >= '[' && ch <= '`' || ch >= '{' && ch <= '~'
}
func rubyBoundary(ch byte, space bool) byte {
	switch ch {
	case '[':
		return ']'
	case '(':
		return ')'
	case '{':
		return '}'
	case '<':
		return '>'
	case ' ':
		if space {
			return '\n'
		}
	}
	if scanPunctuation(ch) {
		return ch
	}
	return 0
}
func rubyLiteralStart(c *scanCursor) bool {
	prev := c.prev()
	return c.curr() == '%' && (c.next() == 'w' || c.next() == 'W') && !scanAlnum(prev) && prev != '_' && prev != ')' && prev != ']' && prev != '}'
}

func preprocessRuby(input string) string {
	e := sourceEdits{input: input}
	c := scanCursor{input: input}
	var brackets scanBrackets
	if utf8.ValidString(input) && strings.Contains(input, "_template") {
		starts := rubyTemplateStart.FindAllStringSubmatchIndex(input, -1)
		ends := rubyTemplateEnd.FindAllStringSubmatchIndex(input, -1)
		for _, start := range starts {
			language, delimiter := input[start[2]:start[3]], input[start[4]:start[5]]
			for _, end := range ends {
				if end[0] < start[1] || input[end[2]:end[3]] != delimiter {
					continue
				}
				e.replace(start[1], end[0], Preprocess(input[start[1]:end[0]], strings.ToLower(language)))
				break
			}
		}
	}
	for c.pos < len(input) {
		if c.curr() == '"' || c.curr() == '\'' {
			preprocessQuoted(&c, &e, false)
			c.pos++
			continue
		}
		if c.curr() == '#' && c.prev() != '%' && c.next() != '{' {
			for c.pos < len(input) && c.curr() != '\n' {
				e.set(c.pos, ' ')
				c.pos++
			}
			c.pos++
			continue
		}
		if c.curr() != '%' || c.next() != 'w' && c.next() != 'W' && c.next() != 'p' {
			c.pos++
			continue
		}
		prev := c.prev()
		if scanAlnum(prev) || prev == '_' || prev == ')' || prev == ']' || prev == '}' {
			c.pos++
			continue
		}
		c.pos += 2
		boundary := rubyBoundary(c.curr(), true)
		if boundary == 0 {
			c.pos++
			continue
		}
		brackets.n = 0
		e.set(c.pos, ' ')
		c.pos++
		for c.pos < len(input) {
			ch := c.curr()
			switch {
			case ch == '\\' && boundary != '\\':
				if c.next() == ' ' {
					e.set(c.pos, ' ')
				}
				c.pos++
			case ch == '[' || ch == '(' || ch == '{' || ch == '<' && boundary == '>':
				brackets.push(ch)
			case (ch == ']' || ch == ')' || ch == '}' || ch == '>' && boundary == '>') && brackets.n > 0:
				if !brackets.pop(ch) {
					c.pos++
				}
			case ch == boundary:
				if boundary != '\n' {
					e.set(c.pos, ' ')
				}
				goto closed
			}
			c.pos++
		}
	closed:
	}
	return e.result()
}

func preprocessEmbeddedRuby(c *scanCursor, e *sourceEdits) bool {
	open := c.at(c.pos + 2)
	close := rubyBoundary(open, false)
	if close == 0 {
		c.pos++
		return false
	}
	for range 3 {
		e.set(c.pos, ' ')
		c.pos++
	}
	depth := 1
	for c.pos < len(c.input) {
		ch := c.curr()
		switch {
		case ch == '\\' && close != '\\':
			if c.next() == ' ' {
				e.set(c.pos, ' ')
			}
			c.pos++
		case ch == open && open != close:
			depth++
		case ch == close:
			depth--
			if depth == 0 {
				e.set(c.pos, ' ')
				return true
			}
		}
		c.pos++
	}
	return true
}

func preprocessSlim(input string) string {
	e := sourceEdits{input: input}
	c := scanCursor{input: input}
	var brackets scanBrackets
	for c.pos < len(input) {
		ch := c.curr()
		switch {
		case ch == '.':
			if brackets.n > 0 {
				c.pos++
				continue
			}
			if preprocessDot(&c) {
				e.set(c.pos, ' ')
			}
		case rubyLiteralStart(&c):
			if !preprocessEmbeddedRuby(&c, &e) {
				continue
			}
		case ch == '[' && brackets.n == 0 && scanAlnum(c.prev()):
			e.set(c.pos, ' ')
			brackets.push(ch)
		case ch == '(' && brackets.n == 0 && c.prev() != '-' && c.prev() != '/':
			e.set(c.pos, ' ')
			brackets.push(ch)
		case ch == '(' || ch == '[' || ch == '{':
			brackets.push(ch)
		case (ch == ')' || ch == ']' || ch == '}') && brackets.n > 0:
			brackets.pop(ch)
		}
		c.pos++
	}
	return e.result()
}

func onlySpace(input string) bool {
	for i := range len(input) {
		if !cssSpace(input[i]) {
			return false
		}
	}
	return true
}
func preprocessHaml(input string) string {
	e := sourceEdits{input: input}
	c := scanCursor{input: input}
	var brackets scanBrackets
	newline := 0
	for c.pos < len(input) {
		ch := c.curr()
		switch {
		case ch == '\\':
			c.pos += 2
			continue
		case ch == '\n':
			newline = c.pos
			c.pos++
			continue
		case (ch == '-' && c.next() == '#' || ch == '/') && onlySpace(input[newline:c.pos]):
			newline = skipHamlBlock(&c, newline)
		case (ch == '-' || ch == '=' || ch == '~') && onlySpace(input[newline:c.pos]):
			start := c.pos + 1
			end := skipHamlBlock(&c, newline)
			newline = end
			e.replace(start, end, preprocessRuby(input[start:end]))
		case ch == '.':
			if brackets.n > 0 {
				c.pos++
				continue
			}
			if preprocessDot(&c) {
				e.set(c.pos, ' ')
			}
		case rubyLiteralStart(&c):
			if !preprocessEmbeddedRuby(&c, &e) {
				continue
			}
		case (ch == '#' || ch == '=') && brackets.n == 0:
			e.set(c.pos, ' ')
		case ch == '(' || ch == '[' || ch == '{':
			if brackets.n == 0 {
				e.set(c.pos, ' ')
			}
			brackets.push(ch)
		case (ch == ')' || ch == ']' || ch == '}') && brackets.n > 0:
			brackets.pop(ch)
			if brackets.n == 0 {
				e.set(c.pos, ' ')
			}
		}
		c.pos++
	}
	return e.result()
}

func skipHamlBlock(c *scanCursor, previousNewline int) int {
	evaluation := c.curr()
	indent := max(0, c.pos-previousNewline-1)
	newline := previousNewline
	for c.pos < len(c.input) && c.curr() != '\n' {
		c.pos++
	}
	if evaluation == '=' && c.prev() != ',' {
		return c.pos
	}
outer:
	for c.pos < len(c.input) {
		switch ch := c.curr(); {
		case ch == '\\':
			c.pos += 2
			continue
		case ch == '\n':
			newline = c.pos
			if evaluation == '=' && c.prev() != ',' {
				break outer
			}
			c.pos++
			continue
		case cssSpace(ch):
			for c.pos < len(c.input) && cssSpace(c.curr()) {
				if c.curr() == '\n' {
					newline = c.pos
					if evaluation == '=' && c.prev() != ',' {
						break outer
					}
				}
				c.pos++
			}
			if max(0, c.pos-newline-1) < indent {
				break outer
			}
		default:
			break outer
		}
		c.pos++
	}
	if newline == previousNewline {
		return c.pos
	}
	c.move(newline)
	return newline
}
