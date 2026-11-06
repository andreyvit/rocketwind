package rocketwind

import (
	"slices"
	"strings"
)

// OptimizeOptions selects compact or readable output. From names the input in
// diagnostics and source maps. Optimization is separate from utility compilation.
type OptimizeOptions struct {
	Minify bool
	From   string
}

// Optimize lowers CSS nesting, combines adjacent rules with the same selector,
// and removes comments other than license comments. Minify also removes optional
// whitespace. Values retain their precision, custom properties and fallbacks.
func Optimize(css string, options OptimizeOptions) (string, error) {
	output, _, err := optimizeCSS(css, options, nil, false)
	return output, err
}

// OptimizeWithSourceMap also maps the output back to its input. Pass a compiler
// source map to retain the original stylesheet locations through optimization,
// or nil to map to css itself. Neither the input map nor its slices are changed.
func OptimizeWithSourceMap(css string, options OptimizeOptions, sourceMap *SourceMap) (string, *SourceMap, error) {
	return optimizeCSS(css, options, sourceMap, true)
}

func optimizeCSS(css string, options OptimizeOptions, sourceMap *SourceMap, track bool) (string, *SourceMap, error) {
	var original []decodedMapping
	if sourceMap != nil {
		var err error
		original, err = sourceMap.decode()
		if err != nil {
			return "", nil, err
		}
		slices.SortStableFunc(original, compareMappings)
	}
	filename := options.From
	if filename == "" {
		filename = "input.css"
	}
	parser := cssParser{input: css, filename: filename, source: &cssSource{file: filename, code: css}, optimize: true}
	nodes, err := parser.parse()
	if err != nil {
		return "", nil, err
	}
	groupOpaqueRules(nodes)
	nodes = flattenRulesMode(nodes, "", true)
	writer := optimizedCSSWriter{compact: options.Minify, track: track}
	writer.out.Grow(len(css))
	writer.write(nodes, 0)
	output := writer.out.String()
	if !track {
		return output, nil, nil
	}
	mapped := createSourceMap(output, writer.mappings)
	if sourceMap != nil {
		for i := range mapped.Mappings {
			mapping := &mapped.Mappings[i]
			position := mapping.Original.sourcePosition
			index, found := slices.BinarySearchFunc(original, position, func(a decodedMapping, b sourcePosition) int {
				return comparePosition(a.Generated, b)
			})
			if !found {
				index--
			}
			mapping.Original, mapping.Name = nil, nil
			if index >= 0 && original[index].Generated.Line == position.Line {
				mapping.Original, mapping.Name = original[index].Original, original[index].Name
			}
		}
	}
	result := mapped.encode()
	if sourceMap != nil {
		result.File = sourceMap.File
	}
	return output, result, nil
}

type optimizedCSSWriter struct {
	out      strings.Builder
	mappings []cssMapping
	compact  bool
	track    bool
}

