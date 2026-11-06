package rocketwind

import (
	"cmp"
	"slices"
	"strings"
)

type variantDefinition struct {
	Kind                            string
	Order, Compounds, CompoundsWith int
	Nodes                           []*Node
}
type parsedVariant struct {
	root, value, modifier string
	arbitrary, relative   bool
	child                 *parsedVariant
	definition            variantDefinition
	matcher               *Variant
}

func (b *buildContext) parseVariant(raw string) *parsedVariant {
	return b.compiledVariant(parseVariantSyntax(raw, b.compiler.variantDefinition))
}

func (b *buildContext) compiledVariant(v *CandidateVariant) *parsedVariant {
	if v == nil {
		return nil
	}
	r := &parsedVariant{root: v.Root, relative: v.Relative, definition: v.definition}
	if v.Kind == "arbitrary" {
		r.root = "[]"
		r.value = v.Selector
		r.arbitrary = true
	}
	if v.Value != nil {
		r.value = v.Value.Value
		r.arbitrary = v.Value.Kind == "arbitrary"
	}
	if v.Modifier != nil {
		r.modifier = v.Modifier.Value
	}
	r.child = b.compiledVariant(v.Variant)
	r.matcher = b.compiler.matchingVariants[v.Root]
	return r
}
func (c *Compiler) variantDefinition(name string) (variantDefinition, bool) {
	if d, ok := c.customVariants[name]; ok {
		return d, true
	}
	if d, ok := builtinVariants[name]; ok {
		if d.Order > builtinVariants["min"].Order {
			d.Order += c.legacyScreenCount
		}
		return d, true
	}
	if c.breakpointVariants != nil {
		d, ok := c.breakpointVariants[name]
		return d, ok
	}
	b := buildContext{compiler: c}
	if value := b.themeValue("--breakpoint-" + name); value != "" {
		return variantDefinition{Kind: "static", Order: builtinVariants["min"].Order, Compounds: 1, Nodes: []*Node{{Kind: "at-rule", Name: "@media", Value: "(width >= " + value + ")", Nodes: []*Node{{Kind: "slot"}}}}}, true
	}
	return variantDefinition{}, false
}
func (c *Compiler) prepareBreakpoints() {
	variants := make(map[string]variantDefinition)
	b := buildContext{compiler: c}
	add := func(key string) {
		if !strings.HasPrefix(key, "--breakpoint-") {
			return
		}
		if value := b.themeValue(key); value != "" {
			variants[strings.TrimPrefix(key, "--breakpoint-")] = variantDefinition{Kind: "static", Order: builtinVariants["min"].Order, Compounds: 1, Nodes: []*Node{{Kind: "at-rule", Name: "@media", Value: "(width >= " + value + ")", Nodes: []*Node{{Kind: "slot"}}}}}
		}
	}
	if !c.config.NoDefaultTheme {
		for key := range builtinTheme {
			add(key)
		}
	}
	for key := range c.config.Theme {
		add(key)
	}
	c.breakpointVariants = variants
}

