package rocketwind

import (
	"slices"
	"strings"
)

func (c *Compiler) registerLegacyScreens(screens ConfigObject) {
	coreOrder := builtinVariants["min"].Order
	type screen struct {
		name, query string
		simple      bool
	}
	var pending []screen
	for _, entry := range screens {
		key := "--breakpoint-" + entry.Key
		if c.config.Theme[key] != "" && c.themeOptionBits(key)&4 == 0 {
			continue
		}
		_, simple := entry.Value.(string)
		pending = append(pending, screen{entry.Key, legacyMediaQuery(entry.Value), simple})
		if !simple {
			c.legacyScreenCount++
		}
	}
	for name, variant := range c.customVariants {
		if variant.Order > coreOrder {
			variant.Order += c.legacyScreenCount
			c.customVariants[name] = variant
		}
	}
	deferred := 0
	for _, screen := range pending {
		order := coreOrder
		if !screen.simple {
			deferred++
			order += deferred
		}
		c.customVariants[screen.name] = variantDefinition{Kind: "static", Order: order, Compounds: 1, Nodes: []*Node{{Kind: "at-rule", Name: "@media", Value: screen.query, Nodes: []*Node{{Kind: "slot"}}}}}
		c.variantOrder = appendUnique(c.variantOrder, screen.name)
	}
}

func legacyMediaQuery(value any) string {
	values, ok := value.([]any)
	if !ok {
		values = []any{value}
	}
	var queries []string
	for _, v := range values {
		object := configObject(v)
		if text, ok := v.(string); ok {
			object = ConfigObject{{"min", text}}
		}
		if object == nil {
			continue
		}
		if raw := object.Get("raw"); raw != nil {
			queries = append(queries, themeString(raw))
			continue
		}
		query := "width"
		if min := object.Get("min"); min != nil {
			query += " >= " + themeString(min)
		}
		if max := object.Get("max"); max != nil {
			query = themeString(max) + " >= " + query
		}
		queries = append(queries, "("+query+")")
	}
	return strings.Join(queries, ", ")
}

func (c *Compiler) registerLegacyContainer(config ConfigObject) {
	if config == nil {
		return
	}
	var nodes []*Node
	if center, ok := config.Get("center").(bool); ok && center {
		nodes = append(nodes, declarations("margin-inline", "auto")...)
	}
	padding := config.Get("padding")
	if value := themeString(padding); value != "" {
		nodes = append(nodes, declarations("padding-inline", value)...)
	} else if value := themeString(configObject(padding).Get("DEFAULT")); value != "" {
		nodes = append(nodes, declarations("padding-inline", value)...)
	}
	screens := configObject(config.Get("screens"))
	var overrides []*Node
	names := map[string]*Node{}
	if screens != nil {
		var breakpoints []NamedValue
		b := buildContext{compiler: c}
		for _, key := range c.themeKeys() {
			if strings.HasPrefix(key, "--breakpoint-") {
				if value := b.themeValue(key); value != "" {
					breakpoints = append(breakpoints, NamedValue{key, value})
				}
			}
		}
		slices.SortStableFunc(breakpoints, func(a, b NamedValue) int { return compareBreakpointValues(a.Value, b.Value, false) })
		if len(breakpoints) > 0 {
			nodes = append(nodes, &Node{Kind: "at-rule", Name: "@media", Value: "(width >= " + breakpoints[0].Value + ")", Nodes: declarations("max-width", "none")})
		}
		for _, entry := range screens {
			value := entry.Value
			if object := configObject(value); object != nil {
				value = object.Get("min")
				if value == nil {
					continue
				}
			}
			text := themeString(value)
			n := &Node{Kind: "at-rule", Name: "@media", Value: "(width >= " + text + ")", Nodes: declarations("max-width", text)}
			overrides = append(overrides, n)
			names[entry.Key] = n
		}
	}
	breakpoints := slices.Clone(configObject(padding))
	b := buildContext{compiler: c}
	slices.SortStableFunc(breakpoints, func(a, z ConfigEntry) int {
		return compareBreakpointValues(b.themeValue("--breakpoint-"+a.Key), b.themeValue("--breakpoint-"+z.Key), false)
	})
	for _, entry := range breakpoints {
		if entry.Key == "DEFAULT" {
			continue
		}
		declaration := declarations("padding-inline", themeString(entry.Value))
		if screens != nil {
			if node := names[entry.Key]; node != nil {
				node.Nodes = append(node.Nodes, declaration...)
			}
		} else {
			nodes = append(nodes, &Node{Kind: "at-rule", Name: "@media", Value: "(width >= theme(--breakpoint-" + entry.Key + "))", Nodes: declaration})
		}
	}
	nodes = append(nodes, overrides...)
	if len(nodes) > 0 {
		(&PluginAPI{compiler: c}).AddUtility("container", nodes)
	}
}
