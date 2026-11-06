package rocketwind

import "strings"

// Print returns the normalized spelling of a parsed candidate.
func (c *Compiler) Print(candidate Candidate) string {
	var parts []string
	if c.config.Prefix != "" {
		parts = append(parts, c.config.Prefix)
	}
	for i := len(candidate.Variants) - 1; i >= 0; i-- {
		parts = append(parts, printVariantMode(candidate.Variants[i], c.config.Compatibility == Tailwind3))
	}
	base := candidate.Root
	if candidate.Kind == "arbitrary" {
		base = "[" + candidate.Property + ":" + printArbitraryValue(candidate.Value.Value) + "]"
	} else if candidate.Kind == "functional" && candidate.Value != nil {
		if candidate.Value.Kind == "named" {
			base += "-" + candidate.Value.Value
		} else if c.config.Compatibility == Tailwind3 {
			hint := candidate.Value.DataType
			if hint != "" {
				hint += ":"
			}
			base += "-[" + hint + printArbitraryValue(candidate.Value.Value) + "]"
		} else {
			base += "-" + printArbitrary(candidate.Value.Value, candidate.Value.DataType)
		}
	}
	if candidate.Kind != "static" {
		base += printModifierMode(candidate.Modifier, c.config.Compatibility == Tailwind3)
	}
	if candidate.Important {
		if c.config.Compatibility != Tailwind3 {
			base += "!"
		}
	}
	if c.config.Compatibility == Tailwind3 {
		if candidate.Kind != "arbitrary" {
			if strings.HasPrefix(base, "-") {
				base = "-" + c.v3Prefix + base[1:]
			} else {
				base = c.v3Prefix + base
			}
		}
		if candidate.Important {
			base = "!" + base
		}
	}
	return strings.Join(append(parts, base), ":")
}

func printModifier(modifier *CandidateModifier) string {
	return printModifierMode(modifier, false)
}

func printModifierMode(modifier *CandidateModifier, legacy bool) string {
	if modifier == nil {
		return ""
	}
	if modifier.Kind == "arbitrary" {
		if legacy {
			return "/[" + printArbitraryValue(modifier.Value) + "]"
		}
		return "/" + printArbitrary(modifier.Value, "")
	}
	return "/" + modifier.Value
}

func printArbitrary(value, hint string) string {
	open, close := "[", "]"
	nodes := parseValue(value)
	if len(nodes) == 1 && nodes[0].kind == 'f' && nodes[0].value == "var" {
		value = value[4 : len(value)-1]
		open, close = "(", ")"
	}
	if hint != "" {
		hint += ":"
	}
	return open + hint + printArbitraryValue(value) + close
}

func printVariant(v *CandidateVariant) string {
	return printVariantMode(v, false)
}

func printVariantMode(v *CandidateVariant, legacy bool) string {
	if v.Kind == "static" {
		return v.Root
	}
	if v.Kind == "arbitrary" {
		value := v.Selector
		nodes := parseValue(value)
		if len(nodes) == 3 && nodes[0].kind == 'w' && nodes[0].value == "&" && nodes[1].kind == 's' && nodes[1].value == ":" && nodes[2].kind == 'f' && nodes[2].value == "is" {
			value = valueCSS(nodes[2].nodes)
		}
		return "[" + printArbitraryValue(value) + "]"
	}
	base := v.Root
	if v.Kind == "compound" {
		base += "-" + printVariantMode(v.Variant, legacy)
	} else if v.Value != nil {
		if v.Root != "@" {
			base += "-"
		}
		if v.Value.Kind == "named" {
			base += v.Value.Value
		} else if legacy {
			base += "[" + printArbitraryValue(v.Value.Value) + "]"
		} else {
			base += printArbitrary(v.Value.Value, "")
		}
	}
	return base + printModifierMode(v.Modifier, legacy)
}

func printArbitraryValue(value string) string {
	nodes := parseValue(value)
	symbol := func(s string) bool { return len(s) == 1 && strings.Contains("~>+-*/", s) }
	var simplify func([]*valueNode) []*valueNode
	simplify = func(nodes []*valueNode) []*valueNode {
		drop := make(map[int]bool)
		for i, n := range nodes {
			if n.kind == 'w' && symbol(n.value) {
				if i > 0 && i+1 < len(nodes) && nodes[i-1].kind == 's' && nodes[i-1].value == " " && nodes[i+1].kind == 's' && nodes[i+1].value == " " {
					if i > 1 && symbol(nodes[i-2].value) || i+2 < len(nodes) && symbol(nodes[i+2].value) {
						continue
					}
					drop[i-1], drop[i+1] = true, true
				}
			} else if n.kind == 's' && n.value != "" && strings.TrimSpace(n.value) == "" {
				if i == 0 || i == len(nodes)-1 {
					drop[i] = true
				}
			} else if n.kind == 's' && strings.TrimSpace(n.value) == "," {
				n.value = ","
			}
			if n.kind == 'f' {
				if strings.HasPrefix(n.value, "--") && i > 0 && !(nodes[i-1].kind == 's' && nodes[i-1].value == ",") && !(i > 1 && !symbol(nodes[i-2].value)) {
					nodes[i] = &valueNode{kind: 'f', nodes: []*valueNode{n}}
					continue
				}
				n.nodes = simplify(n.nodes)
			}
		}
		var result []*valueNode
		for i, n := range nodes {
			if !drop[i] {
				result = append(result, n)
			}
		}
		return result
	}
	nodes = simplify(nodes)
	var escape func([]*valueNode)
	escape = func(nodes []*valueNode) {
		for _, n := range nodes {
			name := n.value
			if n.kind != 'w' || !strings.HasPrefix(name, "--") {
				n.value = strings.ReplaceAll(strings.ReplaceAll(name, "_", "\\_"), " ", "_")
			}
			if n.kind == 'f' && !(name == "url" || strings.HasSuffix(name, "_url")) {
				escape(n.nodes)
			}
		}
	}
	escape(nodes)
	return valueCSS(nodes)
}
