package rocketwind

import (
	"cmp"
	"slices"
	"strings"
)

type variantDefinition struct {
	Kind             string
	Order, Compounds int
	Nodes            []*Node
}
type parsedVariant struct {
	root, value, modifier string
	arbitrary, relative   bool
	child                 *parsedVariant
	definition            variantDefinition
	matcher               *Variant
}

func (b *buildContext) parseVariant(raw string) *parsedVariant {
	if len(raw) > 2 && raw[0] == '[' && raw[len(raw)-1] == ']' {
		value := raw[1 : len(raw)-1]
		if !validArbitrary(value) || value[0] == '@' && strings.Contains(value, "&") {
			return nil
		}
		value = DecodeArbitraryValue(value)
		if strings.TrimSpace(value) == "" {
			return nil
		}
		relative := strings.ContainsRune(">+~", rune(value[0]))
		if !relative && value[0] != '@' && !strings.Contains(value, "&") {
			value = "&:is(" + value + ")"
		}
		compounds := 2
		if value[0] == '@' {
			compounds = 1
		}
		if strings.Contains(value, "::") {
			compounds = 0
		}
		return &parsedVariant{root: "[]", value: value, arbitrary: true, relative: relative, definition: variantDefinition{Kind: "arbitrary", Order: 10000, Compounds: compounds}}
	}
	parts := slices.Collect(Segments(raw, '/'))
	if len(parts) > 2 {
		return nil
	}
	raw = parts[0]
	modifier := ""
	if len(parts) == 2 {
		modifier = parts[1]
		if modifier == "" {
			return nil
		}
		if modifier[0] == '[' || modifier[0] == '(' {
			var ok bool
			modifier, ok = parseModifier(modifier)
			if !ok {
				return nil
			}
		} else if !validNamed(modifier) {
			return nil
		}
	}
	for end := len(raw); end > 0; {
		root, value := raw[:end], ""
		if end < len(raw) {
			value = raw[end+1:]
		}
		if strings.HasPrefix(raw, "@") && root == "@" && len(raw) > 1 {
			value = raw[1:]
		}
		d, ok := b.compiler.variantDefinition(root)
		if ok {
			v := &parsedVariant{root: root, value: value, modifier: modifier, definition: d}
			if match := b.compiler.matchingVariants[root]; match != nil {
				v.matcher = match
			}
			switch d.Kind {
			case "static":
				if value != "" || modifier != "" {
					return nil
				}
				return v
			case "compound":
				if value == "" {
					return nil
				}
				if modifier != "" && (root == "not" || root == "has" || root == "in") {
					value += "/" + modifier
					v.modifier = ""
				}
				v.child = b.parseVariant(value)
				if v.child == nil || v.child.definition.Compounds == 0 {
					return nil
				}
				if root != "not" && v.child.definition.Compounds&2 == 0 {
					return nil
				}
				return v
			case "functional":
				if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "(") {
					var ok bool
					v.value, ok = parseModifier(value)
					if !ok {
						return nil
					}
					v.arbitrary = true
				} else if !validNamed(value) {
					return nil
				}
				return v
			}
		}
		end = strings.LastIndexByte(raw[:end], '-')
		if end < 0 && strings.HasPrefix(raw, "@") {
			end = 1
		}
		if root == "@" {
			break
		}
	}
	return nil
}
func parseModifier(raw string) (string, bool) {
	if len(raw) < 3 {
		return "", false
	}
	if raw[0] == '[' && raw[len(raw)-1] == ']' {
		v := raw[1 : len(raw)-1]
		if !validArbitrary(v) {
			return "", false
		}
		v = DecodeArbitraryValue(v)
		return v, strings.TrimSpace(v) != ""
	}
	if raw[0] == '(' && raw[len(raw)-1] == ')' {
		v := raw[1 : len(raw)-1]
		if !strings.HasPrefix(v, "--") || !validArbitrary(v) {
			return "", false
		}
		return DecodeArbitraryValue("var(" + v + ")"), true
	}
	return "", false
}
func (c *Compiler) variantDefinition(name string) (variantDefinition, bool) {
	if d, ok := c.customVariants[name]; ok {
		return d, true
	}
	if d, ok := builtinVariants[name]; ok {
		return d, true
	}
	b := buildContext{compiler: c}
	if value := b.themeValue("--breakpoint-" + name); value != "" {
		return variantDefinition{Kind: "static", Order: builtinVariants["min"].Order, Compounds: 1, Nodes: []*Node{{Kind: "at-rule", Name: "@media", Value: "(width >= " + value + ")", Nodes: []*Node{{Kind: "slot"}}}}}, true
	}
	return variantDefinition{}, false
}
func (b *buildContext) applyVariants(nodes []*Node, variants []string) ([]*Node, bool) {
	for i := len(variants) - 1; i >= 0; i-- {
		v := b.parseVariant(variants[i])
		if v == nil {
			return nil, false
		}
		var ok bool
		nodes, ok = b.applyParsedVariant(nodes, v, 0)
		if !ok {
			return nil, false
		}
	}
	return nodes, true
}
func slotNodes(template, content []*Node) []*Node {
	var result []*Node
	for _, n := range template {
		if n.Kind == "slot" || n.Name == "@slot" {
			result = append(result, content...)
			continue
		}
		r := *n
		if n.Nodes != nil {
			r.Nodes = slotNodes(n.Nodes, content)
		}
		result = append(result, &r)
	}
	return result
}
func (b *buildContext) applyParsedVariant(nodes []*Node, v *parsedVariant, depth int) ([]*Node, bool) {
	wrap := func(selector string) ([]*Node, bool) {
		return []*Node{{Kind: "rule", Name: selector, Nodes: nodes}}, true
	}
	at := func(name, query string) ([]*Node, bool) {
		return []*Node{{Kind: "at-rule", Name: name, Value: query, Nodes: nodes}}, true
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
				if entry.Name == value {
					value = entry.Value
					found = true
					break
				}
			}
			if !found {
				return nil, false
			}
		}
		selector := v.matcher.Match(value, v.modifier)
		if selector == "" {
			return nil, false
		}
		return wrap(selector)
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
func validAttributeSelector(s string) bool {
	// Full attribute parser is shared with selector handling below.
	body := s[1 : len(s)-1]
	parts := slices.Collect(Segments(body, '='))
	name := strings.TrimSpace(parts[0])
	if name != "" && strings.ContainsRune("~|^$*", rune(name[len(name)-1])) && parts[0] != strings.TrimRight(parts[0], " \t\n") {
		return false
	}
	name = strings.TrimRight(name, "~|^$*")
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " []()\"'<>!@#%&+,;:{}") {
		return false
	}
	if len(parts) > 1 {
		v := strings.TrimSpace(strings.Join(parts[1:], "="))
		if len(v) < 2 || (v[0] != '\'' && v[0] != '"') {
			return false
		}
		q := v[0]
		i := 1
		for i < len(v) && v[i] != q {
			if v[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(v) {
			return false
		}
		tail := strings.TrimSpace(v[i+1:])
		if tail != "" && !containsWord("i I s S", tail) {
			return false
		}
	}
	return true
}
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
	if a == nil {
		if z == nil {
			return 0
		}
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
	if order == builtinVariants["min"].Order || order == builtinVariants["max"].Order || order == builtinVariants["@"].Order || order == builtinVariants["@max"].Order {
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
		return compareBreakpointValues(value(a), value(z), order == builtinVariants["max"].Order || order == builtinVariants["@max"].Order)
	}
	if v := strings.Compare(a.root, z.root); v != 0 {
		return v
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
	return strings.Compare(a.value, z.value)
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (b *buildContext) compareVariant(a, z string) int {
	return b.compareParsedVariant(b.parseVariant(a), b.parseVariant(z))
}
func (b *buildContext) compareVariantSets(a, z []string) int {
	a = slices.Clone(a)
	z = slices.Clone(z)
	reverse := func(a, z string) int { return -b.compareVariant(a, z) }
	slices.SortFunc(a, reverse)
	slices.SortFunc(z, reverse)
	a = slices.CompactFunc(a, func(a, z string) bool { return b.compareVariant(a, z) == 0 })
	z = slices.CompactFunc(z, func(a, z string) bool { return b.compareVariant(a, z) == 0 })
	for i := 0; i < min(len(a), len(z)); i++ {
		if v := b.compareVariant(a[i], z[i]); v != 0 {
			return v
		}
	}
	return cmp.Compare(len(a), len(z))
}
