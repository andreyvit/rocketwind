package rocketwind

import (
	"fmt"
	"strings"
)

// ClassSet contains unescaped HTML class names.
type ClassSet map[string]struct{}

func (s ClassSet) Contains(class string) bool { _, found := s[class]; return found }

// DefinedClasses collects class selectors from already compiled stylesheets.
// Comments, declaration values, attributes and negative/relational selector
// arguments do not define classes. CSS escapes are decoded before comparison.
// Imports are not loaded; pass their compiled stylesheets separately.
func DefinedClasses(stylesheets ...string) (ClassSet, error) {
	classes := make(ClassSet)
	var visit func([]*Node)
	visit = func(nodes []*Node) {
		for _, node := range nodes {
			if node.Kind == "rule" {
				collectSelectorClasses(node.Name, classes)
			}
			visit(node.Nodes)
		}
	}
	for i, stylesheet := range stylesheets {
		nodes, err := ParseCSS(stylesheet, "")
		if err != nil {
			return nil, fmt.Errorf("existing stylesheet %d: %w", i+1, err)
		}
		visit(nodes)
	}
	return classes, nil
}

func collectSelectorClasses(selector string, classes ClassSet) {
	for i := 0; i < len(selector); {
		switch selector[i] {
		case '.':
			start := i + 1
			i = cssIdentifierEnd(selector, start)
			if i > start {
				classes[Unescape(selector[start:i])] = struct{}{}
			}
		case ':':
			start := i
			i++
			if i < len(selector) && selector[i] == ':' {
				i++
			}
			i = cssIdentifierEnd(selector, i)
			if i < len(selector) && selector[i] == '(' {
				name := Unescape(selector[start:i])
				end := cssBracketEnd(selector, i)
				if containsWord(":is :where :-moz-any :-webkit-any :nth-child :nth-last-child ::slotted", name) {
					innerEnd := end
					if innerEnd > i+1 && selector[innerEnd-1] == ')' {
						innerEnd--
					}
					collectSelectorClasses(selector[i+1:innerEnd], classes)
				}
				i = end
			}
		case '[', '(', '\'', '"':
			i = cssBracketEnd(selector, i)
		case '/':
			if i+1 < len(selector) && selector[i+1] == '*' {
				if end := strings.Index(selector[i+2:], "*/"); end >= 0 {
					i += end + 4
				} else {
					return
				}
			} else {
				i++
			}
		case '\\':
			i = cssEscapeEnd(selector, i)
		default:
			i++
		}
	}
}

func cssIdentifierEnd(input string, start int) int {
	i := start
	for i < len(input) {
		c := input[i]
		if c == '\\' {
			i = cssEscapeEnd(input, i)
		} else if c >= 128 || asciiLetter(c) || digit(c) || c == '_' || c == '-' {
			i++
		} else {
			break
		}
	}
	return i
}

func cssEscapeEnd(input string, start int) int {
	i := start + 1
	if i == len(input) {
		return i
	}
	end := min(i+6, len(input))
	for i < end && (digit(input[i]) || input[i] >= 'a' && input[i] <= 'f' || input[i] >= 'A' && input[i] <= 'F') {
		i++
	}
	if i == start+1 {
		return i + 1
	}
	if i < len(input) && cssSpace(input[i]) {
		if input[i] == '\r' && i+1 < len(input) && input[i+1] == '\n' {
			i++
		}
		i++
	}
	return i
}

// cssBracketEnd returns the offset after a bracketed or quoted expression.
func cssBracketEnd(input string, start int) int {
	open := input[start]
	close := open
	if open == '[' {
		close = ']'
	} else if open == '(' {
		close = ')'
	}
	for i := start + 1; i < len(input); {
		switch input[i] {
		case '\\':
			i = cssEscapeEnd(input, i)
		case close:
			return i + 1
		default:
			if open != '\'' && open != '"' && strings.ContainsRune("[('\"", rune(input[i])) {
				i = cssBracketEnd(input, i)
			} else {
				i++
			}
		}
	}
	return len(input)
}
