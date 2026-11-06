package rocketwind

import "slices"

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

type PluginAPI struct{ compiler *Compiler }

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
