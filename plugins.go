package rocketwind

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// NamedValue preserves registration order for plugin values.
type NamedValue struct{ Name, Value string }

// Variant defines a Go variant matcher. An empty selector rejects the value.
// Named values are accepted only when listed; arbitrary values go to Match directly.
type Variant struct {
	Name   string
	Values []NamedValue
	Match  func(value, modifier string) string
	// MatchAll can return parallel selector or at-rule formats. When supplied,
	// it takes precedence over Match. An empty result rejects the value.
	MatchAll func(value, modifier string) []string
	Sort     func(a, b VariantValue) int
}

// VariantValue is a resolved value and optional modifier passed to a variant's
// sort function. Return a negative number to put a before b, zero for a tie.
type VariantValue struct{ Value, Modifier string }

// Plugin registers Go equivalents of stylesheet plugins.
type Plugin func(*PluginAPI) error

type PluginAPI struct {
	compiler  *Compiler
	reference bool
	source    cssLocation
}

// Prefix preserves Tailwind 4's legacy plugin helper, which returns its input.
// The compiler adds configured prefixes when it emits registered utilities.
func (api *PluginAPI) Prefix(class string) string { return class }

// Module is a Go replacement for a configuration file or JavaScript plugin.
// Theme values override defaults but leave explicit stylesheet theme values intact.
type Module struct {
	// Base is the loaded module's directory, used for relative content paths.
	// An empty Base uses the requesting stylesheet's directory.
	Base      string
	Theme     map[string]string
	Legacy    ConfigObject
	Plugins   []Plugin
	Plugin    Plugin
	Configure func(options map[string]any) (Plugin, error)
}

type ModuleLoader func(id, base, kind string) (Module, error)

type moduleRequest struct {
	node      *Node
	name      string
	reference bool
}

func (c *Compiler) loadModules(requests []moduleRequest) error {
	type pendingPlugin struct {
		fn        Plugin
		reference bool
		source    cssLocation
	}
	var pending, configPending []pendingPlugin
	var pluginConfigs, userConfigs []legacyConfigSource
	for _, kind := range []string{"@config", "@plugin"} {
		for _, request := range requests {
			n := request.node
			if n.Name != kind {
				continue
			}
			options := map[string]any{}
			for _, d := range n.Nodes {
				if d.Kind != "declaration" {
					var snippet strings.Builder
					writeCSS(&snippet, []*Node{d}, 0)
					return fmt.Errorf("Unexpected `@plugin` option:\n\n%s\n\n`@plugin` options must be a flat list of declarations.", snippet.String())
				}
				if strings.HasPrefix(d.Value, "{") {
					return fmt.Errorf("Unexpected `@plugin` option: Value of declaration `%s: %s;` is not supported.\n\nUsing an object as a plugin option is currently only supported in JavaScript configuration files.", d.Name, d.Value)
				}
				var values []any
				for value := range Segments(d.Value, ',') {
					values = append(values, pluginOption(strings.TrimSpace(value)))
				}
				if len(values) == 1 {
					options[d.Name] = values[0]
				} else {
					options[d.Name] = values
				}
			}
			module := Module{Plugin: c.config.Plugins[request.name]}
			if module.Plugin == nil {
				if c.config.LoadModule != nil {
					var err error
					module, err = c.config.LoadModule(request.name, n.base, kind[1:])
					if err != nil {
						return err
					}
				} else {
					var ok bool
					module, ok = standardModule(request.name)
					if !ok || kind != "@plugin" {
						return fmt.Errorf("%s requires a Go configuration or plugin", kind)
					}
				}
			}
			for key, value := range module.Theme {
				if _, ok := c.config.Theme[key]; ok && !strings.Contains(c.themeFlags[key], "default") {
					continue
				}
				c.config.Theme[key], c.themeFlags[key] = value, "inline reference"
				c.themeOrder = append(c.themeOrder, key)
			}
			if module.Legacy != nil {
				base := module.Base
				if base == "" {
					base = n.base
				}
				source := legacyConfigSource{module.Legacy, base}
				if kind == "@plugin" {
					pluginConfigs = append(pluginConfigs, source)
				} else {
					userConfigs = append(userConfigs, source)
				}
			}
			plugin := module.Plugin
			if module.Configure != nil {
				var err error
				plugin, err = module.Configure(options)
				if err != nil {
					return err
				}
			} else if len(options) > 0 {
				return fmt.Errorf("The plugin %q does not accept options", request.name)
			}
			for _, fn := range module.Plugins {
				p := pendingPlugin{fn, request.reference, location(n)}
				if kind == "@config" {
					configPending = append(configPending, p)
				} else {
					pending = append(pending, p)
				}
			}
			if plugin != nil {
				p := pendingPlugin{plugin, request.reference, location(n)}
				if kind == "@config" {
					configPending = append(configPending, p)
				} else {
					pending = append(pending, p)
				}
			}
		}
	}
	pending = append(pending, configPending...)
	sources := append(pluginConfigs, userConfigs...)
	if len(sources) == 0 && len(pending) > 0 {
		sources = []legacyConfigSource{{}}
	}
	if err := c.configureLegacy(sources); err != nil {
		return err
	}
	if c.legacyConfig != nil {
		var plugins []any
		for _, p := range pending {
			plugins = append(plugins, ConfigObject{{"handler", p.fn}, {"reference", p.reference}, {"src", nil}})
		}
		plugins = append(plugins, ConfigObject{{"handler", Plugin(func(api *PluginAPI) error {
			api.compiler.configureDarkMode(api.compiler.legacyConfig.Get("darkMode"))
			return nil
		})}, {"reference", true}, {"src", nil}})
		c.legacyConfig.set("plugins", plugins)
	}
	for _, p := range pending {
		if err := p.fn(&PluginAPI{compiler: c, reference: p.reference, source: p.source}); err != nil {
			return err
		}
	}
	if err := c.resolveAllLegacyTheme(); err != nil {
		return err
	}
	return c.finishLegacy()
}

