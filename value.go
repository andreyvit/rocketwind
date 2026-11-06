// Value syntax follows Tailwind CSS v4.3.4. See testdata/upstream/LICENSE.
package rocketwind

import "strings"

// DecodeArbitraryValue decodes Tailwind underscores and formats CSS math.
func DecodeArbitraryValue(s string) string {
	if !strings.ContainsRune(s, '(') {
		return underscores(s, false)
	}
	nodes := parseValue(s)
	decodeValueNodes(nodes)
	var b strings.Builder
	b.Grow(len(s))
	writeValue(&b, nodes)
	return formatMath(b.String())
}

type valueNode struct {
	kind  byte
	value string
	nodes []*valueNode
}

func parseValue(s string) []*valueNode {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	ast := make([]*valueNode, 0)
	var stack []*valueNode
	var parent *valueNode
	var buf []byte
	add := func(n *valueNode) {
		if parent == nil {
			ast = append(ast, n)
		} else {
			parent.nodes = append(parent.nodes, n)
		}
	}
	flush := func() {
		if len(buf) > 0 {
			add(&valueNode{kind: 'w', value: string(buf)})
			buf = buf[:0]
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			buf = append(buf, c)
			i++
			if i < len(s) {
				buf = append(buf, s[i])
			} else {
				buf = append(buf, "undefined"...)
			}
		case c == '/':
			flush()
			add(&valueNode{kind: 'w', value: "/"})
		case valueSeparator(c):
			flush()
			start := i
			for i+1 < len(s) && valueSeparator(s[i+1]) {
				i++
			}
			add(&valueNode{kind: 's', value: s[start : i+1]})
		case c == '\'' || c == '"':
			start := i
			for j := i + 1; j < len(s); j++ {
				if s[j] == '\\' {
					j++
				} else if s[j] == c {
					i = j
					break
				}
			}
			buf = append(buf, s[start:i+1]...)
		case c == '(':
			n := &valueNode{kind: 'f', value: string(buf)}
			buf = buf[:0]
			add(n)
			stack = append(stack, n)
			parent = n
		case c == ')':
			if len(stack) > 0 {
				if len(buf) > 0 {
					parent.nodes = append(parent.nodes, &valueNode{kind: 'w', value: string(buf)})
				}
				stack = stack[:len(stack)-1]
			}
			buf = buf[:0]
			parent = nil
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
		default:
			buf = append(buf, c)
		}
	}
	if len(buf) > 0 {
		ast = append(ast, &valueNode{kind: 'w', value: string(buf)})
	}
	return ast
}

func valueSeparator(c byte) bool {
	return c == ':' || c == ',' || c == '=' || c == '>' || c == '<' || c == '\n' || c == ' ' || c == '\t'
}

func writeValue(b *strings.Builder, nodes []*valueNode) {
	for _, n := range nodes {
		b.WriteString(n.value)
		if n.kind == 'f' {
			b.WriteByte('(')
			writeValue(b, n.nodes)
			b.WriteByte(')')
		}
	}
}

func decodeValueNodes(nodes []*valueNode) {
	for _, n := range nodes {
		if n.kind != 'f' {
			n.value = underscores(n.value, false)
			continue
		}
		name := n.value
		n.value = underscores(name, false)
		if name == "url" || strings.HasSuffix(name, "_url") {
			continue
		}
		children := n.nodes
		if name == "var" || strings.HasSuffix(name, "_var") || name == "theme" || strings.HasSuffix(name, "_theme") {
			if len(children) > 0 && children[0].kind == 'w' {
				children[0].value = underscores(children[0].value, true)
				children = children[1:]
			}
		}
		decodeValueNodes(children)
	}
}

func underscores(s string, keep bool) string {
	if !strings.ContainsRune(s, '_') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '_' {
			b.WriteByte('_')
			i++
		} else if s[i] == '_' && !keep {
			b.WriteByte(' ')
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func mathFunction(s string) bool {
	switch s {
	case "calc", "min", "max", "clamp", "mod", "rem", "sin", "cos", "tan", "asin", "acos", "atan", "atan2", "pow", "sqrt", "hypot", "log", "exp", "round":
		return true
	}
	return false
}

func formatMath(s string) string {
	var local [32]bool
	stack := local[:0]
	out := make([]byte, 0, len(s))
	valuePos, lastValuePos := -1, -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if digit(c) {
			valuePos = i
		} else if valuePos >= 0 && (c == '%' || asciiLetter(c)) {
			valuePos = i
		} else {
			lastValuePos = valuePos
			valuePos = -1
		}
		active := len(stack) > 0 && stack[len(stack)-1]
		switch {
		case c == '(':
			out = append(out, c)
			start := i
			for j := i - 1; j >= 0; j-- {
				if digit(s[j]) || s[j] >= 'a' && s[j] <= 'z' {
					start = j
				} else {
					break
				}
			}
			name := s[start:i]
			stack = append(stack, mathFunction(name) || active && name == "")
		case c == ')':
			out = append(out, c)
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case c == ',' && active:
			out = append(out, ',', ' ')
		case c == ' ' && active && len(out) > 0 && out[len(out)-1] == ' ':
		case operator(c) && active:
			j := len(out)
			for j > 0 && cssSpace(out[j-1]) {
				j--
			}
			var prev, prevPrev, next byte
			if j > 0 {
				prev = out[j-1]
			}
			if j > 1 {
				prevPrev = out[j-2]
			}
			if i+1 < len(s) {
				next = s[i+1]
			}
			switch {
			case (prev == 'e' || prev == 'E') && digit(prevPrev), operator(prev), prev == '(' || prev == ',':
				out = append(out, c)
			case i > 0 && s[i-1] == ' ':
				out = append(out, c, ' ')
			case digit(prev) || digit(next) || prev == ')' || next == '(' || operator(next) || lastValuePos >= 0 && lastValuePos == i-1:
				out = append(out, ' ', c, ' ')
			default:
				out = append(out, c)
			}
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

func digit(c byte) bool       { return c >= '0' && c <= '9' }
func asciiLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func operator(c byte) bool    { return c == '+' || c == '-' || c == '*' || c == '/' }