func slotNodes(template, content []*Node) []*Node {
	var result []*Node
	for _, n := range template {
		if n.Kind == "slot" || n.Name == "@slot" {
			result = append(result, content...)
			continue
		}
		r := *n
		r.variant = true
		if n.Nodes != nil {
			r.Nodes = slotNodes(n.Nodes, content)
		}
		result = append(result, &r)
	}
	return result
}
func (b *buildContext) applyParsedVariant(nodes []*Node, v *parsedVariant, depth int) ([]*Node, bool) {
	if !v.arbitrary {
		if value := b.compiler.legacyVariantValues[v.root].Get(v.value); value != nil {
			copy := *v
			copy.arbitrary = true
			copy.value = themeString(value)
			v = &copy
		}
	}
	wrap := func(selector string) ([]*Node, bool) {
		return []*Node{{Kind: "rule", Name: selector, Nodes: nodes}}, true
	}
	at := func(name, query string) ([]*Node, bool) {
		return []*Node{{Kind: "at-rule", Name: name, Value: query, Nodes: nodes, variant: true}}, true
	}
	if v.root == "[]" {
		if v.relative && depth == 0 {
			return nil, false
		}
		if strings.HasPrefix(v.value, "@") {
			i := 1
			for i < len(v.value) && (asciiLetter(v.value[i]) || v.value[i] == '-') {
				i++
			}
			return at(v.value[:i], strings.TrimSpace(v.value[i:]))
		}
		return wrap(v.value)
	}
	if v.definition.Kind == "static" {
		return slotNodes(v.definition.Nodes, nodes), true
	}
	if v.definition.Kind == "compound" {
		return b.compoundVariant(nodes, v, depth)
	}
	if v.matcher != nil {
		value := v.value
		if !v.arbitrary {
			found := false
			for _, entry := range v.matcher.Values {
				if entry.Name == value || value == "" && entry.Name == "DEFAULT" {
					value = entry.Value
					found = true
					break
				}
			}
			if !found {
				return nil, false
			}
		}
		result, err := pluginVariantFormats(variantFormats(v.matcher, value, v.modifier), nodes)
		if err != nil {
			b.err = err
			return nil, false
		}
		return result, len(result) > 0
	}
	if v.value == "" {
		return nil, false
	}
	switch v.root {
	case "min", "max", "@", "@min", "@max":
		value := v.value
		namespace := "--breakpoint-"
		name := "@media"
		op := ">="
		if strings.HasPrefix(v.root, "@") {
			namespace = "--container-"
			name = "@container"
		} else if v.modifier != "" {
			return nil, false
		}
		if !v.arbitrary {
			value = b.themeValue(namespace + value)
		}
		if value == "" || strings.Contains(value, "var(") {
			return nil, false
		}
		if v.root == "max" || v.root == "@max" {
			op = "<"
		}
		query := "(width " + op + " " + value + ")"
		if v.modifier != "" {
			query = v.modifier + " " + query
		}
		return at(name, query)
	case "data", "aria":
		if v.modifier != "" {
			return nil, false
		}
		value := v.value
		if v.root == "aria" && !v.arbitrary {
			value += "=\"true\""
		} else {
			value = quoteAttributeValue(value)
		}
		selector := "[" + v.root + "-" + value + "]"
		if !validAttributeSelector(selector) {
			return nil, false
		}
		return wrap("&" + selector)
	case "nth", "nth-last", "nth-of-type", "nth-last-of-type":
		if v.modifier != "" || !v.arbitrary && !positiveInteger(v.value) {
			return nil, false
		}
		name := v.root
		if name == "nth" || name == "nth-last" {
			name += "-child"
		}
		return wrap("&:" + name + "(" + v.value + ")")
	case "supports":
		if v.modifier != "" {
			return nil, false
		}
		value := v.value
		i := 0
		for i < len(value) && (asciiLetter(value[i]) || digit(value[i]) || value[i] == '_' || value[i] == '-') {
			i++
		}
		for i < len(value) && cssSpace(value[i]) {
			i++
		}
		if i < len(value) && value[i] == '(' {
			ast := parseValue(value)
			var space func([]*valueNode)
			space = func(nodes []*valueNode) {
				for _, n := range nodes {
					if n.kind != 'f' || n.value == "selector" {
						continue
					}
					if n.value == "and" || n.value == "or" || n.value == "not" {
						n.value = " " + n.value + " "
					}
					space(n.nodes)
				}
			}
			space(ast)
			return at("@supports", valueCSS(ast))
		}
		if !strings.Contains(value, ":") {
			value += ": var(--tw)"
		}
		if value[0] != '(' || value[len(value)-1] != ')' {
			value = "(" + value + ")"
		}
		return at("@supports", value)
	}
	return nil, false
}
func quoteAttributeValue(input string) string {
	parts := slices.Collect(Segments(input, '='))
	if len(parts) < 2 {
		return input
	}
	value := strings.TrimSpace(strings.Join(parts[1:], "="))
	if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
		return input
	}
	suffix := ""
	if len(value) > 1 && value[len(value)-2] == ' ' && strings.ContainsRune("iIsS", rune(value[len(value)-1])) {
		suffix = value[len(value)-2:]
		value = value[:len(value)-2]
	}
	return parts[0] + "=\"" + value + "\"" + suffix
}
func validAttributeSelector(s string) bool { return parseAttribute(s) != nil }