func pluginOption(value string) any {
	if len(value) > 1 && (value[0] == '"' || value[0] == '\'') {
		return value[1 : len(value)-1]
	}
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	if value == "null" {
		return nil
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number
	}
	return value
}

// AddBase appends styles to the base layer, unless the plugin is only referenced.
func (api *PluginAPI) AddBase(nodes []*Node) {
	if !api.reference {
		base := &Node{Kind: "at-rule", Name: "@layer", Value: "base", Nodes: cloneNodes(nodes, "", false)}
		setLocations([]*Node{base}, api.source, true)
		api.compiler.pluginBase = append(api.compiler.pluginBase, base)
	}
}

// AddUtility defines a named utility and its declarations or nested rules.
func (api *PluginAPI) AddUtility(name string, nodes []*Node) {
	c := api.compiler
	if c.custom == nil {
		c.custom = make(map[string][]*Node)
	}
	nodes = cloneNodes(nodes, "", false)
	if c.customGroups == nil {
		c.customGroups = map[string][]staticUtilityGroup{}
	}
	if len(c.customGroups[name]) == 0 && len(c.custom[name]) > 0 {
		// Keep stylesheet utilities when a Go plugin adds the same name.
		c.customGroups[name] = append(c.customGroups[name], staticUtilityGroup{nodes: c.custom[name]})
	}
	c.customGroups[name] = append(c.customGroups[name], staticUtilityGroup{nodes: nodes})
	c.custom[name] = append(c.custom[name], nodes...)
	c.customOrder = append(c.customOrder, name)
}

// AddVariant registers a selector list, at-rule, or a body containing @slot.
func (api *PluginAPI) AddVariant(name string, template []*Node) error {
	if hasMergeSelector(template) {
		return nil
	}
	if len(template) == 0 && validVariantName(name) {
		api.compiler.registerVariantTemplate(name, nil)
		return nil
	}
	return api.compiler.registerCustomVariant(&Node{Kind: "at-rule", Name: "@custom-variant", Value: name, Nodes: cloneNodes(template, "", false)})
}

