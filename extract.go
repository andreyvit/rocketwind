package rocketwind

import (
	"cmp"
	"slices"
)

// Extracted is a candidate or CSS variable found in source text. Start and End
// are byte offsets, with End exclusive. Value shares the input's storage.
type Extracted struct {
	Kind       string // "candidate" or "variable"
	Value      string
	Start, End int
}

// Extract finds Tailwind candidates and CSS variables in arbitrary source text.
// Like Tailwind, it recognizes plausible tokens without consulting a theme or
// evaluating code. The compiler discards tokens that do not name utilities.
// Variables come first; candidates follow in source order. Duplicates remain.
func Extract(input string) []Extracted { return AppendExtracted(nil, input) }

// AppendExtracted appends source tokens, reusing the caller's output buffer.
func AppendExtracted(dst []Extracted, input string) []Extracted {
	c := scanCursor{input: input}
	for c.pos < len(input) {
		if !cssSpace(c.curr()) {
			if span, ok := scanCSSVariable(&c); ok {
				dst = append(dst, Extracted{"variable", input[span.start:span.end], span.start, span.end})
			}
		}
		c.pos++
	}
	firstCandidate := len(dst)
	c.pos = 0
	var machine scanCandidateMachine
	for c.pos < len(input) {
		if cssSpace(c.curr()) {
			c.pos++
			continue
		}
		before := c.pos
		if span, ok := machine.next(&c); ok {
			dst = append(dst, Extracted{"candidate", input[span.start:span.end], span.start, span.end})
			dst = scanSubCandidates(dst, input, before, span.start)
		} else {
			dst = scanSubCandidates(dst, input, before, min(c.pos, len(input)))
		}
		c.pos++
	}
	slices.SortFunc(dst[firstCandidate:], func(a, b Extracted) int {
		if n := cmp.Compare(a.Start, b.Start); n != 0 {
			return n
		}
		return cmp.Compare(b.End, a.End)
	})
	end := -1
	write := firstCandidate
	for _, token := range dst[firstCandidate:] {
		if token.End > end {
			dst[write] = token
			write++
			end = token.End
		}
	}
	clear(dst[write:])
	return dst[:write]
}

// Candidates returns sorted, unique candidates and variable references for
// Compiler.Build. Dynamic source expressions must contain complete class names.
func Candidates(input string) []string {
	tokens := Extract(input)
	values := make([]string, len(tokens))
	for i, token := range tokens {
		values[i] = token.Value
	}
	slices.Sort(values)
	return slices.Compact(values)
}

// CSSVariables extracts var(...) references from CSS, excluding declarations.
func CSSVariables(input string) []Extracted {
	var result []Extracted
	c := scanCursor{input: input}
	for c.pos < len(input) {
		if !cssSpace(c.curr()) {
			if span, ok := scanCSSVariable(&c); ok && span.start >= 4 && input[span.start-4:span.start] == "var(" {
				result = append(result, Extracted{"variable", input[span.start:span.end], span.start, span.end})
			}
		}
		c.pos++
	}
	return result
}

// The extraction machines follow Tailwind CSS v4.3.4's Rust extractor.
// MIT license: testdata/upstream/LICENSE.
type scanSpan struct{ start, end int }
type scanCursor struct {
	input string
	pos   int
}

func (c *scanCursor) at(pos int) byte {
	if pos >= 0 && pos < len(c.input) {
		return c.input[pos]
	}
	return 0
}
func (c *scanCursor) curr() byte                         { return c.at(c.pos) }
func (c *scanCursor) next() byte                         { return c.at(c.pos + 1) }
func (c *scanCursor) prev() byte                         { return c.at(c.pos - 1) }
func (c *scanCursor) move(pos int)                       { c.pos = min(pos, len(c.input)) }
func scanDone(start int, c *scanCursor) (scanSpan, bool) { return scanSpan{start, c.pos + 1}, true }
func scanLower(ch byte) bool                             { return ch >= 'a' && ch <= 'z' }
func scanUpper(ch byte) bool                             { return ch >= 'A' && ch <= 'Z' }
func scanQuote(ch byte) bool                             { return ch == '\'' || ch == '"' || ch == '`' }
func scanAlnum(ch byte) bool                             { return asciiLetter(ch) || digit(ch) }
func scanCommon(ch byte) bool                            { return ch == 0 || cssSpace(ch) || scanQuote(ch) }
func scanBefore(ch byte) bool                            { return scanCommon(ch) || ch == '.' || ch == '}' || ch == '>' }
func scanAfter(ch byte) bool {
	return scanCommon(ch) || ch == ']' || ch == '{' || ch == '=' || ch == '\\' || ch == '<'
}
func scanVariableChar(ch byte) bool { return scanAlnum(ch) || ch == '_' || ch == '-' || ch >= 128 }

