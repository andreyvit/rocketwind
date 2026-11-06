package rocketwind

import "strings"

func namespaceInternalVariables(nodes []*Node, prefix string) []*Node {
	result := make([]*Node, len(nodes))
	rename := func(name string) string {
		if strings.HasPrefix(name, "--tw-") {
			return "--" + Escape(prefix) + "-" + name[2:]
		}
		return name
	}
	var value func([]*valueNode)
	value = func(nodes []*valueNode) {
		for _, n := range nodes {
			if n.kind == 'f' && n.value == "var" && len(n.nodes) > 0 {
				n.nodes[0].value = rename(n.nodes[0].value)
			}
			value(n.nodes)
		}
	}
	for i, n := range nodes {
		copy := *n
		if n.Kind == "declaration" {
			copy.Name = rename(n.Name)
		}
		if n.Name == "@property" {
			copy.Value = rename(n.Value)
		} else if strings.Contains(n.Value, "--tw-") && strings.Contains(n.Value, "var(") {
			parsed := parseValue(n.Value)
			value(parsed)
			copy.Value = valueCSS(parsed)
		}
		if n.Nodes != nil {
			copy.Nodes = namespaceInternalVariables(n.Nodes, prefix)
		}
		result[i] = &copy
	}
	return result
}
