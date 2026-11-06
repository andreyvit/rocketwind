package rocketwind

import (
	"fmt"
	"slices"
	"strings"
)

func hasNode(nodes []*Node, name string) bool {
	for _, n := range nodes {
		if n.Name == name || hasNode(n.Nodes, name) {
			return true
		}
	}
	return false
}

func validVariantName(name string) bool {
	name = strings.TrimPrefix(name, "@")
	if name == "" || !(name[0] >= 'a' && name[0] <= 'z' || digit(name[0])) {
		return false
	}
	if name[len(name)-1] == '-' || name[len(name)-1] == '_' {
		return false
	}
	for i := range len(name) {
		if !asciiLetter(name[i]) && !digit(name[i]) && name[i] != '-' && name[i] != '_' {
			return false
		}
	}
	return true
}

func (c *Compiler) registerCustomVariant(n *Node) error {
	name, selector, _ := strings.Cut(n.Value, " ")
	if !validVariantName(name) {
		return fmt.Errorf("`@custom-variant %s` defines an invalid variant name. Variants should only contain alphanumeric, dashes, or underscore characters and start with a lowercase letter or number.", name)
	}
	if len(n.Nodes) > 0 && selector != "" {
		return fmt.Errorf("`@custom-variant %s` cannot have both a selector and a body.", name)
	}
	template := n.Nodes
	if len(template) == 0 {
		if selector == "" {
			return fmt.Errorf("`@custom-variant %s` has no selector or body.", name)
		}
		selector = strings.TrimSpace(selector)
		if len(selector) < 2 {
			return fmt.Errorf("`@custom-variant %s %s` selector is invalid.", name, selector)
		}
		parts := slices.Collect(Segments(selector[1:len(selector)-1], ','))
		var styles []string
		var atRules []*Node
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				return fmt.Errorf("`@custom-variant %s %s` selector is invalid.", name, selector)
			}
			if strings.HasPrefix(part, "@") {
				name, value, _ := strings.Cut(part, " ")
				atRules = append(atRules, &Node{Kind: "at-rule", Name: name, Value: value, Nodes: []*Node{{Kind: "slot"}}})
			} else {
				styles = append(styles, part)
			}
		}
		if len(styles) > 0 {
			template = append(template, &Node{Kind: "rule", Name: strings.Join(styles, ", "), Nodes: []*Node{{Kind: "slot"}}})
		}
		template = append(template, atRules...)
	}
	order := 100 + len(c.customVariants)
	if previous, ok := c.variantDefinition(name); ok {
		order = previous.Order
	}
	c.customVariants[name] = variantDefinition{Kind: "static", Order: order, Compounds: variantCompounds(template), Nodes: template}
	c.variantOrder = append(c.variantOrder, name)
	return nil
}

func (c *Compiler) resolveVariants() error {
	if hasNode(c.input, "@variant") {
		c.features |= FeatureVariants
	}
	b := buildContext{compiler: c, used: map[string]bool{}}
	state := map[string]uint8{}
	var path []string
	var expand func([]*Node) ([]*Node, error)
	var visit func(string) error
	visit = func(name string) error {
		if _, ok := c.customVariants[name]; !ok || state[name] == 2 {
			return nil
		}
		if state[name] == 1 {
			var out strings.Builder
			out.WriteString("Circular dependency detected in custom variants:\n\n")
			for i, item := range path {
				fmt.Fprintf(&out, "@custom-variant %s {", item)
				if item == name {
					out.WriteString(" /* ← */")
				}
				next := name
				if i+1 < len(path) {
					next = path[i+1]
				}
				fmt.Fprintf(&out, "\n  @variant %s { … }\n}\n", next)
			}
			return fmt.Errorf("%s", out.String())
		}
		state[name] = 1
		path = append(path, name)
		d := c.customVariants[name]
		if d.Kind == "functional" {
			path = path[:len(path)-1]
			state[name] = 2
			return nil
		}
		nodes, err := expand(d.Nodes)
		if err != nil {
			return err
		}
		d.Nodes, d.Compounds = nodes, variantCompounds(nodes)
		c.customVariants[name] = d
		path = path[:len(path)-1]
		state[name] = 2
		return nil
	}
	expand = func(nodes []*Node) ([]*Node, error) {
		var result []*Node
		for _, n := range nodes {
			r := *n
			var err error
			if n.Nodes != nil {
				r.Nodes, err = expand(n.Nodes)
				if err != nil {
					return nil, err
				}
			}
			if n.Kind != "at-rule" || n.Name != "@variant" {
				result = append(result, &r)
				continue
			}
			for group := range Segments(n.Value, ',') {
				content := r.Nodes
				variants := slices.Collect(Segments(group, ':'))
				for i := len(variants) - 1; i >= 0; i-- {
					name := strings.TrimSpace(variants[i])
					if name == "" {
						return nil, fmt.Errorf("Cannot use `@variant` with empty variant")
					}
					if err := visit(name); err != nil {
						return nil, err
					}
					v := b.parseVariant(name)
					if v == nil {
						return nil, fmt.Errorf("Cannot use `@variant` with unknown variant: %s", name)
					}
					var ok bool
					content, ok = b.applyParsedVariant(content, v, 0)
					if !ok {
						return nil, fmt.Errorf("Cannot use `@variant` with variant: %s", name)
					}
				}
				result = append(result, content...)
			}
		}
		return result, nil
	}
	for _, name := range c.variantOrder {
		if err := visit(name); err != nil {
			return err
		}
	}
	var err error
	c.input, err = expand(c.input)
	if err != nil {
		return err
	}
	for _, name := range c.customOrder {
		if strings.HasSuffix(name, "-*") {
			root := strings.TrimSuffix(name, "-*")
			for i, nodes := range c.customFunctional[root] {
				c.customFunctional[root][i], err = expand(nodes)
				if err != nil {
					return err
				}
			}
		} else {
			c.custom[name], err = expand(c.custom[name])
			if err != nil {
				return err
			}
			for i, nodes := range c.customGroups[name] {
				c.customGroups[name][i], err = expand(nodes)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
