package rocketwind

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Utility defines a functional Go utility. Values and modifiers map names to
// CSS. An empty value name (or DEFAULT) supplies the unqualified utility.
// Compile returns declarations and nested rules; nil rejects the candidate.
type Utility struct {
	Name              string
	Types             []ValueType
	Values, Modifiers []NamedValue
	AllowAnyModifier  bool
	SupportsNegative  bool
	// Bare resolves values outside the named table. The fraction is empty when
	// no slash was supplied. Returning false rejects the candidate.
	Bare    func(value, fraction string) (string, bool)
	Compile func(value, modifier string) []*Node
	// CompileWithError takes precedence over Compile and reports a plugin failure.
	// Canonicalization ignores failed speculative replacements; Build returns it.
	CompileWithError func(value, modifier string) ([]*Node, error)
}

type utilityMatcher struct {
	Utility
	values, modifiers map[string]string
}

// NewWithPlugins configures a compiler entirely from Go. Plugin errors are
// returned before the compiler can be used. No CSS entry point is required.
func NewWithPlugins(config Config, plugins ...Plugin) (*Compiler, error) {
	c := New(config)
	for _, plugin := range plugins {
		if err := plugin(&PluginAPI{compiler: c}); err != nil {
			return nil, err
		}
	}
	if err := c.resolveAllLegacyTheme(); err != nil {
		return nil, err
	}
	if err := c.finishLegacy(); err != nil {
		return nil, err
	}
	c.input = c.pluginBase
	c.pluginBase = nil
	if err := c.resolveVariants(); err != nil {
		return nil, err
	}
	b := buildContext{compiler: c, used: map[string]bool{}}
	c.input = b.evaluateNodes(c.input)
	if b.err != nil {
		return nil, b.err
	}
	if err := c.resolveApplies(); err != nil {
		return nil, err
	}
	c.pluginBase, c.input = c.input, nil
	return c, nil
}

// MatchUtility registers a typed utility matcher, including arbitrary values,
// modifiers, optional negative values and named fractions.
func (api *PluginAPI) MatchUtility(utility Utility) error {
	if !validPluginUtilityName(utility.Name) {
		return fmt.Errorf("`matchUtilities({ '%s' : … })` defines an invalid utility name. Utilities should be alphanumeric and start with a lowercase letter, eg. `scrollbar`.", utility.Name)
	}
	if utility.Compile == nil && utility.CompileWithError == nil {
		return fmt.Errorf("utility %q needs a compile function", utility.Name)
	}
	utility.Types = slices.Clone(utility.Types)
	if len(utility.Types) == 0 {
		utility.Types = []ValueType{TypeAny}
	}
	utility.Values = slices.Clone(utility.Values)
	utility.Modifiers = slices.Clone(utility.Modifiers)
	matcher := &utilityMatcher{Utility: utility, values: map[string]string{}, modifiers: map[string]string{}}
	if slices.Contains(utility.Types, TypeColor) {
		matcher.values = map[string]string{"inherit": "inherit", "transparent": "transparent", "current": "currentcolor"}
	}
	for _, value := range utility.Values {
		name := value.Name
		if name == "DEFAULT" {
			name = ""
		}
		matcher.values[name] = value.Value
	}
	for _, value := range utility.Modifiers {
		matcher.modifiers[value.Name] = value.Value
	}
	c := api.compiler
	if c.matchingUtilities == nil {
		c.matchingUtilities = map[string][]*utilityMatcher{}
	}
	c.matchingUtilities[utility.Name] = append(c.matchingUtilities[utility.Name], matcher)
	if utility.SupportsNegative {
		c.matchingUtilities["-"+utility.Name] = append(c.matchingUtilities["-"+utility.Name], matcher)
		c.functionalOrder = appendUnique(c.functionalOrder, "-"+utility.Name)
	}
	c.functionalOrder = appendUnique(c.functionalOrder, utility.Name)
	return nil
}