func scanSubCandidates(dst []Extracted, input string, start, end int) []Extracted {
	for i := start; i < end; i++ {
		if input[i] != '[' {
			continue
		}
		c := scanCursor{input: input, pos: i + 1}
		var machine scanCandidateMachine
		for c.pos < end {
			if span, ok := machine.next(&c); ok {
				dst = append(dst, Extracted{"candidate", input[span.start:span.end], span.start, span.end})
			}
			c.pos++
		}
	}
	return dst
}

func scanString(c *scanCursor) (scanSpan, bool) {
	if !scanQuote(c.curr()) {
		return scanSpan{}, false
	}
	start, quote := c.pos, c.curr()
	c.pos++
	for c.pos < len(c.input) {
		switch ch := c.curr(); {
		case ch == '\\':
			if cssSpace(c.next()) {
				return scanSpan{}, false
			}
			c.pos++
		case ch == quote:
			return scanDone(start, c)
		case cssSpace(ch):
			return scanSpan{}, false
		}
		c.pos++
	}
	return scanSpan{}, false
}

func scanCSSVariable(c *scanCursor) (scanSpan, bool) {
	if c.curr() != '-' || c.next() != '-' {
		return scanSpan{}, false
	}
	start := c.pos
	c.pos += 2
	for c.pos < len(c.input) {
		switch ch := c.curr(); {
		case scanVariableChar(ch):
			if scanVariableChar(c.next()) || c.next() == '\\' {
				c.pos++
			} else {
				return scanDone(start, c)
			}
		case ch == '\\':
			if cssSpace(c.next()) || c.next() == 0 {
				return scanSpan{}, false
			}
			c.pos += 2
		default:
			return scanSpan{}, false
		}
	}
	return scanSpan{}, false
}

const (
	scanValueBody = iota
	scanFallbackBody
	scanPropertyBody
)

func scanBody(c *scanCursor, start, mode int) (scanSpan, bool) {
	var brackets [32]byte
	depth := 0
	valueStart := c.pos
	for c.pos < len(c.input) {
		ch := c.curr()
		switch {
		case ch == '\\':
			if cssSpace(c.next()) {
				if mode == scanValueBody {
					c.pos += 2
				}
				return scanSpan{}, false
			}
			c.pos += 2
		case ch == '(' || ch == '[' || ch == '{':
			if depth == len(brackets) {
				return scanSpan{}, false
			}
			close := ch + 2
			if ch == '(' {
				close = ')'
			}
			brackets[depth] = close
			depth++
			c.pos++
		case (ch == ')' || ch == ']' || ch == '}') && depth > 0:
			depth--
			if brackets[depth] != ch {
				return scanSpan{}, false
			}
			c.pos++
		case ch == ']' && mode != scanFallbackBody && start+1 != c.pos || ch == ')' && mode == scanFallbackBody:
			return scanDone(start, c)
		case scanQuote(ch):
			if _, ok := scanString(c); !ok {
				return scanSpan{}, false
			}
			c.pos++
			if mode == scanPropertyBody {
				valueStart = c.pos
			}
		case cssSpace(ch) || ch == '$' && c.next() == '{' || ch == ':' && depth == 0 && mode != scanValueBody || ch == '!' && depth == 0 && mode == scanPropertyBody || ch == '/' && mode == scanPropertyBody && c.pos == valueStart:
			return scanSpan{}, false
		default:
			c.pos++
		}
	}
	return scanSpan{}, false
}

func scanArbitraryValue(c *scanCursor) (scanSpan, bool) {
	if c.curr() != '[' {
		return scanSpan{}, false
	}
	start := c.pos
	c.pos++
	return scanBody(c, start, scanValueBody)
}

func scanArbitraryVariable(c *scanCursor) (scanSpan, bool) {
	if c.curr() != '(' || c.next() != '-' && !scanLower(c.next()) {
		return scanSpan{}, false
	}
	start := c.pos
	c.pos++
	if scanLower(c.curr()) {
		for c.pos < len(c.input) && (scanLower(c.curr()) || c.curr() == '-') {
			c.pos++
		}
		if c.curr() != ':' || c.next() != '-' {
			return scanSpan{}, false
		}
		c.pos++
	}
	if _, ok := scanCSSVariable(c); !ok {
		return scanSpan{}, false
	}
	if c.next() == ',' {
		c.pos += 2
		return scanBody(c, start, scanFallbackBody)
	}
	c.pos++
	if c.curr() != ')' {
		return scanSpan{}, false
	}
	return scanDone(start, c)
}

