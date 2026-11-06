// Selector syntax follows Tailwind CSS v4.3.4. See testdata/upstream/LICENSE.
package rocketwind

import "strings"

type selectorNode struct {
	Kind  string          `json:"kind"`
	Value string          `json:"value,omitempty"`
	Nodes []*selectorNode `json:"nodes,omitempty"`
}

func parseSelector(input string) []*selectorNode {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	ast := []*selectorNode{}
	target := &ast
	hasCombinator := false
	var list *selectorNode
	type context struct {
		target        *[]*selectorNode
		list          *selectorNode
		hasCombinator bool
	}
	var stack []context
	buffer := ""
	current := func(nodes []*selectorNode) *selectorNode {
		if len(nodes) == 1 {
			return nodes[0]
		}
		kind := "compound"
		if hasCombinator {
			kind = "complex"
		}
		return &selectorNode{Kind: kind, Nodes: nodes}
	}
	appendNode := func(node *selectorNode) {
		n := len(*target)
		if n > 0 {
			last := (*target)[n-1]
			if last.Kind == "compound" {
				last.Nodes = append(last.Nodes, node)
				return
			}
			if last.Kind != "list" && last.Kind != "combinator" {
				(*target)[n-1] = &selectorNode{Kind: "compound", Nodes: []*selectorNode{last, node}}
				return
			}
		}
		*target = append(*target, node)
	}
	flush := func() {
		if buffer != "" {
			appendNode(&selectorNode{Kind: "selector", Value: buffer})
			buffer = ""
		}
	}
	for i := 0; i < len(input); i++ {
		c := input[i]
		switch c {
		case ',':
			flush()
			for i+1 < len(input) && (input[i+1] == '\n' || input[i+1] == ' ' || input[i+1] == '\t') {
				i++
			}
			if list != nil {
				list.Nodes = append(list.Nodes, current(*target))
			} else {
				item := current(*target)
				list = &selectorNode{Kind: "list", Nodes: []*selectorNode{item}}
				*target = []*selectorNode{list}
			}
			next := []*selectorNode{}
			target = &next
			hasCombinator = false
		case '>', '\n', ' ', '+', '\t', '~':
			flush()
			start, end := i, i+1
			for end < len(input) && strings.ContainsRune(">\n +\t~", rune(input[end])) {
				end++
			}
			i = end - 1
			value := strings.TrimSpace(input[start:end])
			if value == "" && (len(*target) == 0 || end >= len(input) || input[end] == ',') {
				continue
			}
			if value == "" {
				value = " "
			}
			*target = append(*target, &selectorNode{Kind: "combinator", Value: value})
			hasCombinator = true
		case '(':
			node := &selectorNode{Kind: "function", Value: buffer, Nodes: []*selectorNode{}}
			buffer = ""
			if !containsWord(":not :where :has :is :-moz-any :-webkit-any", node.Value) {
				start, nesting := i+1, 0
				for j := i + 1; j < len(input); j++ {
					if input[j] == '(' {
						nesting++
					} else if input[j] == ')' {
						if nesting == 0 {
							i = j
							break
						}
						nesting--
					}
				}
				end := i
				contents := ""
				if start <= end {
					contents = input[start:end]
				}
				if (node.Value == ":nth-child" || node.Value == ":nth-last-child") && strings.Contains(contents, "of ") {
					idx := strings.Index(contents, "of ")
					node.Nodes = append(node.Nodes, &selectorNode{Kind: "value", Value: contents[:idx+3]})
					node.Nodes = append(node.Nodes, parseSelector(contents[idx+3:])...)
				} else {
					node.Nodes = append(node.Nodes, &selectorNode{Kind: "value", Value: contents})
				}
				appendNode(node)
				continue
			}
			appendNode(node)
			stack = append(stack, context{target, list, hasCombinator})
			target = &node.Nodes
			hasCombinator = false
			list = nil
		case ')':
			flush()
			if list != nil {
				list.Nodes = append(list.Nodes, current(*target))
			} else if hasCombinator {
				nodes := *target
				*target = []*selectorNode{{Kind: "complex", Nodes: nodes}}
			}
			target = &ast
			list = nil
			hasCombinator = false
			if len(stack) > 0 {
				ctx := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				target, list, hasCombinator = ctx.target, ctx.list, ctx.hasCombinator
			}
		case '.', ':', '#':
			if c == ':' && buffer == ":" {
				buffer += ":"
				continue
			}
			flush()
			buffer = input[i : i+1]
		case '[':
			flush()
			start, nesting := i, 0
			for j := i + 1; j < len(input); j++ {
				if input[j] == '[' {
					nesting++
				} else if input[j] == ']' {
					if nesting == 0 {
						i = j
						break
					}
					nesting--
				}
			}
			appendNode(&selectorNode{Kind: "selector", Value: input[start : i+1]})
		case '\'', '"':
			start := i
			for j := i + 1; j < len(input); j++ {
				if input[j] == '\\' {
					j++
				} else if input[j] == c {
					i = j
					break
				}
			}
			buffer += input[start : i+1]
		case '&', '*':
			flush()
			appendNode(&selectorNode{Kind: "selector", Value: input[i : i+1]})
		case '\\':
			buffer += input[i : i+1]
			i++
			if i < len(input) {
				buffer += input[i : i+1]
			} else {
				buffer += "undefined"
			}
		default:
			buffer += input[i : i+1]
		}
	}
	flush()
	if list != nil {
		list.Nodes = append(list.Nodes, current(*target))
	} else if hasCombinator {
		nodes := *target
		*target = []*selectorNode{{Kind: "complex", Nodes: nodes}}
	}
	return ast
}
func selectorCSS(nodes []*selectorNode) string { return selectorCSSMode(nodes, false) }
func selectorCSSMode(nodes []*selectorNode, minify bool) string {
	var out strings.Builder
	var write func([]*selectorNode)
	write = func(nodes []*selectorNode) {
		for _, n := range nodes {
			switch n.Kind {
			case "selector", "value":
				out.WriteString(n.Value)
			case "combinator":
				if n.Value == " " || minify {
					out.WriteString(n.Value)
				} else {
					out.WriteByte(' ')
					out.WriteString(n.Value)
					out.WriteByte(' ')
				}
			case "function":
				out.WriteString(n.Value)
				out.WriteByte('(')
				write(n.Nodes)
				out.WriteByte(')')
			case "list":
				for i, child := range n.Nodes {
					if i > 0 {
						if minify {
							out.WriteByte(',')
						} else {
							out.WriteString(", ")
						}
					}
					write([]*selectorNode{child})
				}
			default:
				write(n.Nodes)
			}
		}
	}
	write(nodes)
	return out.String()
}
func normalizeSelector(s string) string {
	nodes := parseSelector(s)
	var simplify func([]*selectorNode)
	simplify = func(nodes []*selectorNode) {
		for _, n := range nodes {
			simplify(n.Nodes)
			if n.Kind == "compound" && len(n.Nodes) > 1 {
				var children []*selectorNode
				for _, child := range n.Nodes {
					if child.Kind == "selector" && child.Value == "*" {
						continue
					}
					children = append(children, child)
				}
				if len(children) > 0 {
					n.Nodes = children
				}
			}
		}
	}
	simplify(nodes)
	return selectorCSS(nodes)
}