func (b *buildContext) compoundVariant(nodes []*Node, v *parsedVariant, depth int) ([]*Node, bool) {
	if v.modifier != "" && v.root != "group" && v.root != "peer" {
		return nil, false
	}
	if v.child.relative && v.root != "has" && v.root != "in" {
		return nil, false
	}
	isolated, ok := b.applyParsedVariant([]*Node{{Kind: "slot"}}, v.child, depth+1)
	if !ok {
		return nil, false
	}
	if v.root == "not" {
		return negateVariant(isolated, nodes)
	}
	applied, valid := false, true
	var visit func([]*Node, int)
	visit = func(children []*Node, ruleDepth int) {
		for _, n := range children {
			if n.Kind == "slot" {
				continue
			}
			if n.Kind == "rule" {
				if ruleDepth > 0 {
					valid = false
					return
				}
				selector := n.Name
				switch v.root {
				case "group", "peer":
					name := v.root
					if p := b.compiler.config.Prefix; p != "" {
						name = p + ":" + name
					}
					if v.modifier != "" {
						name += "/" + v.modifier
					}
					selector = strings.ReplaceAll(selector, "&", ":where(."+Escape(name)+")")
					if len(slices.Collect(Segments(selector, ','))) > 1 {
						selector = ":is(" + selector + ")"
					}
					combinator := " "
					if v.root == "peer" {
						combinator = " ~ "
					}
					selector = ":is(" + selector + combinator + "&)"
				case "has":
					selector = "&:has(" + strings.ReplaceAll(selector, "&", "*") + ")"
				case "in":
					selector = ":where(" + strings.ReplaceAll(selector, "&", "*") + ") &"
				}
				n.Name = selector
				applied = true
				visit(n.Nodes, ruleDepth+1)
			} else {
				visit(n.Nodes, ruleDepth)
			}
		}
	}
	visit(isolated, 0)
	return slotNodes(isolated, nodes), valid && applied
}
func negateVariant(isolated, content []*Node) ([]*Node, bool) {
	if len(isolated) != 1 {
		return nil, false
	}
	var rules, ats []*Node
	var walk func([]*Node) bool
	walk = func(nodes []*Node) bool {
		for _, n := range nodes {
			switch n.Kind {
			case "slot":
				continue
			case "rule":
				rules = append(rules, n)
			case "at-rule":
				ats = append(ats, n)
			default:
				return false
			}
			if !walk(n.Nodes) {
				return false
			}
		}
		return true
	}
	if !walk(isolated) || len(rules) > 1 || len(ats) > 1 {
		return nil, false
	}
	var result []*Node
	for _, n := range rules {
		if strings.Contains(n.Name, "::") {
			return nil, false
		}
		selector := "&:not(" + strings.ReplaceAll(n.Name, "&", "*") + ")"
		result = append(result, &Node{Kind: "rule", Name: selector, Nodes: content})
	}
	for _, n := range ats {
		if !containsWord("@media @supports @container", n.Name) || len(slices.Collect(Segments(n.Value, ','))) > 1 {
			return nil, false
		}
		query := strings.TrimSpace(n.Value)
		if n.Name == "@container" && !strings.HasPrefix(query, "(") && !strings.HasPrefix(query, "not ") {
			name, tail, found := strings.Cut(query, " ")
			if !found {
				query = "not " + query
			} else if strings.HasPrefix(tail, "not ") {
				query = name + " " + tail[4:]
			} else {
				query = name + " not " + tail
			}
		} else if strings.HasPrefix(query, "not ") {
			query = query[4:]
		} else {
			query = "not " + query
		}
		result = append(result, &Node{Kind: "at-rule", Name: n.Name, Value: query, Nodes: content})
	}
	return result, len(result) > 0
}
func (b *buildContext) compareParsedVariant(a, z *parsedVariant) int {
	if a == z {
		return 0
	}
	if a == nil {
		return -1
	}
	if z == nil {
		return 1
	}
	if v := cmp.Compare(a.definition.Order, z.definition.Order); v != 0 {
		return v
	}
	if a.child != nil && z.child != nil {
		if v := b.compareParsedVariant(a.child, z.child); v != 0 {
			return v
		}
		return strings.Compare(a.modifier, z.modifier)
	}
	order := a.definition.Order
	if order == builtinVariants["min"].Order || order == builtinVariants["max"].Order || order == builtinVariants["@"].Order+b.compiler.legacyScreenCount || order == builtinVariants["@max"].Order+b.compiler.legacyScreenCount {
		value := func(v *parsedVariant) string {
			if v.arbitrary {
				return v.value
			}
			if v.definition.Kind == "static" {
				return b.themeValue("--breakpoint-" + v.root)
			}
			namespace := "--breakpoint-"
			if strings.HasPrefix(v.root, "@") {
				namespace = "--container-"
			}
			return b.themeValue(namespace + v.value)
		}
		return compareBreakpointValues(value(a), value(z), order == builtinVariants["max"].Order || order == builtinVariants["@max"].Order+b.compiler.legacyScreenCount)
	}
	if v := strings.Compare(a.root, z.root); v != 0 {
		return v
	}
	if a.matcher != nil && a.matcher.Sort != nil {
		if a.root == z.root && a.value == z.value && a.modifier == z.modifier && a.arbitrary == z.arbitrary {
			return 0
		}
		resolve := func(v *parsedVariant) VariantValue {
			value := v.value
			if value == "" {
				value = "DEFAULT"
			}
			for _, entry := range v.matcher.Values {
				if entry.Name == value {
					value = entry.Value
					break
				}
			}
			return VariantValue{Value: value, Modifier: v.modifier}
		}
		return a.matcher.Sort(resolve(a), resolve(z))
	}
	if v := cmp.Compare(boolInt(a.arbitrary), boolInt(z.arbitrary)); v != 0 {
		return v
	}
	if a.matcher != nil && !a.arbitrary {
		rank := func(v string) int {
			for i, entry := range a.matcher.Values {
				if entry.Name == v {
					return i
				}
			}
			return -1
		}
		return cmp.Compare(rank(a.value), rank(z.value))
	}
	if a.matcher != nil && a.value == z.value && a.modifier != z.modifier {
		return 1
	}
	return strings.Compare(a.value, z.value)
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (b *buildContext) compareVariantKeys(a, z []*parsedVariant) int {
	for i := 0; i < min(len(a), len(z)); i++ {
		if v := b.compareParsedVariant(a[i], z[i]); v != 0 {
			return v
		}
	}
	return cmp.Compare(len(a), len(z))
}
