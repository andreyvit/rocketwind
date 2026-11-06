package rocketwind

import (
	"regexp"
	"slices"
	"strings"
)

func cloneCandidateVariant(v *CandidateVariant) *CandidateVariant {
	c := *v
	if v.Value != nil {
		value := *v.Value
		c.Value = &value
	}
	if v.Modifier != nil {
		modifier := *v.Modifier
		c.Modifier = &modifier
	}
	if v.Variant != nil {
		c.Variant = cloneCandidateVariant(v.Variant)
	}
	return &c
}

func (k *Canonicalizer) parseVariant(name string) *CandidateVariant {
	return parseVariantSyntax(name, k.compiler.variantDefinition)
}

var canonicalSupports = regexp.MustCompile(`(?i)^[a-z-][a-z0-9-]*$`)

func (k *Canonicalizer) canonicalVariant(input *CandidateVariant) []*CandidateVariant {
	v := cloneCandidateVariant(input)
	for n := v; n != nil; n = n.Variant {
		if n.Kind == "arbitrary" {
			n.Selector, _ = k.convertTheme(n.Selector, false, true)
		} else if n.Kind == "functional" && n.Value != nil && n.Value.Kind == "arbitrary" {
			n.Value.Value, _ = k.convertTheme(n.Value.Value, false, true)
		}
		if n.Kind == "functional" && n.Value != nil && n.Value.Kind == "arbitrary" {
			value := n.Value.Value
			if n.Root == "data" && !strings.Contains(value, "=") || n.Root == "supports" && canonicalSupports.MatchString(value) {
				n.Value.Kind = "named"
			} else if n.Root == "aria" && (strings.HasSuffix(value, "=true") || strings.HasSuffix(value, `="true"`) || strings.HasSuffix(value, "='true'")) {
				key, _, _ := strings.Cut(value, "=")
				if key != "" && !strings.ContainsAny(key[len(key)-1:], "~|^$*") {
					n.Value = &CandidateValue{Kind: "named", Value: key}
				}
			}
		}
	}
	result := k.modernizeVariant(v)
	k.indexVariants()
	for _, v := range result {
		for n := v; n != nil; n = n.Variant {
			if n.Kind == "compound" {
				continue
			}
			name := printVariant(n)
			sig := k.variantSignature(name)
			if sig == "" {
				continue
			}
			if found := k.variantLookup[sig]; len(found) > 0 && !slices.Contains(found, name) {
				if replacement := k.parseVariant(found[0]); replacement != nil {
					*n = *replacement
				}
			}
		}
	}
	return result
}

func (k *Canonicalizer) variantSignature(name string) string {
	if sig, ok := k.variantSignatures[name]; ok {
		return sig
	}
	ast := k.compileApply(k.compiler, name+":flex")
	if ast == nil {
		k.variantSignatures[name] = ""
		return ""
	}
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Kind == "at-rule" {
				n.Value = strings.ReplaceAll(constantFoldDeclaration(n.Value, k.options.Rem, true), " ", "")
			} else if n.Kind == "rule" {
				changed := false
				var selectors func([]*selectorNode) []*selectorNode
				selectors = func(nodes []*selectorNode) []*selectorNode {
					var result []*selectorNode
					for _, n := range nodes {
						if n.Kind == "list" || n.Kind == "combinator" {
							changed = true
						} else if n.Kind == "function" && n.Value == ":is" {
							if len(n.Nodes) == 1 {
								changed = true
								result = append(result, selectors(n.Nodes)...)
								continue
							}
						} else if n.Kind == "function" && strings.HasPrefix(n.Value, ":") && len(n.Nodes) > 0 && n.Nodes[0].Kind == "selector" && strings.HasPrefix(n.Nodes[0].Value, ":") {
							changed = true
							n.Nodes = append([]*selectorNode{{Kind: "selector", Value: "*"}}, n.Nodes...)
						}
						n.Nodes = selectors(n.Nodes)
						result = append(result, n)
					}
					return result
				}
				parsed := selectors(parseSelector(n.Name))
				if changed {
					n.Name = selectorCSSMode(parsed, true)
				}
			}
			walk(n.Nodes)
		}
	}
	walk(ast)
	var css strings.Builder
	writeCSS(&css, ast, 0)
	sig := css.String()
	k.variantSignatures[name] = sig
	return sig
}