func (b *buildContext) matchUtility(m *utilityMatcher, candidate Candidate) ([]*Node, bool) {
	arbitrary := candidate.Value != nil && candidate.Value.Kind == "arbitrary"
	if arbitrary && !slices.Contains(m.Types, TypeAny) {
		kind := ValueType(candidate.Value.DataType)
		if kind != "" && !slices.Contains(m.Types, kind) {
			return nil, false
		}
		if kind == "" {
			found := false
			for _, kind := range m.Types {
				if inferType(candidate.Value.Value, string(kind)) != "" {
					found = true
					break
				}
			}
			if !found {
				return nil, false
			}
		}
	}
	value, found, ignoreModifier := "", false, false
	switch {
	case candidate.Value == nil:
		value, found = m.values[""]
	case arbitrary:
		value, found = candidate.Value.Value, true
	default:
		v := candidate.Value
		if v.Fraction != "" {
			value, found = m.values[v.Fraction]
			ignoreModifier = found
		}
		if !found {
			value, found = m.values[v.Value]
		}
		if !found && m.Bare != nil {
			value, found = m.Bare(v.Value, v.Fraction)
			ignoreModifier = v.Fraction != "" && strings.Contains(value, "/")
		}
	}
	if !found {
		return nil, false
	}
	modifier, validModifier := "", true
	color := slices.Contains(m.Types, TypeColor)
	if mod := candidate.Modifier; mod != nil {
		switch {
		case m.AllowAnyModifier || mod.Kind == "arbitrary":
			modifier = mod.Value
		default:
			modifier, validModifier = m.modifiers[mod.Value]
			if !validModifier && color {
				if _, err := strconv.ParseFloat(mod.Value, 64); err == nil {
					modifier, validModifier = mod.Value+"%", true
				}
			}
		}
		if !validModifier && !ignoreModifier {
			return nil, arbitrary
		}
	}
	if color && modifier != "" {
		value = b.colorAlpha(value, modifier, true)
	}
	if strings.HasPrefix(candidate.Root, "-") {
		value = "calc(" + value + " * -1)"
	}
	var nodes []*Node
	if m.CompileWithError != nil {
		nodes, b.err = m.CompileWithError(value, modifier)
		if b.err != nil {
			return nil, true
		}
	} else {
		nodes = m.Compile(value, modifier)
	}
	if hasNode(nodes, "@apply") || hasNode(nodes, "@variant") {
		if slices.Contains(b.applyStack, candidate.Raw) {
			b.err = fmt.Errorf("You cannot `@apply` the `%s` utility here because it creates a circular dependency.", candidate.Raw)
			return nil, true
		}
		if len(b.applyStack) >= 100 {
			b.err = fmt.Errorf("Plugin utility expansion is too deeply nested")
			return nil, true
		}
		nested := *b
		nested.applyStack = append(slices.Clone(b.applyStack), candidate.Raw)
		nodes = nested.expandPluginDirectives(nodes, false)
		if nested.err != nil {
			b.err = nested.err
			return nil, true
		}
	}
	return replaceUtilityReferences(nodes, m.Name, candidate.Raw), false
}

func (b *buildContext) expandPluginDirectives(nodes []*Node, keyframes bool) []*Node {
	var result []*Node
	for _, node := range nodes {
		if node.Kind == "at-rule" && node.Name == "@apply" {
			if keyframes {
				b.err = fmt.Errorf("You cannot use `@apply` inside `@keyframes`.")
				return nil
			}
			classes := strings.Fields(node.Value)
			important := false
			if len(classes) > 0 && classes[len(classes)-1] == "!important" {
				important = true
				classes = classes[:len(classes)-1]
			}
			if len(node.Nodes) > 0 {
				b.err = fmt.Errorf("The rule `@apply %s` must not have a body.", strings.Join(classes, " "))
				return nil
			}
			nested := *b
			nested.inApply = true
			rules := nested.compileRules(classes)
			if nested.err != nil {
				b.err = nested.err
				return nil
			}
			for _, class := range classes {
				found := false
				for _, rule := range rules {
					if rule.raw == class {
						found = true
						break
					}
				}
				if !found {
					b.err = fmt.Errorf("Cannot apply unknown utility class `%s`", class)
					return nil
				}
			}
			for _, rule := range rules {
				content := rule.node.Nodes
				if important {
					content = cloneNodes(content, "", true)
				}
				result = append(result, content...)
			}
			continue
		}
		copy := *node
		if len(node.Nodes) > 0 {
			copy.Nodes = b.expandPluginDirectives(node.Nodes, keyframes || node.Name == "@keyframes")
		}
		if node.Kind == "at-rule" && node.Name == "@variant" {
			for group := range Segments(node.Value, ',') {
				variants := slices.Collect(Segments(strings.TrimSpace(group), ':'))
				content := copy.Nodes
				for i := len(variants) - 1; i >= 0; i-- {
					variant := b.parseVariant(variants[i])
					if variant == nil {
						b.err = fmt.Errorf("Cannot apply unknown variant `%s`.", variants[i])
						return nil
					}
					var ok bool
					content, ok = b.applyParsedVariant(content, variant, 0)
					if !ok {
						b.err = fmt.Errorf("Cannot apply variant `%s`.", variants[i])
						return nil
					}
				}
				result = append(result, content...)
			}
			continue
		}
		result = append(result, &copy)
	}
	return result
}