func scanArbitraryProperty(c *scanCursor) (scanSpan, bool) {
	if c.curr() != '[' {
		return scanSpan{}, false
	}
	start := c.pos
	c.pos++
	for c.pos < len(c.input) {
		switch ch := c.curr(); {
		case ch == '-' && c.next() == '-':
			if _, ok := scanCSSVariable(c); !ok || c.next() != ':' {
				return scanSpan{}, false
			}
			c.pos += 2
			return scanBody(c, start, scanPropertyBody)
		case ch == '-' || scanLower(ch):
			c.pos++
		case ch == ':' && c.pos > start+1:
			c.pos++
			return scanBody(c, start, scanPropertyBody)
		default:
			return scanSpan{}, false
		}
	}
	return scanSpan{}, false
}

func scanModifier(c *scanCursor) (scanSpan, bool) {
	if c.curr() != '/' {
		return scanSpan{}, false
	}
	start := c.pos
	c.pos++
	if c.curr() == '[' {
		if _, ok := scanArbitraryValue(c); ok {
			return scanDone(start, c)
		}
		return scanSpan{}, false
	}
	if c.curr() == '(' {
		if _, ok := scanArbitraryVariable(c); ok {
			return scanDone(start, c)
		}
		return scanSpan{}, false
	}
	if !scanAlnum(c.curr()) {
		return scanSpan{}, false
	}
	for {
		if ch := c.next(); !scanAlnum(ch) && ch != '-' && ch != '_' && ch != '.' {
			return scanDone(start, c)
		}
		c.pos++
	}
}

func scanNamedUtility(c *scanCursor) (scanSpan, bool) {
	start := c.pos
	ch, next := c.curr(), c.next()
	if scanLower(ch) && scanCommon(next) {
		return scanDone(start, c)
	}
	if !scanLower(ch) && ch != '@' && !(ch == '-' && scanLower(next)) {
		return scanSpan{}, false
	}
	c.pos++
	for c.pos < len(c.input) {
		ch, next = c.curr(), c.next()
		switch {
		case asciiLetter(ch):
			if scanAfter(next) || next == ':' || next == '/' || next == '!' {
				return scanDone(start, c)
			}
			c.pos++
		case ch == '-':
			switch {
			case next == '[':
				c.pos++
				if _, ok := scanArbitraryValue(c); ok {
					return scanDone(start, c)
				}
				return scanSpan{}, false
			case next == '(':
				c.pos++
				if _, ok := scanArbitraryVariable(c); ok {
					return scanDone(start, c)
				}
				return scanSpan{}, false
			case scanAlnum(next) || next == '-':
				c.pos++
			default:
				return scanSpan{}, false
			}
		case ch == '_':
			if scanAlnum(next) || next == '_' {
				c.pos++
			} else if scanAfter(next) || next == ':' || next == '/' || next == '!' {
				return scanDone(start, c)
			} else {
				return scanSpan{}, false
			}
		case ch == '.':
			if !digit(c.prev()) || !digit(next) {
				return scanSpan{}, false
			}
			c.pos++
		case digit(ch):
			prev := c.prev()
			if !scanAlnum(prev) && prev != '-' && prev != '_' && prev != '.' {
				return scanSpan{}, false
			}
			if !scanAlnum(next) && next != '.' && next != '%' && next != '_' && next != '-' {
				return scanDone(start, c)
			}
			c.pos++
		case ch == '%':
			if !digit(c.prev()) {
				return scanSpan{}, false
			}
			return scanDone(start, c)
		default:
			return scanSpan{}, false
		}
	}
	return scanSpan{}, false
}

type scanNamedVariantMachine struct{ start int }

func (m *scanNamedVariantMachine) next(c *scanCursor) (scanSpan, bool) {
	ch := c.curr()
	if (scanLower(ch) || ch == '*') && c.next() == ':' {
		c.pos++
		return scanDone(m.start, c)
	}
	if !scanLower(ch) && ch != '*' && !digit(ch) && ch != '@' {
		return scanSpan{}, false
	}
	m.start = c.pos
	c.pos++
	for c.pos < len(c.input) {
		ch, next := c.curr(), c.next()
		switch {
		case ch == '-':
			if next == '[' || next == '(' {
				c.pos++
				var ok bool
				if next == '[' {
					_, ok = scanArbitraryValue(c)
				} else {
					_, ok = scanArbitraryVariable(c)
				}
				if !ok {
					return scanSpan{}, false
				}
				return m.arbitraryEnd(c)
			}
			if !scanAlnum(next) && next != '-' && next != '_' {
				return scanSpan{}, false
			}
			c.pos++
		case ch == '[':
			if _, ok := scanArbitraryValue(c); !ok {
				return scanSpan{}, false
			}
			return m.arbitraryEnd(c)
		case ch == '_':
			if !scanAlnum(next) && next != '-' && next != '_' {
				return scanSpan{}, false
			}
			c.pos++
		case scanAlnum(ch) || ch == '*':
			c.pos++
		case ch == '/':
			return m.modifier(c)
		case ch == ':':
			return scanDone(m.start, c)
		case ch == '.':
			if !digit(c.prev()) || !digit(next) {
				return scanSpan{}, false
			}
			c.pos++
		default:
			return scanSpan{}, false
		}
	}
	return scanSpan{}, false
}