func (k *Canonicalizer) indexVariants() {
	if k.variantLookup != nil {
		return
	}
	k.variantLookup = map[string][]string{}
	for _, entry := range k.compiler.VariantList() {
		def, _ := k.compiler.variantDefinition(entry.Name)
		var names []string
		if def.Kind == "static" {
			names = []string{entry.Name}
		} else if def.Kind == "functional" {
			for _, value := range entry.Values {
				name := entry.Name
				if name != "@" {
					name += "-"
				}
				names = append(names, name+value)
			}
		}
		for _, name := range names {
			if sig := k.variantSignature(name); sig != "" {
				k.variantLookup[sig] = append(k.variantLookup[sig], name)
			}
		}
	}
}

func selectorIs(n *selectorNode, value string) bool { return n.Kind == "selector" && n.Value == value }
func selectorAttribute(n *selectorNode) bool {
	return n.Kind == "selector" && strings.HasPrefix(n.Value, "[") && strings.HasSuffix(n.Value, "]")
}
func selectorPseudo(n *selectorNode) bool {
	return (n.Kind == "selector" || n.Kind == "function") && strings.HasPrefix(n.Value, ":")
}

func (k *Canonicalizer) modernizeVariant(root *CandidateVariant) []*CandidateVariant {
	for v, parent := root, (*CandidateVariant)(nil); v != nil; parent, v = v, v.Variant {
		if v.Kind != "arbitrary" || v.Relative {
			continue
		}
		ast := parseSelector(v.Selector)
		if len(ast) == 0 || len(ast) == 1 && ast[0].Kind == "list" {
			continue
		}
		if len(ast) == 1 && ast[0].Kind == "complex" {
			ast = ast[0].Nodes
		}
		if parent == nil && len(ast) == 3 && selectorIs(ast[0], "&") && ast[1].Kind == "combinator" && selectorIs(ast[2], "*") {
			if ast[1].Value == ">" {
				*v = *k.parseVariant("*")
				continue
			}
			if ast[1].Value == " " {
				*v = *k.parseVariant("**")
				continue
			}
		}
		if parent == nil && len(ast) == 1 && ast[0].Kind == "compound" && len(ast[0].Nodes) == 2 && selectorIs(ast[0].Nodes[0], "&") {
			n := ast[0].Nodes[1]
			if n.Kind == "function" && n.Value == ":has" && len(n.Nodes) == 1 && n.Nodes[0].Kind == "selector" {
				if r := k.parseVariant("has-[" + selectorCSSMode(n.Nodes, true) + "]"); r != nil {
					*v = *r
					continue
				}
			}
		}
		if parent == nil && len(ast) == 3 && ast[0].Kind == "selector" && ast[1].Kind == "combinator" && ast[1].Value == " " && selectorIs(ast[2], "&") {
			if r := k.parseVariant("in-[" + selectorCSSMode(ast[:1], true) + "]"); r != nil {
				*v = *r
				continue
			}
		}
		if parent == nil && ast[0].Kind == "selector" && (ast[0].Value == "@media" || ast[0].Value == "@supports") {
			containsNot := false
			var remove func([]*valueNode) []*valueNode
			remove = func(nodes []*valueNode) []*valueNode {
				var result []*valueNode
				for _, n := range nodes {
					if n.kind == 'w' && n.value == "not" {
						containsNot = true
						continue
					}
					n.nodes = remove(n.nodes)
					result = append(result, n)
				}
				return result
			}
			nodes := parseValue(valueCSS(remove(parseValue(selectorCSSMode(ast, true)))))
			var spaces func([]*valueNode)
			spaces = func(nodes []*valueNode) {
				for _, n := range nodes {
					if n.kind == 's' && strings.TrimSpace(n.value) == "" {
						n.value = " "
					}
					spaces(n.nodes)
				}
			}
			spaces(nodes)
			if containsNot {
				name := "not-[" + valueCSS(nodes) + "]"
				if r := k.parseVariant(name); r != nil {
					sig := k.variantSignature(printVariant(v))
					if sig != "" && sig == k.variantSignature(printVariant(r)) {
						*v = *r
						continue
					}
				}
			}
		}
		var prefix *CandidateVariant
		if parent == nil && len(ast) == 3 && selectorIs(ast[0], "&") && ast[1].Kind == "combinator" && ast[2].Kind == "selector" && (selectorPseudo(ast[2]) || selectorAttribute(ast[2])) {
			if ast[1].Value == ">" {
				prefix = k.parseVariant("*")
				ast = ast[2:]
			} else if ast[1].Value == " " {
				prefix = k.parseVariant("**")
				ast = ast[2:]
			}
		}
		var removeNesting func([]*selectorNode) []*selectorNode
		removeNesting = func(nodes []*selectorNode) []*selectorNode {
			var result []*selectorNode
			for _, n := range nodes {
				if selectorIs(n, "&") {
					continue
				}
				if n.Kind != "function" {
					n.Nodes = removeNesting(n.Nodes)
				}
				if n.Kind == "compound" && len(n.Nodes) == 1 {
					result = append(result, n.Nodes...)
				} else {
					result = append(result, n)
				}
			}
			return result
		}
		ast = removeNesting(ast)
		if len(ast) != 1 {
			continue
		}
		target := ast[0]
		if target.Kind == "function" && target.Value == ":is" {
			if len(target.Nodes) != 1 || target.Nodes[0].Kind == "list" {
				continue
			}
			if target.Nodes[0].Kind == "selector" && !selectorAttribute(target.Nodes[0]) {
				continue
			}
			target = target.Nodes[0]
		}
		if selectorPseudo(target) {
			n := target
			not := false
			if n.Kind == "function" && n.Value == ":not" {
				not = true
				if len(n.Nodes) != 1 || !selectorPseudo(n.Nodes[0]) {
					continue
				}
				n = n.Nodes[0]
			}
			name := ""
			if n.Kind == "function" && n.Value == ":nth-child" && len(n.Nodes) == 1 && n.Nodes[0].Kind == "value" && (n.Nodes[0].Value == "odd" || n.Nodes[0].Value == "even") {
				name = n.Nodes[0].Value
				if not {
					if name == "odd" {
						name = "even"
					} else {
						name = "odd"
					}
					not = false
				}
			} else {
				for _, pair := range [][2]string{{":nth-child", "nth"}, {":nth-last-child", "nth-last"}, {":nth-of-type", "nth-of-type"}, {":nth-last-of-type", "nth-of-last-type"}} {
					if n.Value == pair[0] && n.Kind == "function" && len(n.Nodes) == 1 {
						name = pair[1] + "-[" + selectorCSSMode(n.Nodes, true) + "]"
						if n.Nodes[0].Kind == "value" && positiveInteger(n.Nodes[0].Value) {
							name = pair[1] + "-" + n.Nodes[0].Value
						}
						break
					}
				}
				if name == "" && not {
					sig := k.variantSignature(printVariant(v))
					if sig != "" && sig == k.variantSignature("not-["+n.Value+"]") {
						name = "[&" + n.Value + "]"
					}
				}
			}
			if name == "" {
				if prefix != nil {
					selector := target.Value
					if strings.HasPrefix(selector, "::") || slices.Contains([]string{":before", ":after", ":first-line", ":first-letter"}, strings.ToLower(selector)) {
						selector = "&" + selector
					}
					*v = CandidateVariant{Kind: "arbitrary", Selector: selector}
					return []*CandidateVariant{prefix, v}
				}
				continue
			}
			if not {
				name = "not-" + name
			}
			parsed := k.parseVariant(name)
			if parsed == nil {
				continue
			}
			*v = *parsed
		} else if selectorAttribute(target) {
			attr := parseAttribute(target.Value)
			if attr == nil {
				continue
			}
			if strings.HasPrefix(attr.Attribute, "data-") || strings.HasPrefix(attr.Attribute, "aria-") {
				family, name, _ := strings.Cut(attr.Attribute, "-")
				value := &CandidateValue{Kind: "arbitrary", Value: name}
				if family == "data" && attr.Value == nil || family == "aria" && attr.Value != nil && *attr.Operator == "=" && *attr.Value == "true" && attr.Sensitivity == nil {
					value.Kind = "named"
				} else if attr.Value != nil {
					key := name
					if family == "aria" {
						key = attr.Attribute
					}
					quote := ""
					if attr.Quote != nil {
						quote = *attr.Quote
					}
					value.Value = key + *attr.Operator + quote + *attr.Value + quote
					if attr.Sensitivity != nil {
						value.Value += " " + *attr.Sensitivity
					}
				}
				*v = CandidateVariant{Kind: "functional", Root: family, Value: value}
			} else {
				*v = CandidateVariant{Kind: "arbitrary", Selector: target.Value}
			}
		}
		if prefix != nil {
			return []*CandidateVariant{prefix, v}
		}
	}
	return []*CandidateVariant{root}
}