// AddUtilities registers complete selector rules, splitting selector lists and
// preserving pseudo-classes and descendant rules. AddComponents is identical.
func (api *PluginAPI) AddUtilities(nodes []*Node) error {
	definitions := map[string][]*Node{}
	var order []string
	for _, node := range nodes {
		if node.Kind == "at-rule" && node.Name == "@keyframes" {
			if !api.reference {
				api.compiler.pluginBase = append(api.compiler.pluginBase, cloneNode(node, "", false))
			}
			continue
		}
		for name := range Segments(node.Name, ',') {
			name = strings.TrimSpace(name)
			selector := parseSelector(name)
			found := false
			var walk func([]*selectorNode)
			walk = func(nodes []*selectorNode) {
				for _, part := range nodes {
					if part.Kind == "function" && containsWord(":not :nth-child :nth-last-child", part.Value) {
						continue
					}
					if part.Kind == "selector" && strings.HasPrefix(part.Value, ".") && validPluginUtilityName(part.Value[1:]) {
						class := part.Value[1:]
						part.Value = "&"
						value := selectorCSS(selector)
						part.Value = "." + class
						content := cloneNodes(node.Nodes, "", false)
						if value != "&" {
							content = []*Node{{Kind: "rule", Name: value, Nodes: content}}
						}
						if _, exists := definitions[class]; !exists {
							order = append(order, class)
						}
						definitions[class] = append(definitions[class], content...)
						found = true
					}
					walk(part.Nodes)
				}
			}
			walk(selector)
			if !found {
				return fmt.Errorf("`addUtilities({ '%s' : … })` defines an invalid utility selector. Utilities must be a single class name and start with a lowercase letter, eg. `.scrollbar-none`.", name)
			}
		}
	}
	c := api.compiler
	if c.customReferences == nil {
		c.customReferences = map[string]bool{}
	}
	for _, name := range order {
		nodes := definitions[name]
		if c.config.Prefix != "" {
			nodes = transformSelectors(nodes, func(part *selectorNode) {
				if part.Kind == "selector" && strings.HasPrefix(part.Value, ".") {
					part.Value = "." + Escape(c.config.Prefix+":") + part.Value[1:]
				}
			})
		}
		api.AddUtility(name, nodes)
		c.customReferences[name] = true
	}
	return nil
}

func (api *PluginAPI) AddComponents(nodes []*Node) error { return api.AddUtilities(nodes) }

func validPluginUtilityName(name string) bool {
	if name == "" || !(name[0] >= 'a' && name[0] <= 'z' || name[0] == '@') {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !asciiLetter(c) && !digit(c) && !strings.ContainsRune("/%._-", rune(c)) {
			return false
		}
	}
	return true
}

func replaceUtilityReferences(nodes []*Node, root, raw string) []*Node {
	return transformSelectors(nodes, func(part *selectorNode) {
		if part.Kind == "selector" && part.Value == "."+root {
			part.Value = "." + Escape(raw)
		}
	})
}

func transformSelectors(nodes []*Node, transform func(*selectorNode)) []*Node {
	result := make([]*Node, len(nodes))
	for i, n := range nodes {
		r := *n
		if n.Kind == "rule" {
			selector := parseSelector(n.Name)
			var walk func([]*selectorNode)
			walk = func(nodes []*selectorNode) {
				for _, node := range nodes {
					transform(node)
					walk(node.Nodes)
				}
			}
			walk(selector)
			r.Name = selectorCSS(selector)
		}
		if n.Nodes != nil {
			r.Nodes = transformSelectors(n.Nodes, transform)
		}
		// Context maps are private but can be supplied by internal Go plugins.
		r.context = maps.Clone(n.context)
		result[i] = &r
	}
	return result
}
