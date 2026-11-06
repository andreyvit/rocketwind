package rocketwind

import (
	"fmt"
	"slices"
	"strings"
)

func (c *Compiler) resolveApplies() error {
	b := buildContext{compiler: c, used: map[string]bool{}, inApply: true}
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
		if err := c.transformStaticUtility(name, func(nodes []*Node) ([]*Node, error) { return expand(nodes, false) }, true); err != nil {
			return err
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
				var offsets map[string]int
				if n.source != nil {
					offsets = make(map[string]int, len(candidates))
					cursor := 0
					for _, raw := range candidates {
						offset := cursor + strings.Index(n.Value[cursor:], raw)
						offsets[raw] = offset
						cursor = offset + len(raw)
					}
				}
				var normal, mixins []string
				for _, raw := range candidates {
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
				if b.err != nil {
					err := b.err
					b.err = nil
					return nil, err
				}
				for _, raw := range normal {
					found := false
					for _, r := range rules {
						if r.raw == raw {
							found = true
							break
						}
					}
					if !found {
						if c.blocklist[raw] {
							return nil, fmt.Errorf("Cannot apply utility class `%s` because it has been explicitly disabled: https://tailwindcss.com/docs/detecting-classes-in-source-files#explicitly-excluding-classes", raw)
						}
						_, variants, _, _ := parseClass(raw, c.config.Prefix)
						var unknown []string
						for _, variant := range variants {
							if b.parseVariant(variant) == nil {
								unknown = append(unknown, "`"+variant+"`")
							}
						}
						if len(unknown) > 0 {
							if len(unknown) == 1 {
								return nil, fmt.Errorf("Cannot apply utility class `%s` because the %s variant does not exist.", raw, unknown[0])
							}
							return nil, fmt.Errorf("Cannot apply utility class `%s` because the %s and %s variants do not exist.", raw, strings.Join(unknown[:len(unknown)-1], ", "), unknown[len(unknown)-1])
						}
						if prefix := c.config.Prefix; prefix != "" && !strings.HasPrefix(raw, prefix+":") {
							return nil, fmt.Errorf("Cannot apply unprefixed utility class `%s`. Did you mean `%s:%s`?", raw, prefix, raw)
						}
						if len(c.config.Theme) == 0 {
							return nil, fmt.Errorf("Cannot apply unknown utility class `%s`. Are you using CSS modules or similar and missing `@reference`? https://tailwindcss.com/docs/functions-and-directives#reference-directive", raw)
						}
						return nil, fmt.Errorf("Cannot apply unknown utility class `%s`", raw)
					}
				}
				for _, r := range rules {
					content := r.node.Nodes
					if c.config.Compatibility == Tailwind3 {
						content = v3AppliedNodes(content)
					}
					if n.source != nil {
						content = cloneNodes(content, "", false)
					}
					if n.source != nil {
						src := location(n)
						src.start += 7 + offsets[r.raw]
						src.end = src.start + len(r.raw)
						setLocations(content, src, true)
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
	if hasNode(c.input, "@apply") {
		c.features |= FeatureApply
	}
	c.input, err = expand(c.input, false)
	if err != nil {
		return err
	}
	for i := range c.v3AlwaysRules {
		nodes, err := expand([]*Node{c.v3AlwaysRules[i].node}, false)
		if err != nil {
			return err
		}
		c.v3AlwaysRules[i].node = nodes[0]
	}
	return nil
}
