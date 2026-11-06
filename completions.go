package rocketwind

import (
	"maps"
	"slices"
	"strings"
)

// ClassSuggestion describes a named utility and its suggested slash modifiers.
// Suggestions are unprefixed, as in Tailwind's editor API.
type ClassSuggestion struct {
	Name      string
	Modifiers []string
}

type completionGroup struct {
	Values, Modifiers, ValueKeys, ModifierKeys []string
	Negative, Fractions, Default               bool
}
type completionDefinition struct {
	Keys  []string
	Cases [][]completionGroup
}
type completionRoot struct {
	Name        string
	Definitions []completionDefinition
}
type completionItem struct {
	ClassSuggestion
	root     string
	fraction bool
}

// ClassList returns the theme's named utility suggestions in Tailwind order.
// It includes Go plugins and @utility definitions, and does not compile CSS.
func (c *Compiler) ClassList() []ClassSuggestion {
	keys := c.themeKeys()
	items := make(map[string]int)
	storage := make([]completionItem, 0, len(builtinStaticCompletions)+len(keys)*16)
	item := func(name string) *completionItem {
		if index, found := items[name]; found {
			return &storage[index]
		}
		items[name] = len(storage)
		storage = append(storage, completionItem{ClassSuggestion: ClassSuggestion{Name: name}, root: name})
		return &storage[len(storage)-1]
	}
	for _, name := range builtinStaticCompletions {
		item(name)
	}
	for _, name := range c.customOrder {
		if c.custom[name] != nil {
			item(name)
		}
	}
	themeKeys := keys
	themeCache := make(map[string][]string)
	resolve := func(namespaces []string) []string {
		var result []string
		for _, namespace := range namespaces {
			values, ok := themeCache[namespace]
			if !ok {
				values = c.completionThemeValues(themeKeys, []string{namespace})
				themeCache[namespace] = values
			}
			result = append(result, values...)
		}
		return result
	}
	addGroup := func(root string, group completionGroup) {
		values := append(slices.Clone(group.Values), resolve(group.ValueKeys)...)
		if group.Fractions {
			values = append(values, fractionSuggestions...)
		}
		if group.Default {
			values = append([]string{""}, values...)
		}
		modifiers := append(slices.Clone(group.Modifiers), resolve(group.ModifierKeys)...)
		modifiers = appendUnique(modifiers[:0], modifiers...)
		for _, value := range values {
			name := root
			if value != "" {
				name += "-" + value
			}
			lhs, rhs, slash := strings.Cut(value, "/")
			fraction := slash && decimalDigits(lhs) && decimalDigits(rhs)
			entry := item(name)
			entry.root, entry.fraction = root, entry.fraction || fraction
			entry.Modifiers = mergeCompletionModifiers(entry.Modifiers, modifiers)
			if group.Negative {
				entry := item("-" + name)
				entry.root, entry.fraction = "-"+root, entry.fraction || fraction
				if len(entry.Modifiers) == 0 {
					entry.Modifiers = modifiers
				} else {
					entry.Modifiers = append(slices.Clone(entry.Modifiers), modifiers...)
				}
			}
		}
	}
	b := buildContext{compiler: c}
	for _, root := range builtinFunctionalCompletions {
		for _, definition := range root.Definitions {
			mask := 0
			for i, key := range definition.Keys {
				if b.themeValue(key) != "" {
					mask |= 1 << i
				}
			}
			for _, group := range definition.Cases[mask] {
				addGroup(root.Name, group)
			}
		}
	}
	for _, root := range slices.Sorted(maps.Keys(c.matchingUtilities)) {
		for _, matcher := range c.matchingUtilities[root] {
			if matcher.Name != root {
				continue
			}
			group := completionGroup{Negative: matcher.SupportsNegative}
			for _, value := range matcher.Values {
				if value.Name == "__BARE_VALUE__" || value.Name == "__CSS_VALUES__" {
					continue
				}
				if value.Name == "" || value.Name == "DEFAULT" {
					group.Default = true
				} else {
					group.Values = append(group.Values, value.Name)
				}
			}
			for _, value := range matcher.Modifiers {
				group.Modifiers = append(group.Modifiers, value.Name)
			}
			addGroup(root, group)
		}
	}
	seenCustom := make(map[string]bool)
	for _, name := range c.customOrder {
		root := strings.TrimSuffix(name, "-*")
		if seenCustom[root] {
			continue
		}
		seenCustom[root] = true
		for _, nodes := range c.customFunctional[root] {
			addGroup(root, customCompletions(nodes))
		}
	}
	list := make([]int, len(storage))
	for i := range list {
		list[i] = i
	}
	slices.SortFunc(list, func(a, b int) int { return naturalCompare(storage[a].Name, storage[b].Name) })
	// Fractions follow the last ordinary class in their utility's last bucket.
	last := make(map[string]int)
	fractions := make(map[string][]int)
	modifierCount := 0
	for i, index := range list {
		entry := &storage[index]
		modifierCount += len(entry.Modifiers)
		last[entry.root] = i
		if entry.fraction {
			fractions[entry.root] = append(fractions[entry.root], index)
		}
	}
	result := make([]ClassSuggestion, 0, len(list))
	modifiers := make([]string, 0, modifierCount)
	add := func(entry ClassSuggestion) {
		start := len(modifiers)
		modifiers = append(modifiers, entry.Modifiers...)
		entry.Modifiers = modifiers[start:len(modifiers):len(modifiers)]
		result = append(result, entry)
	}
	for i, index := range list {
		entry := &storage[index]
		if !entry.fraction {
			add(entry.ClassSuggestion)
		}
		if last[entry.root] == i {
			for _, fraction := range fractions[entry.root] {
				add(storage[fraction].ClassSuggestion)
			}
		}
	}
	return result
}

