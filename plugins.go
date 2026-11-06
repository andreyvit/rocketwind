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
}

// Plugin registers Go equivalents of stylesheet plugins.
type Plugin func(*PluginAPI) error

type PluginAPI struct {
	compiler  *Compiler
	reference bool
}

// Module is a Go replacement for a configuration file or JavaScript plugin.
// Theme values override defaults but leave explicit stylesheet theme values intact.
type Module struct {
	Theme     map[string]string
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
	}
	var pending []pendingPlugin
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
				pending = append(pending, pendingPlugin{fn, request.reference})
			}
			if plugin != nil {
				pending = append(pending, pendingPlugin{plugin, request.reference})
			}
		}
	}
	for _, p := range pending {
		if err := p.fn(&PluginAPI{compiler: c, reference: p.reference}); err != nil {
			return err
		}
	}
	return nil
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
		api.compiler.pluginBase = append(api.compiler.pluginBase, &Node{Kind: "at-rule", Name: "@layer", Value: "base", Nodes: cloneNodes(nodes, "", false)})
	}
}

// AddUtility defines a named utility and its declarations or nested rules.
func (api *PluginAPI) AddUtility(name string, nodes []*Node) {
	c := api.compiler
	if c.custom == nil {
		c.custom = make(map[string][]*Node)
	}
	c.custom[name] = append(c.custom[name], cloneNodes(nodes, "", false)...)
	c.customOrder = append(c.customOrder, name)
}

// AddVariant registers a selector list, at-rule, or a body containing @slot.
func (api *PluginAPI) AddVariant(name string, template []*Node) error {
	return api.compiler.registerCustomVariant(&Node{Kind: "at-rule", Name: "@custom-variant", Value: name, Nodes: cloneNodes(template, "", false)})
}

// Theme returns the resolved value of a CSS variable or legacy theme path.
func (api *PluginAPI) Theme(path string) string {
	b := buildContext{compiler: api.compiler, used: map[string]bool{}}
	value, _ := b.evaluateFunction("theme", []string{path}, false)
	return value
}

func (api *PluginAPI) MatchVariant(variant Variant) { api.compiler.addVariant(variant) }

func (c *Compiler) addVariant(v Variant) {
	v.Values = slices.Clone(v.Values)
	c.matchingVariants[v.Name] = &v
	c.customVariants[v.Name] = variantDefinition{Kind: "functional", Order: 100 + len(c.customVariants), Compounds: 2}
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
