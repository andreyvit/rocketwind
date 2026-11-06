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