// AddVariantFormats registers the string forms accepted by Tailwind plugins,
// including at-rules, parallel selectors and formats with nested placeholders.
func (api *PluginAPI) AddVariantFormats(name string, formats ...string) error {
	for _, format := range formats {
		if strings.Contains(format, ":merge(") {
			return nil
		}
	}
	template, err := pluginVariantFormats(formats, []*Node{{Kind: "slot"}})
	if err != nil {
		return err
	}
	return api.AddVariant(name, template)
}

func hasMergeSelector(nodes []*Node) bool {
	for _, n := range nodes {
		if strings.Contains(n.Name, ":merge(") || hasMergeSelector(n.Nodes) {
			return true
		}
	}
	return false
}

func pluginVariantFormats(formats []string, nodes []*Node) ([]*Node, error) {
	var result []*Node
	for _, format := range formats {
		if format == "" {
			continue
		}
		scoped := strings.Contains(format, "@scope")
		content := nodes
		if scoped {
			content = []*Node{{Kind: "context", context: map[string]any{"source": "user"}, Nodes: nodes}}
		}
		var rules []*Node
		if strings.HasSuffix(strings.TrimSpace(format), "}") {
			var err error
			rules, err = ParseCSS(strings.Replace(format, "}", "{@slot}}", 1), "")
			if err != nil {
				return nil, err
			}
			rules = slotNodes(rules, content)
		} else {
			n := &Node{Kind: "rule", Name: format, Nodes: content}
			if strings.HasPrefix(format, "@") {
				n.Kind = "at-rule"
				n.Name, n.Value, _ = strings.Cut(format, " ")
				n.Value = strings.TrimSpace(n.Value)
				n.variant = true
			}
			rules = []*Node{n}
		}
		if scoped {
			rules = []*Node{{Kind: "context", context: map[string]any{"source": "variant"}, Nodes: rules}}
		}
		result = append(result, rules...)
	}
	return result, nil
}

func variantFormats(v *Variant, value, modifier string) []string {
	if v.MatchAll != nil {
		return v.MatchAll(value, modifier)
	}
	if v.Match != nil {
		if selector := v.Match(value, modifier); selector != "" {
			return []string{selector}
		}
	}
	return nil
}

// Theme returns the resolved value of a CSS variable or legacy theme path.
func (api *PluginAPI) Theme(path string) string {
	if api.compiler.legacyTheme != nil {
		return themeString(api.ThemeValue(path))
	}
	b := buildContext{compiler: api.compiler, used: map[string]bool{}}
	value, _ := b.evaluateFunction("theme", []string{path}, false)
	return value
}

func (api *PluginAPI) MatchVariant(variant Variant) { api.compiler.addVariant(variant) }

func (c *Compiler) addVariant(v Variant) {
	for _, format := range variantFormats(&v, "a", "") {
		if strings.Contains(format, ":merge(") {
			return
		}
	}
	v.Values = slices.Clone(v.Values)
	c.matchingVariants[v.Name] = &v
	order := 100 + len(c.customVariants)
	if existing, ok := c.variantDefinition(v.Name); ok {
		order = existing.Order
	}
	c.customVariants[v.Name] = variantDefinition{Kind: "functional", Order: order, Compounds: 2}
	c.variantOrder = appendUnique(c.variantOrder, v.Name)
}

func variantCompounds(nodes []*Node) int {
	bits := 0
	for _, n := range nodes {
		if n.Kind == "rule" {
			bits |= 2
		} else if n.Kind == "at-rule" && n.Name != "@slot" {
			if n.Name != "@media" && n.Name != "@supports" && n.Name != "@container" {
				return 0
			}
			bits |= 1
		}
		bits |= variantCompounds(n.Nodes)
	}
	return bits
}