// Completion groups share immutable modifier lists while being assembled.
func mergeCompletionModifiers(before, added []string) []string {
	if len(before) == 0 {
		return added
	}
	for _, value := range added {
		if !slices.Contains(before, value) {
			result := make([]string, len(before), len(before)+len(added))
			copy(result, before)
			return appendUnique(result, added...)
		}
	}
	return before
}

func appendUnique(values []string, added ...string) []string {
	for _, value := range added {
		if !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func decimalDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if !digit(s[i]) {
			return false
		}
	}
	return true
}

func (c *Compiler) completionThemeValues(keys, namespaces []string) []string {
	var result []string
	b := buildContext{compiler: c}
	for _, namespace := range namespaces {
		for _, key := range keys {
			value, ok := strings.CutPrefix(key, namespace+"-")
			if !ok || strings.Contains(key[2:], "--") || ignoredThemeKey(key, namespace) || b.themeValue(key) == "" {
				continue
			}
			// Old theme keys used underscores for decimal points only between digits.
			for i := 1; i+1 < len(value); i++ {
				if value[i] == '_' && digit(value[i-1]) && digit(value[i+1]) {
					value = value[:i] + "." + value[i+1:]
				}
			}
			result = append(result, value)
		}
	}
	return result
}

func customCompletions(nodes []*Node) completionGroup {
	type args struct {
		literals, keys  []string
		number, integer bool
	}
	var values, modifiers args
	var visitValue func([]*valueNode, bool)
	visitValue = func(nodes []*valueNode, spacing bool) {
		for _, node := range nodes {
			if node.kind != 'f' {
				continue
			}
			if node.value == "--value" || node.value == "--modifier" {
				target := &values
				if node.value == "--modifier" {
					target = &modifiers
				}
				for arg := range Segments(valueCSS(node.nodes), ',') {
					arg = normalizeCustomArg(arg)
					switch {
					case len(arg) >= 2 && (arg[0] == '\'' || arg[0] == '"') && arg[len(arg)-1] == arg[0]:
						target.literals = appendUnique(target.literals, arg[1:len(arg)-1])
					case strings.HasPrefix(arg, "--") && !strings.Contains(arg, "("):
						key, _, _ := strings.Cut(arg, "-*")
						target.keys = appendUnique(target.keys, key)
					case spacing && arg == "number":
						target.number = true
					case spacing && arg == "integer":
						target.integer = true
					}
				}
			}
			visitValue(node.nodes, spacing || node.value == "--spacing")
		}
	}
	var visit func([]*Node)
	visit = func(nodes []*Node) {
		for _, node := range nodes {
			if node.Kind == "declaration" {
				visitValue(parseValue(node.Value), false)
			}
			visit(node.Nodes)
		}
	}
	visit(nodes)
	for _, target := range []*args{&values, &modifiers} {
		if target.number || target.integer {
			for _, value := range spacingSuggestions {
				if target.number || decimalDigits(value) {
					target.literals = append(target.literals, value)
				}
			}
		}
	}
	return completionGroup{Values: values.literals, Modifiers: modifiers.literals, ValueKeys: values.keys, ModifierKeys: modifiers.keys}
}

// VariantSuggestion describes a variant and its named arguments.
type VariantSuggestion struct {
	Name               string
	Values             []string
	Arbitrary, HasDash bool
}