func (m *scanNamedVariantMachine) arbitraryEnd(c *scanCursor) (scanSpan, bool) {
	switch c.next() {
	case '/':
		c.pos++
		return m.modifier(c)
	case ':':
		c.pos++
		return scanDone(m.start, c)
	}
	return scanSpan{}, false
}

func (m *scanNamedVariantMachine) modifier(c *scanCursor) (scanSpan, bool) {
	if _, ok := scanModifier(c); !ok || c.next() != ':' {
		return scanSpan{}, false
	}
	c.pos++
	return scanDone(m.start, c)
}

func scanUtility(c *scanCursor) (scanSpan, bool) {
	start := c.pos
	important := c.curr() == '!'
	if important {
		c.pos++
	}
	var ok bool
	if c.curr() == '[' {
		_, ok = scanArbitraryProperty(c)
	} else {
		_, ok = scanNamedUtility(c)
	}
	if !ok {
		return scanSpan{}, false
	}
	if c.next() == '/' {
		c.pos++
		if _, ok := scanModifier(c); !ok || c.next() == '/' {
			return scanSpan{}, false
		}
	}
	if c.next() == '!' {
		c.pos++
		if important {
			return scanSpan{}, false
		}
	}
	return scanDone(start, c)
}

type scanVariantMachine struct{ named scanNamedVariantMachine }

func (m *scanVariantMachine) next(c *scanCursor) (scanSpan, bool) {
	start := c.pos
	if c.curr() == '[' {
		if _, ok := scanArbitraryValue(c); !ok || c.next() != ':' {
			return scanSpan{}, false
		}
		c.pos++
	} else if _, ok := m.named.next(c); !ok {
		return scanSpan{}, false
	}
	return scanDone(start, c)
}

type scanCandidateMachine struct {
	start   int
	hasLast bool
	variant scanVariantMachine
}

func (m *scanCandidateMachine) reset() { m.start = 0; m.hasLast = false }
func (m *scanCandidateMachine) next(c *scanCursor) (scanSpan, bool) {
	for c.pos < len(c.input) {
		ch := c.curr()
		if cssSpace(ch) || ch == ':' || scanQuote(ch) {
			m.reset()
			c.pos++
			continue
		}
		if ch == '<' || scanUpper(ch) {
			i := c.pos
			for i < len(c.input) && !scanBefore(c.input[i]) {
				i++
			}
			if i == len(c.input) {
				m.reset()
				return scanSpan{}, false
			}
			m.reset()
			c.move(i + 1)
			continue
		}
		vc, uc := *c, *c
		variant, vok := m.variant.next(&vc)
		utility, uok := scanUtility(&uc)
		switch {
		case !vok && uok:
			c.move(uc.pos + 1)
			span := utility
			if m.hasLast {
				// Both child scanners start at c.pos. After a variant, c.pos
				// is exactly one byte past it; every intervening skip resets
				// hasLast. No separate adjacency check or end offset is needed.
				span.start = m.start
			}
			m.reset()
			if !scanBefore(c.at(span.start-1)) || !scanAfter(c.at(span.end)) {
				return scanSpan{}, false
			}
			return span, true
		case vok && uok:
			c.move(vc.pos + 1)
			if !m.hasLast {
				if after := c.at(utility.end + 1); cssSpace(after) || after == 0 {
					c.move(uc.pos + 2)
					m.reset()
					return utility, true
				}
				m.start = variant.start
			}
			m.hasLast = true
		case vok:
			c.move(vc.pos + 1)
			if !m.hasLast {
				m.start = variant.start
			}
			m.hasLast = true
		default:
			if vc.pos > c.pos || uc.pos > c.pos {
				c.move(max(vc.pos, uc.pos))
			}
			m.reset()
			c.pos++
		}
	}
	return scanSpan{}, false
}
