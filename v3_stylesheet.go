package rocketwind

import (
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

//go:embed data/v3/preflight.css
var v3Preflight string

func v3Ordered(nodes []*Node, order int) []*Node {
	return append([]*Node{{Kind: "v3-order", Value: strconv.Itoa(order)}}, nodes...)
}

func (c *Compiler) nextV3Order() int {
	order := 10000 + c.v3NextOrder
	if c.v3Layer == 1 {
		order = -10000 + c.v3NextOrder
	}
	c.v3NextOrder++
	return order
}

func compileV3(input string, config Config) (*Compiler, error) {
	ast, err := ParseCSS(input, config.From)
	if err != nil {
		return nil, err
	}
	features := importFeatures(ast)
	ast, err = resolveImports(ast, config, config.Base, 0)
	if err != nil {
		return nil, err
	}
	c := New(config)
	c.hasInput, c.inputCSS, c.input = true, input, ast
	c.features = features
	c.root = &SourceRoot{Disabled: true}
	var modules []moduleRequest
	var layers [3][]*Node
	var walk func([]*Node, string) ([]*Node, error)
	walk = func(nodes []*Node, parent string) ([]*Node, error) {
		var result []*Node
		for _, n := range nodes {
			if n.Kind == "at-rule" {
				switch n.Name {
				case "@config", "@plugin":
					modules = append(modules, moduleRequest{node: n, name: unquoteThemePath(n.Value)})
					continue
				case "@layer":
					layer := v3LayerIndex(n.Value)
					if layer >= 0 {
						children, err := walk(n.Nodes, "")
						if err != nil {
							return nil, err
						}
						layers[layer] = append(layers[layer], children...)
						continue
					}
				case "@tailwind":
					layer := v3LayerIndex(n.Value)
					if layer >= 0 || n.Value == "variants" {
						n.Kind = "v3-layer"
						c.features |= FeatureUtilities
						if layer == 2 || layer == 1 {
							c.hasUtilities = true
						}
					}
				case "@apply":
					c.features |= FeatureApply
					if strings.HasPrefix(parent, "@") {
						name, value, _ := strings.Cut(parent, " ")
						if name == "@screen" {
							return nil, fmt.Errorf("@apply is not supported within nested at-rules like @screen. We suggest you write this as @apply %s:%s instead.", value, n.Value)
						}
						return nil, fmt.Errorf("@apply is not supported within nested at-rules like %s", name)
					}
					if raw, ok := strings.CutSuffix(n.Value, " !important"); ok {
						classes := strings.Fields(raw)
						for i, class := range classes {
							parts := appendSegments(nil, class, ':')
							parts[len(parts)-1] = "!" + parts[len(parts)-1]
							classes[i] = strings.Join(parts, ":")
						}
						n.Value = strings.Join(classes, " ")
					}
				}
			}
			if n.Nodes != nil {
				n.Nodes, err = walk(n.Nodes, n.Name+" "+n.Value)
				if err != nil {
					return nil, err
				}
			}
			result = append(result, n)
		}
		return result, nil
	}
	c.input, err = walk(c.input, "")
	if err != nil {
		return nil, err
	}
	if err := c.loadModules(modules); err != nil {
		return nil, err
	}
	var expand func([]*Node) ([]*Node, error)
	expand = func(nodes []*Node) ([]*Node, error) {
		var result []*Node
		for _, n := range nodes {
			if n.Kind == "v3-layer" && n.Value == "base" {
				base, err := ParseCSS(v3Preflight, "")
				if err != nil {
					return nil, err
				}
				result = append(result, c.v3BaseDefaults()...)
				result = append(result, &Node{Kind: "comment", Value: "\n! tailwindcss v3.4.17 | MIT License | https://tailwindcss.com\n"})
				result = append(result, base...)
				for _, layer := range c.pluginBase {
					result = append(result, layer.Nodes...)
				}
				result = append(result, layers[0]...)
				continue
			}
			if n.Name == "@screen" {
				query := v3ScreenQuery(configObject(c.themeLookup("screens")).Get(n.Value))
				if query == "" {
					return nil, fmt.Errorf("No `%s` screen found.", n.Value)
				}
				n.Name, n.Value = "@media", query
			}
			if n.Nodes != nil {
				n.Nodes, err = expand(n.Nodes)
				if err != nil {
					return nil, err
				}
			}
			result = append(result, n)
		}
		return result, nil
	}
	for i := range layers {
		layers[i], err = expand(layers[i])
		if err != nil {
			return nil, err
		}
	}
	for layer, nodes := range layers {
		if layer == 0 {
			continue
		}
		c.v3Layer = layer
		for _, n := range nodes {
			c.registerV3Rule(n, nil, false)
		}
	}
	c.v3Layer = 2
	c.input, err = expand(c.input)
	if err != nil {
		return nil, err
	}
	b := buildContext{compiler: c, used: map[string]bool{}}
	c.input = b.evaluateNodes(c.input)
	for i := range c.v3AlwaysRules {
		c.v3AlwaysRules[i].node = b.evaluateNodes([]*Node{c.v3AlwaysRules[i].node})[0]
	}
	if b.err != nil {
		return nil, b.err
	}
	if err := c.resolveApplies(); err != nil {
		return nil, err
	}
	c.features |= b.features
	return c, nil
}

func v3LayerIndex(layer string) int {
	switch layer {
	case "base":
		return 0
	case "components":
		return 1
	case "utilities":
		return 2
	}
	return -1
}

func (c *Compiler) registerV3Rule(node *Node, wrappers []*Node, prefix bool) {
	if node.Kind == "at-rule" {
		for _, child := range node.Nodes {
			c.registerV3Rule(child, append(slices.Clone(wrappers), node), prefix)
		}
		return
	}
	selector := parseSelectorMode(node.Name, true)
	classes := map[string]bool{}
	var visit func([]*selectorNode)
	visit = func(nodes []*selectorNode) {
		for _, n := range nodes {
			if n.Kind == "function" && containsWord(":not :has", n.Value) {
				continue
			}
			if n.Kind == "selector" && strings.HasPrefix(n.Value, ".") {
				classes[Unescape(n.Value[1:])] = true
			}
			visit(n.Nodes)
		}
	}
	visit(selector)
	if len(classes) == 0 {
		for i := len(wrappers) - 1; i >= 0; i-- {
			copy := *wrappers[i]
			copy.Nodes = []*Node{node}
			node = &copy
		}
		c.v3AlwaysRules = append(c.v3AlwaysRules, compiledRule{node: node, order: []int{c.nextV3Order()}})
		return
	}
	for _, name := range slices.Sorted(maps.Keys(classes)) {
		tree := parseSelectorMode(node.Name, true)
		var replace func([]*selectorNode)
		replace = func(nodes []*selectorNode) {
			for _, n := range nodes {
				if n.Kind == "selector" && strings.HasPrefix(n.Value, ".") && Unescape(n.Value[1:]) == name {
					n.Value = "&"
				} else if prefix && n.Kind == "selector" && strings.HasPrefix(n.Value, ".") {
					n.Value = "." + v3Escape(c.v3ConfiguredPrefix()+Unescape(n.Value[1:]))
				}
				replace(n.Nodes)
			}
		}
		replace(tree)
		nodes := cloneNodes(node.Nodes, "", false)
		if selector := selectorCSS(tree); selector != "&" {
			nodes = []*Node{{Kind: "v3-rule", Name: selector, Nodes: nodes}}
		}
		for i := len(wrappers) - 1; i >= 0; i-- {
			copy := *wrappers[i]
			copy.Nodes = nodes
			nodes = []*Node{&copy}
		}
		(&PluginAPI{compiler: c}).AddUtility(name, nodes)
	}
}

func (c *Compiler) v3BaseDefaults() []*Node {
	var declarations []*Node
	positions := map[string]int{}
	for i, n := range v3Defaults {
		positions[n.Name] = i
	}
	for i, n := range v3Defaults {
		if positions[n.Name] != i {
			continue
		}
		copy := *n
		declarations = append(declarations, &copy)
	}
	for _, n := range declarations {
		switch n.Name {
		case "--tw-ring-offset-width":
			n.Value = themeString(c.themeLookup("ringOffsetWidth.DEFAULT", "0px"))
		case "--tw-ring-offset-color":
			n.Value = themeString(c.themeLookup("ringOffsetColor.DEFAULT", "#fff"))
		case "--tw-ring-color":
			n.Value = v3WithAlpha(themeString(c.themeLookup("ringColor.DEFAULT")), themeString(c.themeLookup("ringOpacity.DEFAULT", "0.5")), "rgb(147 197 253 / 0.5)")
		}
	}
	return []*Node{{Kind: "rule", Name: "*, ::before, ::after", Nodes: declarations}, {Kind: "rule", Name: "::backdrop", Nodes: cloneNodes(declarations, "", false)}}
}

func (b *buildContext) renderV3Rules(rules []compiledRule) string {
	c := b.compiler
	var components, utilities, variants []*Node
	if len(c.v3AlwaysRules) > 0 {
		rules = append(slices.Clone(rules), c.v3AlwaysRules...)
		slices.SortStableFunc(rules, b.compareRules)
	}
	for _, rule := range rules {
		if len(rule.variants) > 0 {
			variants = append(variants, rule.node)
		} else if len(rule.order) > 0 && rule.order[0] < 0 {
			components = append(components, rule.node)
		} else {
			utilities = append(utilities, rule.node)
		}
	}
	if !c.hasInput {
		return b.renderV3(append(append(components, utilities...), variants...))
	}
	placedVariants := false
	var expand func([]*Node) []*Node
	expand = func(nodes []*Node) []*Node {
		var result []*Node
		for _, n := range nodes {
			if n.Kind == "v3-layer" {
				if n.Value == "components" {
					result = append(result, components...)
				} else if n.Value == "utilities" {
					result = append(result, utilities...)
				} else if n.Value == "variants" {
					result = append(result, variants...)
					placedVariants = true
				}
				continue
			}
			copy := *n
			if n.Nodes != nil {
				copy.Nodes = expand(n.Nodes)
			}
			result = append(result, &copy)
		}
		return result
	}
	nodes := expand(c.input)
	if !placedVariants {
		nodes = append(nodes, variants...)
	}
	return b.renderV3(nodes)
}
