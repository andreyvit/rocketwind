package rocketwind

import "strings"

func (b *buildContext) evaluateNodes(nodes []*Node) []*Node {
	var result []*Node
	for i, n := range nodes {
		value, children := n.Value, n.Nodes
		if strings.Contains(value, "__ROCKETWIND_DEFAULT_") {
			for key, fallback := range defaultValues {
				resolved := b.themeValue(key)
				if strings.HasPrefix(key, "--default-transition-") {
					resolved = b.theme(key)
				}
				if resolved == "" {
					resolved = fallback
				}
				value = strings.ReplaceAll(value, "__ROCKETWIND_DEFAULT_"+strings.ReplaceAll(key[2:], "-", "_")+"__", resolved)
			}
		}
		if strings.Contains(value, "--spacing(") || strings.Contains(value, "--alpha(") || strings.Contains(value, "theme(") {
			values := parseValue(value)
			if !b.evaluateFunctions(values, n.Kind == "at-rule", 0) {
				return nil
			}
			value = valueCSS(values)
		}
		if n.Nodes != nil {
			children = b.evaluateNodes(n.Nodes)
			if len(children) == 0 && len(n.Nodes) > 0 {
				return nil
			}
		}
		changed := value != n.Value || !sameNodeSlice(children, n.Nodes)
		if changed || result != nil {
			if result == nil {
				result = make([]*Node, i, len(nodes))
				copy(result, nodes[:i])
			}
			if changed {
				r := *n
				r.Value, r.Nodes = value, children
				result = append(result, &r)
			} else {
				result = append(result, n)
			}
		}
	}
	if result == nil {
		return nodes
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
					if child.kind == 'f' && child.value == "var" && len(child.nodes) > 0 && child.nodes[0].kind == 'w' {
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
								if len(parsed[0].nodes) == 0 || parsed[0].nodes[0].kind != 'w' {
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
	for i, n := range nodes {
		if n.Kind == "declaration" && !inside && strings.Contains(n.Value, "color-mix(") {
			if fallback, needed := b.colorFallback(n.Value); needed {
				if result == nil {
					result = make([]*Node, i, len(nodes)+1)
					copy(result, nodes[:i])
				}
				r := *n
				r.Value = fallback
				result = append(result, &r, withLocation(&Node{Kind: "at-rule", Name: "@supports", Value: "(color: color-mix(in lab, red, red))", Nodes: []*Node{n}}, location(n)))
				continue
			}
		}
		children := n.Nodes
		if len(children) > 0 {
			children = b.polyfillColors(children, inside || n.Name == "@keyframes" || n.Name == "@supports" && strings.Contains(n.Value, "color-mix("))
		}
		changed := !sameNodeSlice(children, n.Nodes)
		if changed || result != nil {
			if result == nil {
				result = make([]*Node, i, len(nodes))
				copy(result, nodes[:i])
			}
			if changed {
				r := *n
				r.Nodes = children
				result = append(result, &r)
			} else {
				result = append(result, n)
			}
		}
	}
	if result == nil {
		return nodes
	}
	return result
}

// Recursive transforms retain the original slice when no node changes.
func sameNodeSlice(a, b []*Node) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}
