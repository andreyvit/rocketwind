package rocketwind

import "strings"

func (c *Compiler) v3VariantDefinition(name string) (variantDefinition, bool) {
	if strings.HasPrefix(name, "group-") || strings.HasPrefix(name, "peer-") {
		return variantDefinition{}, false
	}
	order, known := v3VariantOrder[name]
	if formats, ok := v3VariantFormats[name]; ok {
		nodes, _ := pluginVariantFormats(formats, []*Node{{Kind: "slot"}})
		compounds := 2
		if len(formats) > 0 && strings.HasPrefix(formats[0], "@") {
			compounds = 1
		}
		return variantDefinition{Kind: "static", Order: order, Compounds: compounds, Nodes: nodes}, true
	}
	if value := configObject(c.themeLookup("screens")).Get(name); value != nil {
		query := v3ScreenQuery(value)
		if query != "" {
			return variantDefinition{Kind: "static", Order: v3VariantOrder["min"], Compounds: 1, Nodes: []*Node{{Kind: "at-rule", Name: "@media", Value: query, Nodes: []*Node{{Kind: "slot"}}}}}, true
		}
	}
	if known {
		kind := "functional"
		with := 0
		if containsWord("group peer has", name) {
			kind = "compound"
			with = 2
		}
		return variantDefinition{Kind: kind, Order: order, Compounds: 2, CompoundsWith: with}, true
	}
	return variantDefinition{}, false
}

func v3ScreenQuery(value any) string {
	if width, ok := value.(string); ok {
		return "(min-width: " + width + ")"
	}
	if values, ok := value.([]any); ok {
		var queries []string
		for _, value := range values {
			if q := v3ScreenQuery(value); q != "" {
				queries = append(queries, q)
			}
		}
		return strings.Join(queries, ", ")
	}
	object := configObject(value)
	if raw := object.Get("raw"); raw != nil {
		return themeString(raw)
	}
	var clauses []string
	for _, name := range []string{"min", "max"} {
		if v := object.Get(name); v != nil {
			clauses = append(clauses, "("+name+"-width: "+themeString(v)+")")
		}
	}
	return strings.Join(clauses, " and ")
}

func (b *buildContext) applyV3Variant(nodes []*Node, v *parsedVariant, depth int) ([]*Node, bool) {
	wrap := func(selector string) ([]*Node, bool) {
		return []*Node{{Kind: "rule", Name: selector, Nodes: nodes}}, true
	}
	at := func(query string) ([]*Node, bool) {
		return []*Node{{Kind: "at-rule", Name: "@media", Value: query, Nodes: nodes, variant: true}}, true
	}
	if v.root == "[]" {
		if v.relative && depth == 0 {
			return nil, false
		}
		if strings.HasPrefix(v.value, "@") {
			name, value, _ := strings.Cut(v.value, " ")
			return []*Node{{Kind: "at-rule", Name: name, Value: value, Nodes: nodes}}, true
		}
		return wrap(v.value)
	}
	if v.root == "before" || v.root == "after" {
		if !hasNode(nodes, "content") {
			nodes = append(declarations("content", "var(--tw-content)"), nodes...)
		}
	}
	if v.root == "visited" || v.root == "marker" {
		names := []string{"--tw-text-opacity"}
		if v.root == "visited" {
			names = append(names, "--tw-border-opacity", "--tw-bg-opacity")
		}
		nodes = v3RemoveAlpha(nodes, names)
	}
	if v.definition.Kind == "static" {
		return slotNodes(v.definition.Nodes, nodes), true
	}
	if v.root == "has" {
		if v.modifier != "" || v.child == nil || !v.child.arbitrary {
			return nil, false
		}
		selector := v.child.value
		if strings.HasPrefix(selector, "&:is(") && strings.HasSuffix(selector, ")") {
			selector = selector[5 : len(selector)-1]
		}
		return wrap("&:has(" + selector + ")")
	}
	if v.root == "group" || v.root == "peer" {
		if v.child == nil {
			return nil, false
		}
		if v.child.root == "visited" {
			nodes = v3RemoveAlpha(nodes, []string{"--tw-text-opacity", "--tw-bg-opacity", "--tw-border-opacity"})
		}
		children, ok := b.applyV3Variant([]*Node{{Kind: "slot"}}, v.child, depth+1)
		if !ok {
			return nil, false
		}
		marker := "." + Escape(v.root)
		if v.modifier != "" {
			marker = "." + Escape(v.root+"/"+v.modifier)
		}
		combinator := " &"
		if v.root == "peer" {
			combinator = " ~ &"
		}
		for _, n := range children {
			if n.Kind != "rule" {
				return nil, false
			}
			n.Name = strings.ReplaceAll(n.Name, "&", marker) + combinator
		}
		return slotNodes(children, nodes), true
	}
	if v.value == "" || v.modifier != "" {
		return nil, false
	}
	switch v.root {
	case "min", "max":
		value := v.value
		if !v.arbitrary {
			value = themeString(configObject(b.compiler.themeLookup("screens")).Get(value))
		}
		if value == "" {
			return nil, false
		}
		query := "(min-width: " + value + ")"
		if v.root == "max" {
			if v.arbitrary {
				query = "(max-width: " + value + ")"
			} else {
				query = "not all and " + query
			}
		}
		return at(query)
	case "aria", "data":
		value := v.value
		if !v.arbitrary {
			value = themeString(configObject(b.compiler.themeLookup(v.root)).Get(value))
			if value == "" {
				return nil, false
			}
		}
		return wrap("&[" + v.root + "-" + quoteAttributeValue(value) + "]")
	case "supports":
		value := v.value
		if !v.arbitrary {
			value = themeString(configObject(b.compiler.themeLookup("supports")).Get(value))
			if value == "" {
				return nil, false
			}
		}
		if !strings.ContainsAny(value, ":(") {
			value += ": var(--tw)"
		}
		if !strings.HasPrefix(value, "(") && !strings.HasPrefix(value, "selector(") && !strings.HasPrefix(value, "font-tech(") && !strings.HasPrefix(value, "font-format(") {
			value = "(" + value + ")"
		}
		return []*Node{{Kind: "at-rule", Name: "@supports", Value: value, Nodes: nodes, variant: true}}, true
	}
	return nil, false
}

func v3RemoveAlpha(nodes []*Node, names []string) []*Node {
	var result []*Node
	for _, n := range nodes {
		remove := false
		copy := *n
		for _, name := range names {
			if n.Kind == "declaration" && n.Name == name {
				remove = true
				break
			}
			copy.Value = strings.ReplaceAll(copy.Value, "/ var("+name+", 1)", "")
			copy.Value = strings.ReplaceAll(copy.Value, "/ var("+name+")", "")
		}
		if remove {
			continue
		}
		if n.Nodes != nil {
			copy.Nodes = v3RemoveAlpha(n.Nodes, names)
		}
		result = append(result, &copy)
	}
	return result
}
