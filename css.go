// Parser behavior ported from Tailwind CSS v4.3.4 under the MIT license.
// See testdata/upstream/LICENSE.
package rocketwind

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Node is a CSS rule, at-rule, declaration or preserved license comment.
// Name holds a selector, at-rule name or property; Value holds parameters or a
// declaration/comment value. Source spans use UTF-8 byte offsets.
type Node struct {
	Kind       string
	Name       string
	Value      string
	Important  bool
	Nodes      []*Node
	Start, End int
}

// ParseCSS parses Tailwind's CSS input dialect. An optional filename enables
// source locations in diagnostics and nodes.
func ParseCSS(input string, filename string) ([]*Node, error) {
	p := cssParser{input: input, filename: filename}
	return p.parse()
}

type cssParser struct {
	input, filename string
}

func (p *cssParser) fail(at int, message string) error {
	if p.filename == "" {
		return fmt.Errorf("%s", message)
	}
	line, col := 1, 1
	for _, r := range p.input[:at] {
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
			if r > 0xffff {
				col++
			}
		}
	}
	return fmt.Errorf("%s:%d:%d: %s", p.filename, line, col, message)
}

func (p *cssParser) span(n *Node, start, end int) *Node {
	if p.filename != "" {
		n.Start = start
		n.End = end
	}
	return n
}

func (p *cssParser) parse() ([]*Node, error) {
	input := p.input
	ast := make([]*Node, 0)
	var licenses []*Node
	var stack []*Node
	var parent *Node
	var local [256]byte
	buf := local[:0]
	var brackets []byte
	start := 0
	add := func(n *Node) {
		if parent == nil {
			ast = append(ast, n)
		} else {
			parent.Nodes = append(parent.Nodes, n)
		}
	}
	last := func() byte {
		if len(brackets) > 0 {
			return brackets[len(brackets)-1]
		}
		return 0
	}
	i := 0
	if strings.HasPrefix(input, "\ufeff") {
		i = 3
	}
	for ; i < len(input); i++ {
		c := input[i]
		if c == '\r' && i+1 < len(input) && input[i+1] == '\n' {
			continue
		}
		switch {
		case c == '\\':
			if len(buf) == 0 {
				start = i
			}
			end := min(i+2, len(input))
			buf = append(buf, input[i:end]...)
			i++
		case c == '/' && i+1 < len(input) && input[i+1] == '*':
			end := strings.Index(input[i+2:], "*/")
			if end < 0 {
				return nil, p.fail(i, "Unterminated comment")
			}
			end += i + 4
			if i+2 < len(input) && input[i+2] == '!' {
				licenses = append(licenses, p.span(&Node{Kind: "comment", Value: input[i+2 : end-2]}, i, end))
			}
			i = end - 1
		case c == '\'' || c == '"':
			end, err := p.quoted(i)
			if err != nil {
				return nil, err
			}
			buf = append(buf, input[i:end+1]...)
			i = end
		case (c == ' ' || c == '\n' || c == '\t') && i+1 < len(input) && (input[i+1] == ' ' || input[i+1] == '\n' || input[i+1] == '\t' || input[i+1] == '\r' && i+2 < len(input) && input[i+2] == '\n'):
			continue
		case c == '\n':
			if len(buf) > 0 && buf[len(buf)-1] != ' ' && buf[len(buf)-1] != '\t' && buf[len(buf)-1] != '\n' {
				buf = append(buf, ' ')
			}
		case c == '-' && i+1 < len(input) && input[i+1] == '-' && len(buf) == 0:
			n, end, err := p.custom(i)
			if err != nil {
				return nil, err
			}
			add(n)
			i = end
		case c == ';' && len(buf) > 0 && buf[0] == '@':
			add(p.span(cssRule(strings.TrimSpace(string(buf))), start, i))
			buf = buf[:0]
		case c == ';' && last() != ')':
			if len(buf) == 0 {
				continue
			}
			n := cssDeclaration(string(buf), len(buf))
			if n == nil {
				return nil, p.fail(start, "Invalid declaration: `"+strings.TrimSpace(string(buf))+"`")
			}
			add(p.span(n, start, i))
			buf = buf[:0]
		case c == '{' && last() != ')':
			brackets = append(brackets, '}')
			n := p.span(cssRule(strings.TrimSpace(string(buf))), start, i)
			if parent != nil {
				parent.Nodes = append(parent.Nodes, n)
			}
			stack = append(stack, parent)
			parent = n
			buf = buf[:0]
		case c == '}' && last() != ')':
			if len(brackets) == 0 {
				return nil, p.fail(i, "Missing opening {")
			}
			brackets = brackets[:len(brackets)-1]
			if len(buf) > 0 {
				var n *Node
				if buf[0] == '@' {
					n = cssRule(strings.TrimSpace(string(buf)))
				} else {
					n = cssDeclaration(string(buf), len(buf))
				}
				if n == nil {
					return nil, p.fail(start, "Invalid declaration: `"+strings.TrimSpace(string(buf))+"`")
				}
				add(p.span(n, start, i))
			}
			grand := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if grand == nil {
				ast = append(ast, parent)
			}
			parent = grand
			buf = buf[:0]
		case c == '(':
			brackets = append(brackets, ')')
			buf = append(buf, c)
		case c == ')':
			if last() != ')' {
				return nil, p.fail(i, "Missing opening (")
			}
			brackets = brackets[:len(brackets)-1]
			buf = append(buf, c)
		default:
			if len(buf) == 0 {
				if c == ' ' || c == '\t' {
					continue
				}
				start = i
			}
			buf = append(buf, c)
		}
	}
	if len(buf) > 0 && buf[0] == '@' {
		ast = append(ast, p.span(cssRule(strings.TrimSpace(string(buf))), start, len(input)))
	} else if len(buf) > 0 && len(brackets) == 0 {
		if n := cssDeclaration(string(buf), len(buf)); n != nil {
			add(p.span(n, start, len(input)))
		}
	}
	if len(brackets) > 0 && parent != nil {
		name := parent.Name
		if parent.Kind == "at-rule" {
			name += " " + parent.Value
		}
		return nil, p.fail(parent.Start, "Missing closing } at "+name)
	}
	if len(licenses) > 0 {
		ast = append(licenses, ast...)
	}
	return ast, nil
}

