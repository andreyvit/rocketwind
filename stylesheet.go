package rocketwind

import (
	"fmt"
	"maps"
	"strings"
)

// Compile prepares a stylesheet, its theme and its utility definitions.
func Compile(input string, config Config) (*Compiler, error) {
	if config.Compatibility == Tailwind3 {
		return compileV3(input, config)
	}
	ast, err := ParseCSS(input, config.From)
	if err != nil {
		return nil, err
	}
	features := importFeatures(ast)
	ast, err = resolveImports(ast, config, config.Base, 0)
	if err != nil {
		return nil, err
	}
	config.NoDefaultTheme = true
	c := New(config)
	c.features = features
	c.breakpointVariants = nil
	if c.config.Theme == nil {
		c.config.Theme = make(map[string]string)
	}
	c.input = ast
	c.inputCSS = input
	c.hasInput = true
	c.themeFlags = make(map[string]string)
	if config.From != "" {
		c.themeSources = make(map[string]cssLocation)
	}
	c.custom = make(map[string][]*Node)
	c.customFunctional = make(map[string][][]*Node)
	configuredVariantCount := len(c.variantOrder)
	var modules []moduleRequest
	var walk func([]*Node, bool, bool) ([]*Node, error)
	walk = func(nodes []*Node, nested, reference bool) ([]*Node, error) {
		result := make([]*Node, 0, len(nodes))
		for _, n := range nodes {
			if n.Kind == "at-rule" {
				if n.Name == "@variant" && !nested && (len(n.Nodes) == 0 || hasNode(n.Nodes, "@slot")) {
					n.Name = "@custom-variant"
				}
				switch n.Name {
				case "@source":
					if len(n.Nodes) > 0 {
						return nil, fmt.Errorf("`@source` cannot have a body.")
					}
					if nested {
						return nil, fmt.Errorf("`@source` cannot be nested.")
					}
					if err := c.addSource(n); err != nil {
						return nil, err
					}
					continue
				case "@media":
					var unknown []string
					ref := reference
					for part := range Segments(n.Value, ' ') {
						switch {
						case part == "reference":
							ref = true
						case part == "important":
							c.config.Important = true
						case strings.HasPrefix(part, "theme(") && strings.HasSuffix(part, ")"):
							flags := part[6 : len(part)-1]
							if err := annotateTheme(n.Nodes, flags, strings.Contains(flags, "reference")); err != nil {
								return nil, err
							}
						case strings.HasPrefix(part, "prefix("):
							// Validation is needed only for theme(reference) imports.
							_ = annotateTheme(n.Nodes, part, false)
						case strings.HasPrefix(part, "source("):
							forwardSourceRoot(n.Nodes, part, n.base)
						default:
							unknown = append(unknown, part)
						}
					}
					if len(unknown) == 0 {
						children, err := walk(n.Nodes, nested, ref)
						if err != nil {
							return nil, err
						}
						result = append(result, children...)
						continue
					}
					n.Value = strings.Join(unknown, " ")

				case "@theme":
					c.features |= FeatureTheme
					if reference {
						n.Value += " reference"
					}
					for _, arg := range parseValue(n.Value) {
						if arg.kind == 'f' && arg.value == "prefix" {
							prefix := valueCSS(arg.nodes)
							for _, c := range prefix {
								if c < 'a' || c > 'z' {
									return nil, fmt.Errorf("The prefix %q is invalid. Prefixes must be lowercase ASCII letters (a-z) only.", prefix)
								}
							}
							c.config.Prefix = prefix
						}
					}
					for _, d := range n.Nodes {
						d.Name = Unescape(d.Name)
						if d.Kind == "at-rule" && d.Name == "@keyframes" {
							c.keyframes = append(c.keyframes, d)
							continue
						}
						if d.Kind != "declaration" || !strings.HasPrefix(d.Name, "--") {
							var snippet strings.Builder
							writeCSS(&snippet, []*Node{{Kind: "at-rule", Name: n.Name, Value: n.Value, Nodes: []*Node{d}}}, 0)
							lines := strings.Split(snippet.String(), "\n")
							for i, line := range lines {
								prefix := "> "
								if i == 0 || i >= len(lines)-2 {
									prefix = "  "
								}
								lines[i] = prefix + line
							}
							return nil, fmt.Errorf("`@theme` blocks must only contain custom properties or `@keyframes`.\n\n%s", strings.Join(lines, "\n"))
						}
						if strings.HasSuffix(d.Name, "-*") {
							if d.Value != "initial" {
								return nil, fmt.Errorf("Invalid theme value `%s` for namespace `%s`", d.Value, d.Name)
							}
							prefix := strings.TrimSuffix(d.Name, "*")
							for key := range c.config.Theme {
								if strings.HasPrefix(key, prefix) && !ignoredThemeKey(key, strings.TrimSuffix(prefix, "-")) {
									delete(c.config.Theme, key)
								}
							}
							continue
						}
						_, defined := c.config.Theme[d.Name]
						if strings.Contains(n.Value, "default") && defined && !strings.Contains(c.themeFlags[d.Name], "default") {
							continue
						}
						if d.Value == "initial" {
							delete(c.config.Theme, d.Name)
							continue
						}
						c.config.Theme[d.Name] = d.Value
						if c.themeSources != nil {
							c.themeSources[d.Name] = location(d)
						}
						c.themeFlags[d.Name] = n.Value
						c.themeOrder = append(c.themeOrder, d.Name)
					}
					if !c.themePlaced && !strings.Contains(n.Value, "reference") {
						result = append(result, &Node{Kind: "theme"})
						c.themePlaced = true
						c.themeSource = location(n)
					}
					continue
				case "@tailwind":
					if strings.HasPrefix(n.Value, "utilities") {
						if reference || c.hasUtilities {
							continue
						}
						c.hasUtilities = true
						c.utilitiesSource = location(n)
						c.features |= FeatureUtilities
						base := n.base
						if n.context != nil {
							if sourceBase, ok := n.context["sourceBase"].(string); ok {
								base = sourceBase
							}
						}
						if err := c.setSourceRoot(n.Value, base); err != nil {
							return nil, err
						}

						n = &Node{Kind: "utilities"}
					}
				case "@utility":
					c.features |= directiveFeatures(n.Nodes)
					if nested {
						return nil, fmt.Errorf("`@utility` cannot be nested.")
					}
					if len(n.Nodes) == 0 {
						return nil, fmt.Errorf("`@utility %s` is empty. Utilities should include at least one property.", n.Value)
					}
					name := Unescape(n.Value)
					if strings.HasSuffix(name, "*") && !strings.HasSuffix(name, "-*") {
						return nil, fmt.Errorf("`@utility %s` defines an invalid utility name. A functional utility must end in `-*`.", n.Value)
					}
					if strings.Contains(strings.TrimSuffix(name, "-*"), "-*") {
						return nil, fmt.Errorf("`@utility %s` defines an invalid utility name. The dynamic portion marked by `-*` must appear once at the end.", n.Value)
					}
					if !validUtilityName(name) {
						return nil, fmt.Errorf("`@utility %s` defines an invalid utility name. Utilities should be alphanumeric and start with a lowercase letter.", n.Value)
					}
					if strings.HasSuffix(name, "-*") {
						root := strings.TrimSuffix(name, "-*")
						c.customFunctional[root] = append(c.customFunctional[root], n.Nodes)
					} else {
						c.custom[name] = append(c.custom[name], n.Nodes...)
					}
					c.customOrder = append(c.customOrder, name)
					continue
				case "@custom-variant":
					if nested {
						return nil, fmt.Errorf("`@custom-variant` cannot be nested.")
					}
					if err := c.registerCustomVariant(n); err != nil {
						return nil, err
					}
					continue
				case "@plugin", "@config":
					c.features |= FeaturePlugin
					if nested {
						return nil, fmt.Errorf("`%s` cannot be nested.", n.Name)
					}
					name := strings.Trim(n.Value, "\"'")
					if name == "" {
						return nil, fmt.Errorf("`%s` must have a path.", n.Name)
					}
					modules = append(modules, moduleRequest{node: n, name: name, reference: reference})
					continue
				}
			}
			if len(n.Nodes) > 0 {
				n.Nodes, err = walk(n.Nodes, true, reference)
				if err != nil {
					return nil, err
				}
			}
			if !reference {
				result = append(result, n)
			}
		}
		return result, nil
	}
	c.input, err = walk(ast, false, false)
	if err != nil {
		return nil, err
	}
	cssVariants := maps.Clone(c.customVariants)
	cssVariantOrder := c.variantOrder[configuredVariantCount:]
	c.variantOrder = c.variantOrder[:configuredVariantCount:configuredVariantCount]
	if err := c.loadModules(modules); err != nil {
		return nil, err
	}
	for name, variant := range cssVariants {
		c.customVariants[name] = variant
	}
	c.variantOrder = appendUnique(c.variantOrder, cssVariantOrder...)
	c.input = append(c.input, c.pluginBase...)
	c.pluginBase = nil
	c.config.LoadModule, c.config.LoadStylesheet = nil, nil
	if err := c.resolveVariants(); err != nil {
		return nil, err
	}
	b := buildContext{compiler: c, used: map[string]bool{}}
	for _, key := range c.themeOrder {
		stored, present := c.config.Theme[key]
		if !present {
			continue
		}
		value := parseValue(stored)
		if !b.evaluateFunctions(value, false, 0) {
			return nil, b.err
		}
		c.config.Theme[key] = valueCSS(value)
	}
	c.input = b.evaluateNodes(c.input)
	c.features |= b.features
	if b.err != nil {
		return nil, b.err
	}
	if err := c.resolveApplies(); err != nil {
		return nil, err
	}
	c.prepareBreakpoints()
	return c, nil
}

func annotateTheme(nodes []*Node, flags string, only bool) error {
	for _, n := range nodes {
		if n.Kind == "at-rule" && n.Name == "@theme" {
			n.Value += " " + flags
			continue
		}
		if only && n.Kind != "at-rule" {
			return fmt.Errorf("Files imported with `@import \"…\" theme(reference)` must only contain `@theme` blocks.\nUse `@reference \"…\";` instead.")
		}
		if err := annotateTheme(n.Nodes, flags, only); err != nil {
			return err
		}
	}
	return nil
}