// VariantList returns variants in registration order, including Go plugins.
func (c *Compiler) VariantList() []VariantSuggestion {
	names := make([]string, 0, len(builtinVariantCompletionOrder)+len(c.variantOrder))
	keys := c.themeKeys()
	for _, name := range builtinVariantCompletionOrder {
		if name == "min" {
			names = appendUnique(names, c.completionThemeValues(keys, []string{"--breakpoint"})...)
		}
		names = appendUnique(names, name)
	}
	names = appendUnique(names, c.variantOrder...)
	result := make([]VariantSuggestion, 0, len(names))
	for _, name := range names {
		definition, _ := c.variantDefinition(name)
		entry := VariantSuggestion{Name: name, Arbitrary: definition.Kind != "static", HasDash: name != "@"}
		if matcher := c.matchingVariants[name]; matcher != nil && definition.Kind == "functional" {
			for _, value := range matcher.Values {
				if value.Name != "" && value.Name != "DEFAULT" {
					entry.Values = append(entry.Values, value.Name)
				}
			}
		} else {
			switch name {
			case "not", "group", "peer", "has", "in":
				for _, child := range names {
					d, ok := c.variantDefinition(child)
					if ok && definition.CompoundsWith&d.Compounds != 0 {
						entry.Values = append(entry.Values, child)
					}
				}
			case "aria":
				entry.Values = []string{"busy", "checked", "disabled", "expanded", "hidden", "pressed", "readonly", "required", "selected"}
			case "min", "max":
				entry.Values = c.completionThemeValues(keys, []string{"--breakpoint"})
			case "@", "@min", "@max":
				entry.Values = c.completionThemeValues(keys, []string{"--container"})
			}
		}
		result = append(result, entry)
	}
	return result
}

// VariantSelectors returns nested selector examples for a complete variant,
// such as "hover", "group-hover/sidebar" or "min-[800px]".
func (c *Compiler) VariantSelectors(name string) []string {
	b := buildContext{compiler: c, used: map[string]bool{}}
	v := b.parseVariant(name)
	if v == nil {
		return nil
	}
	nodes, ok := b.applyParsedVariant(nil, v, 0)
	if !ok {
		return nil
	}
	var result []string
	var visit func([]*Node, []*Node)
	visit = func(nodes, path []*Node) {
		for _, node := range nodes {
			if node.Kind != "rule" && node.Kind != "at-rule" {
				visit(node.Nodes, path)
				continue
			}
			path := append(slices.Clone(path), node)
			if len(node.Nodes) > 0 {
				visit(node.Nodes, path)
				continue
			}
			slices.SortStableFunc(path, func(a, b *Node) int { return boolInt(b.Kind == "at-rule") - boolInt(a.Kind == "at-rule") })
			var parts []string
			for _, parent := range path {
				if parent.Kind == "at-rule" {
					parts = append(parts, parent.Name+" "+parent.Value)
				} else if parent.Name != "&" {
					parts = append(parts, parent.Name)
				}
			}
			selector := ""
			for i := len(parts) - 1; i >= 0; i-- {
				if selector == "" {
					selector = parts[i]
				} else {
					selector = parts[i] + " { " + selector + " }"
				}
			}
			result = append(result, selector)
		}
	}
	visit(nodes, nil)
	return result
}

// CandidateCSS returns one CSS preview per candidate, without theme declarations,
// stylesheet rules or browser polyfills. Invalid candidates have an empty result.
func (c *Compiler) CandidateCSS(classes []string) []string {
	css, _ := c.candidatePreviews(classes, false)
	return css
}

// CandidateAST returns the same previews as CandidateCSS as editable CSS trees.
// Invalid candidates have no nodes. Each tree is independent of the compiler.
func (c *Compiler) CandidateAST(classes []string) [][]*Node {
	_, nodes := c.candidatePreviews(classes, true)
	return nodes
}

func (c *Compiler) candidatePreviews(classes []string, ast bool) ([]string, [][]*Node) {
	preview := *c
	preview.hasInput = true
	preview.keyframes = nil
	preview.config.DisablePolyfills = PolyfillAll
	preview.config.ExistingClasses = nil
	preview.excludedCandidates = nil
	var css []string
	var trees [][]*Node
	if ast {
		trees = make([][]*Node, len(classes))
	} else {
		css = make([]string, len(classes))
	}
	for i, class := range classes {
		b := buildContext{compiler: &preview, used: map[string]bool{}}
		rules := b.compileRules([]string{class})
		var nodes []*Node
		for _, rule := range rules {
			nodes = append(nodes, rule.node)
		}
		if ast {
			b.outputAST = &trees[i]
			b.render(nodes)
			trees[i] = cloneNodes(trees[i], "", false)
		} else {
			css[i] = b.render(nodes)
		}
	}
	return css, trees
}