func (w *optimizedCSSWriter) write(nodes []*Node, depth int) {
	for index, node := range nodes {
		if !w.compact {
			w.out.WriteString(strings.Repeat("  ", depth))
		}
		start := w.out.Len()
		switch node.Kind {
		case "comment":
			w.out.WriteString("/*")
			w.out.WriteString(node.Value)
			w.out.WriteString("*/")
		case "declaration":
			w.out.WriteString(node.Name)
			w.out.WriteByte(':')
			if !w.compact {
				w.out.WriteByte(' ')
			}
			value := node.Value
			if w.compact {
				value = compactCSSValue(value)
			}
			w.out.WriteString(value)
			if node.Important {
				if !w.compact {
					w.out.WriteByte(' ')
				}
				w.out.WriteString("!important")
			}
			if !w.compact || index+1 < len(nodes) || depth == 0 {
				w.out.WriteByte(';')
			}
		default:
			if node.Kind == "rule" && w.compact {
				w.out.WriteString(selectorCSSMode(parseSelectorMode(node.Name, true), true))
			} else {
				w.out.WriteString(node.Name)
			}
			if node.Kind == "at-rule" && node.Value != "" {
				w.out.WriteByte(' ')
				w.out.WriteString(node.Value)
			}
			block := node.Kind == "rule" || len(node.Nodes) > 0 || node.source != nil && node.End < len(node.source.code) && node.source.code[node.End] == '{'
			if !block {
				w.out.WriteByte(';')
			} else {
				if !w.compact {
					w.out.WriteByte(' ')
				}
				w.out.WriteByte('{')
				w.mapNode(node, start, w.out.Len())
				if !w.compact {
					w.out.WriteByte('\n')
				}
				compact := w.compact
				if node.Kind == "at-rule" && !knownCSSBlock(node.Name) {
					// Future CSS mixins and other opaque at-rules can interpret
					// their contents themselves. Keep full declaration syntax.
					w.compact = false
				}
				w.write(node.Nodes, depth+1)
				w.compact = compact
				if !w.compact {
					w.out.WriteString(strings.Repeat("  ", depth))
				}
				w.out.WriteByte('}')
				start = -1
			}
		}
		if start >= 0 {
			w.mapNode(node, start, w.out.Len())
		}
		if !w.compact {
			w.out.WriteByte('\n')
			if index+1 < len(nodes) && (node.Kind == "rule" || node.Kind == "at-rule" && len(node.Nodes) > 0) {
				w.out.WriteByte('\n')
			}
		}
	}
}

func (w *optimizedCSSWriter) mapNode(node *Node, start, end int) {
	if w.track {
		w.mappings = append(w.mappings, cssMapping{location(node), start, end})
	}
}

// Preserve token boundaries, strings, CSS escapes, math operators, and the
// whitespace-only custom property value. Commas and parentheses only refer to
// punctuation tokens; an escaped comma or parenthesis is part of an identifier.
func compactCSSValue(value string) string {
	var output strings.Builder
	output.Grow(len(value))
	space, afterSeparator := false, false
	for i := 0; i < len(value); {
		c := value[i]
		if cssWhitespace(c) {
			space = true
			i++
			continue
		}
		if space && output.Len() > 0 && !afterSeparator && c != ')' && c != ',' {
			output.WriteByte(' ')
		}
		space = false
		start := i
		afterSeparator = c == '(' || c == ','
		switch c {
		case '\\':
			i = cssEscapeEnd(value, i)
		case '\'', '"':
			quote := c
			for i++; i < len(value); {
				if value[i] == '\\' {
					i = min(i+2, len(value))
				} else if value[i] == quote {
					i++
					break
				} else {
					i++
				}
			}
		default:
			i++
		}
		output.WriteString(value[start:i])
	}
	if output.Len() == 0 && space {
		return " "
	}
	return output.String()
}

func cssWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func knownCSSBlock(name string) bool {
	return hoistable(name) || containsWord("@keyframes @-webkit-keyframes @font-face @property @font-feature-values @font-palette-values @counter-style @color-profile @position-try", name)
}

// Tailwind's optimizer attaches unknown at-rules to the parent style rule,
// before the nested rules it lowers. This includes future CSS @apply mixins.
// Their declaration blocks remain opaque; regular declarations keep order.
func groupOpaqueRules(nodes []*Node) {
	for _, node := range nodes {
		groupOpaqueRules(node.Nodes)
		if node.Kind != "rule" {
			continue
		}
		first := -1
		var opaque []*Node
		for i, child := range node.Nodes {
			if first < 0 && (child.Kind == "rule" || child.Kind == "at-rule" && knownCSSBlock(child.Name)) {
				first = i
			}
			if first >= 0 && child.Kind == "at-rule" && !knownCSSBlock(child.Name) {
				opaque = append(opaque, child)
			}
		}
		if len(opaque) == 0 {
			continue
		}
		result := make([]*Node, 0, len(node.Nodes))
		result = append(result, node.Nodes[:first]...)
		result = append(result, opaque...)
		for _, child := range node.Nodes[first:] {
			if child.Kind != "at-rule" || knownCSSBlock(child.Name) {
				result = append(result, child)
			}
		}
		node.Nodes = result
	}
}
