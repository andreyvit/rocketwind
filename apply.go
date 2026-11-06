package rocketwind

import (
	"fmt"
	"slices"
	"strings"
)

func (c *Compiler) resolveApplies() error {
	b := buildContext{compiler: c, used: map[string]bool{}}
	complete := map[string]bool{}
	var path, edges []string
	var expand func([]*Node, bool) ([]*Node, error)
	var visit func(string, string) error
	visit = func(name, edge string) error {
		if complete[name] {
			return nil
		}
		if i := slices.Index(path, name); i >= 0 {
			candidate := edge
			if i+1 < len(edges) {
				candidate = edges[i+1]
			}
			return fmt.Errorf("You cannot `@apply` the `%s` utility here because it creates a circular dependency.", candidate)
		}
		path = append(path, name)
		edges = append(edges, edge)
		if nodes := c.custom[name]; nodes != nil {
			resolved, err := expand(nodes, false)
			if err != nil {
				return err
			}
			c.custom[name] = resolved
		}
		for i, nodes := range c.customFunctional[name] {
			resolved, err := expand(nodes, false)
			if err != nil {
				return err
			}
			c.customFunctional[name][i] = resolved
		}
		path = path[:len(path)-1]
		edges = edges[:len(edges)-1]
		complete[name] = true
		return nil
	}
	expand = func(nodes []*Node, keyframes bool) ([]*Node, error) {
		var result []*Node
		for _, n := range nodes {
			if n.Kind == "at-rule" && n.Name == "@apply" {
				if keyframes {
					return nil, fmt.Errorf("You cannot use `@apply` inside `@keyframes`.")
				}
				candidates := strings.Fields(n.Value)
				var normal, mixins []string
				important := false
				for _, raw := range candidates {
					if raw == "!important" {
						important = true
						continue
					}
					if strings.HasPrefix(raw, "--") {
						mixins = append(mixins, raw)
					} else {
						normal = append(normal, raw)
					}
				}
				if len(mixins) > 0 {
					if len(normal) > 0 {
						return nil, fmt.Errorf("You cannot use `@apply` with both mixins and utilities. Please move `@apply %s` into a separate rule.", strings.Join(mixins, " "))
					}
					result = append(result, n)
					continue
				}
				if len(n.Nodes) > 0 {
					return nil, fmt.Errorf("The rule `@apply %s` must not have a body.", strings.Join(normal, " "))
				}
				for _, raw := range normal {
					base, _, _, _ := parseClass(raw, c.config.Prefix)
					if c.custom[base] != nil {
						if err := visit(base, raw); err != nil {
							return nil, err
						}
					}
					for end := len(base); end > 0; end = strings.LastIndexByte(base[:end], '-') {
						root := base[:end]
						if c.customFunctional[root] != nil {
							if err := visit(root, raw); err != nil {
								return nil, err
							}
						}
					}
				}
				rules := b.compileRules(normal)
				for _, raw := range normal {
					found := false
					for _, r := range rules {
						if r.raw == raw {
							found = true
							break
						}
					}
					if !found {
						return nil, fmt.Errorf("Cannot apply unknown utility class `%s`", raw)
					}
				}
				for _, r := range rules {
					content := r.node.Nodes
					if important {
						content = cloneNodes(content, "", true)
					}
					result = append(result, content...)
				}
				continue
			}
			r := *n
			if n.Nodes != nil {
				var err error
				r.Nodes, err = expand(n.Nodes, keyframes || n.Name == "@keyframes")
				if err != nil {
					return nil, err
				}
			}
			result = append(result, &r)
		}
		return result, nil
	}
	for _, name := range c.customOrder {
		if err := visit(strings.TrimSuffix(name, "-*"), ""); err != nil {
			return err
		}
	}
	var err error
	c.input, err = expand(c.input, false)
	return err
}
