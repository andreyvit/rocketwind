package rocketwind

import (
	"fmt"
	"strings"
)

// Compile prepares a stylesheet for incremental utility generation.
func Compile(input string, config Config) (*Compiler, error) {
	ast, err := ParseCSS(input, "")
	if err != nil {
		return nil, err
	}
	config.NoDefaultTheme = true
	c := New(config)
	if c.config.Theme == nil {
		c.config.Theme = make(map[string]string)
	}
	c.input = ast
	c.hasInput = true
	c.themeFlags = make(map[string]string)
	c.custom = make(map[string][]*Node)
	c.customFunctional = make(map[string][][]*Node)
	var walk func([]*Node) ([]*Node, error)
	walk = func(nodes []*Node) ([]*Node, error) {
		result := make([]*Node, 0, len(nodes))
		for _, n := range nodes {
			if n.Kind == "at-rule" {
				switch n.Name {
				case "@theme":
					for _, d := range n.Nodes {
						if d.Kind == "at-rule" && d.Name == "@keyframes" {
							c.keyframes = append(c.keyframes, d)
							continue
						}
						if d.Kind != "declaration" || !strings.HasPrefix(d.Name, "--") {
							return nil, fmt.Errorf("`@theme` blocks must only contain custom properties or `@keyframes`.")
						}
						if strings.HasSuffix(d.Name, "-*") {
							if d.Value != "initial" {
								return nil, fmt.Errorf("Invalid theme value `%s` for namespace `%s`", d.Value, d.Name)
							}
							prefix := strings.TrimSuffix(d.Name, "*")
							for key := range c.config.Theme {
								if strings.HasPrefix(key, prefix) {
									delete(c.config.Theme, key)
								}
							}
							continue
						}
						if strings.Contains(n.Value, "default") && c.config.Theme[d.Name] != "" && !strings.Contains(c.themeFlags[d.Name], "default") {
							continue
						}
						c.config.Theme[d.Name] = d.Value
						c.themeFlags[d.Name] = n.Value
						c.themeOrder = append(c.themeOrder, d.Name)
					}
					if !c.themePlaced && !strings.Contains(n.Value, "reference") {
						result = append(result, &Node{Kind: "theme"})
						c.themePlaced = true
					}
					continue
				case "@tailwind":
					if strings.HasPrefix(n.Value, "utilities") {
						n = &Node{Kind: "utilities"}
					}
				case "@utility":
					name := Unescape(n.Value)
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
					name, selector, _ := strings.Cut(n.Value, " ")
					template := n.Nodes
					if selector != "" {
						selector = strings.TrimSpace(selector)
						if len(selector) < 2 || selector[0] != '(' || selector[len(selector)-1] != ')' {
							return nil, fmt.Errorf("Invalid custom variant: %s", n.Value)
						}
						template = nil
						for part := range Segments(selector[1:len(selector)-1], ',') {
							part = strings.TrimSpace(part)
							if strings.HasPrefix(part, "@") {
								name, value, _ := strings.Cut(part, " ")
								template = append(template, &Node{Kind: "at-rule", Name: name, Value: value, Nodes: []*Node{{Kind: "slot"}}})
							} else {
								template = append(template, &Node{Kind: "rule", Name: part, Nodes: []*Node{{Kind: "slot"}}})
							}
						}
					}
					c.customVariants[name] = variantDefinition{Kind: "static", Order: 100 + len(c.customVariants), Compounds: variantCompounds(template), Nodes: template}
					continue
				case "@plugin":
					name := strings.Trim(n.Value, "\"'")
					if plugin := c.config.Plugins[name]; plugin != nil {
						if err := plugin(&PluginAPI{compiler: c}); err != nil {
							return nil, err
						}
						continue
					}
					return nil, fmt.Errorf("%s requires a Go configuration or plugin", n.Name)
				case "@config":
					return nil, fmt.Errorf("%s requires a Go configuration or plugin", n.Name)
				}
			}
			if len(n.Nodes) > 0 {
				n.Nodes, err = walk(n.Nodes)
				if err != nil {
					return nil, err
				}
			}
			result = append(result, n)
		}
		return result, nil
	}
	c.input, err = walk(ast)
	if err != nil {
		return nil, err
	}
	if err := c.resolveApplies(); err != nil {
		return nil, err
	}
	return c, nil
}