func (p *cssParser) quoted(start int) (int, error) {
	s := p.input
	for i := start + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case s[start]:
			return i, nil
		case ';':
			if i+1 < len(s) && (s[i+1] == '\n' || s[i+1] == '\r' && i+2 < len(s) && s[i+2] == '\n') {
				return 0, p.fail(start, "Unterminated string: "+s[start:i+1]+s[start:start+1])
			}
		case '\n', '\r':
			if s[i] == '\n' || i+1 < len(s) && s[i+1] == '\n' {
				return 0, p.fail(start, "Unterminated string: "+s[start:i]+s[start:start+1])
			}
		}
	}
	return start, nil
}

func (p *cssParser) custom(start int) (*Node, int, error) {
	s := p.input
	var local [32]byte
	stack := local[:0]
	end := len(s)
	valueEnd := 0
	next := len(s)
	for i := start + 2; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			i++
		case c == '\'' || c == '"':
			j, err := p.quoted(i)
			if err != nil {
				return nil, 0, err
			}
			i = j
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return nil, 0, p.fail(i, "Unterminated comment")
			}
			i += j + 3
			continue
		case c == ';' && len(stack) == 0:
			end = i
			next = i
			goto done
		case c == '}' && len(stack) == 0:
			end = i
			next = i - 1
			goto done
		case c == '(':
			stack = append(stack, ')')
		case c == '[':
			stack = append(stack, ']')
		case c == '{':
			stack = append(stack, '}')
		case c == ')' || c == ']' || c == '}':
			if len(stack) > 0 && stack[len(stack)-1] == c {
				stack = stack[:len(stack)-1]
			}
		}
		if !cssSpace(c) {
			valueEnd = min(i+1, len(s)) - start
		}
	}
done:
	n := cssDeclaration(s[start:end], valueEnd)
	if n == nil {
		return nil, 0, p.fail(start, "Invalid custom property, expected a value")
	}
	return p.span(n, start, next), next, nil
}

func cssRule(s string) *Node {
	n := &Node{Kind: "rule", Name: s, Nodes: make([]*Node, 0)}
	if strings.HasPrefix(s, "@") {
		n.Kind = "at-rule"
		for i := 1; i < len(s); i++ {
			if cssSpace(s[i]) || s[i] == '(' {
				n.Name = s[:i]
				n.Value = strings.TrimSpace(s[i:])
				break
			}
		}
	}
	return n
}

func cssDeclaration(s string, valueEnd int) *Node {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return nil
	}
	n := &Node{Kind: "declaration", Name: strings.TrimSpace(s[:i])}
	value := trimCSSValue(s[i+1 : max(i+1, valueEnd)])
	bang := len(value) - len("!important")
	if strings.HasSuffix(value, "!important") && !isEscaped(value, bang) {
		n.Value = trimCSSValue(value[:bang])
		n.Important = true
	} else {
		n.Value = trimCSSValue(s[i+1:])
	}
	return n
}

func trimCSSValue(s string) string {
	s = strings.TrimLeftFunc(s, func(r rune) bool { return r < utf8.RuneSelf && cssSpace(byte(r)) })
	trimmed := strings.TrimRightFunc(s, func(r rune) bool { return r < utf8.RuneSelf && cssSpace(byte(r)) })
	if len(trimmed) < len(s) && isEscaped(s, len(trimmed)) {
		return s[:len(trimmed)+1]
	}
	return trimmed
}

func isEscaped(s string, i int) bool {
	n := 0
	for i > 0 && s[i-1] == '\\' {
		n++
		i--
	}
	return n%2 == 1
}
