package rocketwind

import "strings"

func (b *buildContext) evaluateNodes(nodes []*Node) []*Node {
	result := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		r := *n
		if strings.Contains(r.Value, "__ROCKETWIND_DEFAULT_") {
			for key, fallback := range defaultValues {
				value := b.themeValue(key)
				if strings.HasPrefix(key, "--default-transition-") {
					value = b.theme(key)
				}
				if value == "" {
					value = fallback
				}
				r.Value = strings.ReplaceAll(r.Value, "__ROCKETWIND_DEFAULT_"+strings.ReplaceAll(key[2:], "-", "_")+"__", value)
			}
		}
		if strings.Contains(r.Value, "--spacing(") || strings.Contains(r.Value, "--alpha(") || strings.Contains(r.Value, "theme(") {
			values := parseValue(r.Value)
			if !b.evaluateFunctions(values, r.Kind == "at-rule", 0) {
				return nil
			}
			r.Value = valueCSS(values)
		}
		if n.Nodes != nil {
			r.Nodes = b.evaluateNodes(n.Nodes)
			if len(r.Nodes) == 0 && len(n.Nodes) > 0 {
				return nil
			}
		}
		result = append(result, &r)
	}
	return result
}

func valueCSS(nodes []*valueNode) string {
	var out strings.Builder
	writeValue(&out, nodes)
	return out.String()
}

func (b *buildContext) colorFallback(value string) (string, bool) {
	nodes := parseValue(value)
	required := false
	var walk func([]*valueNode)
	walk = func(nodes []*valueNode) {
		for _, n := range nodes {
			if n.kind != 'f' {
				continue
			}
			if n.value != "color-mix" {
				walk(n.nodes)
				continue
			}
			unresolved, current := false, false
			var inline func([]*valueNode)
			inline = func(children []*valueNode) {
				for _, child := range children {
					if child.kind == 'w' && strings.EqualFold(child.value, "currentcolor") {
						required, current = true, true
					}
					if child.kind == 'f' && child.value == "var" && len(child.nodes) > 0 {
						required = true
						v := child
						seen := make(map[string]bool)
						for {
							key := v.nodes[0].value
							if seen[key] {
								unresolved = true
								break
							}
							seen[key] = true
							resolved := b.themeValue(key)
							if resolved == "" {
								unresolved = true
								break
							}
							if strings.EqualFold(resolved, "currentcolor") {
								current = true
								break
							}
							if strings.HasPrefix(resolved, "var(") {
								parsed := parseValue(resolved)
								if len(parsed) == 0 || len(parsed[0].nodes) == 0 {
									unresolved = true
									break
								}
								v = parsed[0]
								continue
							}
							child.kind, child.value, child.nodes = 'w', resolved, nil
							break
						}
					} else {
						inline(child.nodes)
					}
				}
			}
			inline(n.nodes)
			if unresolved || current {
				for i, child := range n.nodes {
					if child.kind == 's' && strings.Contains(child.value, ",") && i+1 < len(n.nodes) {
						*n = *n.nodes[i+1]
						walk([]*valueNode{n})
						break
					}
				}
			} else if required && len(n.nodes) > 2 {
				if c := n.nodes[2]; c.kind == 'w' && (c.value == "oklab" || c.value == "oklch" || c.value == "lab" || c.value == "lch") {
					c.value = "srgb"
				}
			}
			walk(n.nodes)
		}
	}
	walk(nodes)
	return valueCSS(nodes), required
}

func (b *buildContext) polyfillColors(nodes []*Node, inside bool) []*Node {
	var result []*Node
	for _, n := range nodes {
		r := *n
		if n.Kind == "declaration" && !inside && strings.Contains(n.Value, "color-mix(") {
			if fallback, needed := b.colorFallback(n.Value); needed {
				r.Value = fallback
				result = append(result, &r, &Node{Kind: "at-rule", Name: "@supports", Value: "(color: color-mix(in lab, red, red))", Nodes: []*Node{n}})
				continue
			}
		}
		if n.Nodes != nil {
			r.Nodes = b.polyfillColors(n.Nodes, inside || n.Name == "@keyframes" || n.Name == "@supports" && strings.Contains(n.Value, "color-mix("))
		}
		result = append(result, &r)
	}
	return result
}

// Stop flattening at the first declaration, preserving useful browser devtools rules.
func flattenRules(input []*Node, parent string) []*Node {
	var result []*Node
	for _, n := range input {
		if n.Kind == "at-root" || n.Kind == "declaration" && n.Name == "--tw-sort" {
			continue
		}
		r := *n
		if n.Kind == "rule" {
			if n.Name == "&" {
				for _, child := range flattenRules(n.Nodes, parent) {
					result = appendFlattened(result, child)
				}
				continue
			}
			if parent != "" {
				r.Name = nestSelector(n.Name, parent)
			} else if strings.Contains(n.Name, "&") {
				r.Name = replaceTopLevelNesting(n.Name, ":scope")
			}
			if deadSelector(r.Name) {
				continue
			}
			hasDeclarations := false
			for _, child := range n.Nodes {
				if child.Kind == "declaration" && child.Name != "--tw-sort" {
					hasDeclarations = true
					break
				}
			}
			if !hasDeclarations || hasNode(n.Nodes, "@scope") {
				for _, child := range flattenRules(n.Nodes, r.Name) {
					result = appendFlattened(result, child)
				}
				continue
			}
		} else if n.Kind == "at-rule" && n.Name == "@scope" {
			innerParent := parent
			if strings.Contains(n.Value, "&") || !n.variant && parent != "" {
				params := parseValue(n.Value)
				end := false
				for _, p := range params {
					if p.kind == 'w' && p.value == "to" {
						end = true
					}
					if p.kind != 'f' || p.value != "" {
						continue
					}
					selector := valueCSS(p.nodes)
					if !end && !n.variant && parent != "" {
						selector = nestSelector(selector, parent)
						innerParent = ":where(:scope)"
					} else if strings.Contains(selector, "&") {
						replacement := parent
						if replacement == "" {
							replacement = ":scope"
						}
						if end {
							replacement = ":where(:scope)"
						}
						selector = nestSelector(selector, replacement)
						if !end && parent != "" {
							innerParent = ":where(:scope)"
						}
					}
					p.nodes = []*valueNode{{kind: 'w', value: selector}}
				}
				r.Value = valueCSS(params)
			}
			if innerParent == "" {
				innerParent = ":where(:scope)"
			}
			r.Nodes = flattenRules(n.Nodes, innerParent)
		} else if n.Kind == "at-rule" && len(n.Nodes) > 0 && (n.Name == "@media" || n.Name == "@supports" || n.Name == "@layer" || n.Name == "@container" || n.Name == "@starting-style") {
			r.Nodes = flattenRules(n.Nodes, parent)
		} else if parent != "" {
			r = Node{Kind: "rule", Name: parent, Nodes: []*Node{n}}
		}
		result = appendFlattened(result, &r)
	}
	return result
}

func appendFlattened(nodes []*Node, n *Node) []*Node {
	if len(nodes) > 0 {
		last := nodes[len(nodes)-1]
		if n.Kind == last.Kind && n.Name == last.Name && n.Value == last.Value && (n.Kind == "rule" || n.Name == "@media" || n.Name == "@supports" || n.Name == "@container" || n.Name == "@scope") {
			last.Nodes = append(last.Nodes, n.Nodes...)
			seen := map[string]bool{}
			for i := len(last.Nodes) - 1; i >= 0; i-- {
				d := last.Nodes[i]
				if d.Kind != "declaration" {
					continue
				}
				key := d.Name + "\x00" + d.Value
				if d.Important {
					key += "\x01"
				}
				if seen[key] {
					last.Nodes = append(last.Nodes[:i], last.Nodes[i+1:]...)
				} else {
					seen[key] = true
				}
			}
			return nodes
		}
	}
	return append(nodes, n)
}

func nestSelector(selector, parent string) string {
	var selectors []string
	for s := range Segments(selector, ',') {
		s = strings.TrimSpace(s)
		if s == "&" {
			selectors = append(selectors, parent)
			continue
		}
		replacement := parent
		for part := range Segments(parent, ',') {
			if part != parent {
				replacement = ":is(" + parent + ")"
				break
			}
		}
		ast := parseSelector(s)
		changed := replaceNesting(ast, replacement)
		if changed {
			s = selectorCSS(ast)
		} else {
			s = replacement + " " + s
		}
		selectors = append(selectors, s)
	}
	return normalizeSelector(strings.Join(selectors, ", "))
}

func replaceNesting(nodes []*selectorNode, parent string) bool {
	changed := false
	for _, n := range nodes {
		if n.Kind == "selector" && n.Value == "&" {
			n.Value = parent
			changed = true
		}
		if replaceNesting(n.Nodes, parent) {
			changed = true
		}
	}
	return changed
}

func replaceTopLevelNesting(selector, parent string) string {
	nodes := parseSelector(selector)
	replaceNesting(nodes, parent)
	return selectorCSS(nodes)
}

func cleanRules(nodes []*Node) []*Node {
	var result []*Node
	for _, n := range nodes {
		r := *n
		if n.Nodes != nil {
			r.Nodes = cleanRules(n.Nodes)
			if len(r.Nodes) == 0 {
				continue
			}
		}
		result = appendFlattened(result, &r)
	}
	seen := make(map[string]bool)
	for i := len(result) - 1; i >= 0; i-- {
		n := result[i]
		if n.Kind != "declaration" {
			continue
		}
		key := n.Name + "\x00" + n.Value
		if n.Important {
			key += "\x01"
		}
		if seen[key] {
			result = append(result[:i], result[i+1:]...)
		} else {
			seen[key] = true
		}
	}
	return result
}

func deadSelector(s string) bool {
	for _, pseudo := range []string{"::selection", "::backdrop", "::before", "::after", "::placeholder", "::first-letter", "::first-line", "::file-selector-button"} {
		if _, tail, ok := strings.Cut(s, pseudo); ok && (strings.HasPrefix(tail, " >") || strings.HasPrefix(tail, " +") || strings.HasPrefix(tail, " ~") || strings.HasPrefix(tail, " ")) {
			return true
		}
	}
	return false
}
