// CSS nesting follows Tailwind CSS v4.3.4 under the MIT license.
package rocketwind

import "strings"

type selectorFrame struct {
	selector string
	dead     bool
}

func flattenRules(input []*Node, parent string) []*Node {
	var result []*Node
	var selectors []selectorFrame
	if parent != "" {
		selectors = append(selectors, selectorFrame{selector: parent})
	}
	var atRules []*Node
	var source string
	var group *[]*Node
	seenProperties := map[string]bool{}
	dedupe := map[*[]*Node]bool{}
	scopes := map[*Node]bool{}
	var scan func([]*Node) bool
	scan = func(nodes []*Node) bool {
		found := false
		for _, n := range nodes {
			if scan(n.Nodes) || n.Name == "@scope" {
				scopes[n] = true
				found = true
			}
		}
		return found
	}
	scan(input)
	emit := func(node *Node) {
		if group != nil {
			if node.Kind == "declaration" {
				if seenProperties[node.Name] {
					dedupe[group] = true
				} else {
					seenProperties[node.Name] = true
				}
			}
			*group = append(*group, node)
			return
		}
		if len(selectors) == 0 && len(atRules) == 0 {
			if len(result) > 0 {
				last := result[len(result)-1]
				if last.Kind == "at-rule" && node.Kind == "at-rule" && len(last.Nodes) == 0 && len(node.Nodes) == 0 && last.Name == node.Name && last.Value == node.Value {
					return
				}
			}
			result = append(result, node)
			return
		}
		clear(seenProperties)
		if node.Kind == "declaration" {
			seenProperties[node.Name] = true
		}
		target := &result
		offset := 0
		for _, at := range atRules {
			if len(*target) == 0 {
				break
			}
			last := (*target)[len(*target)-1]
			if last.Kind != "at-rule" || last.Name != at.Name || last.Value != at.Value {
				break
			}
			offset++
			target = &last.Nodes
		}
		var root *Node
		if len(selectors) > 0 {
			selector := selectors[len(selectors)-1].selector
			if offset == len(atRules) && len(*target) > 0 {
				last := (*target)[len(*target)-1]
				if last.Kind == "rule" && last.Name == selector {
					last.Nodes = append(last.Nodes, node)
					group = &last.Nodes
					dedupe[group] = true
					return
				}
			}
			root = &Node{Kind: "rule", Name: selector, Nodes: []*Node{node}}
			group = &root.Nodes
		}
		for i := len(atRules) - 1; i >= offset; i-- {
			wrapper := *atRules[i]
			if root != nil {
				wrapper.Nodes = []*Node{root}
			} else {
				wrapper.Nodes = []*Node{node}
				group = &wrapper.Nodes
			}
			root = &wrapper
		}
		if root != nil {
			*target = append(*target, root)
		} else {
			*target = append(*target, node)
			group = target
		}
	}
	var walk func(*Node)
	walk = func(node *Node) {
		switch node.Kind {
		case "rule":
			group = nil
			if len(selectors) > 0 && node.Name == "&" {
				for _, child := range node.Nodes {
					walk(child)
				}
				return
			}
			selector := node.Name
			dead := false
			if len(selectors) == 0 {
				if strings.Contains(selector, "&") {
					replacement := ":scope"
					for _, at := range atRules {
						if at.Name == "@scope" {
							replacement = ":where(:scope)"
							break
						}
					}
					nodes := parseSelector(selector)
					if replaceNesting(nodes, replacement) {
						selector = selectorCSS(nodes)
					}
				}
			} else {
				var all, live []string
				for part := range Segments(selector, ',') {
					resolved := nestSelector(part, selectors[len(selectors)-1].selector)
					all = append(all, resolved)
					if !deadSelector(resolved) {
						live = append(live, resolved)
					}
				}
				if len(live) == 0 {
					dead = true
					selector = strings.Join(all, ", ")
				} else {
					selector = strings.Join(live, ", ")
				}
			}
			selectors = append(selectors, selectorFrame{selector, dead})
			fast := false
			if !dead && !scopes[node] {
				for _, child := range node.Nodes {
					if child.Kind == "declaration" {
						fast = true
						break
					}
				}
			}
			for _, child := range node.Nodes {
				if fast {
					emit(child)
				} else {
					walk(child)
				}
			}
			group = nil
			selectors = selectors[:len(selectors)-1]
		case "at-rule":
			group = nil
			if len(selectors) > 0 && selectors[len(selectors)-1].dead {
				if node.Name == "@layer" && len(node.Nodes) == 0 {
					saved := selectors
					selectors = nil
					emit(node)
					selectors = saved
					return
				}
				if len(node.Nodes) == 0 || !hoistable(node.Name) {
					return
				}
			}
			if len(node.Nodes) == 0 && !droppable(node.Name) {
				emit(node)
				return
			}
			if !hoistable(node.Name) {
				emit(node)
				return
			}
			r := *node
			saved := selectors
			if node.Name == "@scope" {
				r.Nodes = wrapScopeDeclarations(node.Nodes)
				variant := node.variant || source == "variant"
				if strings.Contains(r.Value, "&") || !variant && len(selectors) > 0 {
					parent := ""
					if len(selectors) > 0 {
						parent = selectors[len(selectors)-1].selector
					}
					params := parseValue(r.Value)
					end, consumed := false, false
					start := ""
					for _, part := range params {
						if part.kind == 'w' && part.value == "to" {
							end = true
							continue
						}
						if part.kind != 'f' || part.value != "" {
							continue
						}
						selector := valueCSS(part.nodes)
						if !end && !variant && parent != "" {
							var resolved []string
							for item := range Segments(selector, ',') {
								resolved = append(resolved, nestSelector(strings.TrimSpace(item), parent))
							}
							selector = strings.Join(resolved, ", ")
							consumed = true
						} else if strings.Contains(selector, "&") {
							replacement := parent
							if replacement == "" {
								replacement = ":scope"
							}
							if end {
								replacement = ":where(:scope)"
							}
							selector = nestSelector(selector, replacement)
							if !end && parent != "" {
								consumed = true
							}
						}
						part.nodes = []*valueNode{{kind: 'w', value: selector}}
						if !end {
							start = selector
						}
					}
					r.Value = valueCSS(params)
					if consumed {
						if len(selectors) > 0 && selectors[len(selectors)-1].dead && start != "" && deadSelector(start) {
							return
						}
						selectors = nil
					}
				}
			}
			atRules = append(atRules, &r)
			for _, child := range r.Nodes {
				walk(child)
			}
			group = nil
			atRules = atRules[:len(atRules)-1]
			selectors = saved
		case "declaration", "comment":
			if node.Kind == "declaration" && node.Name == "--tw-sort" {
				return
			}
			if len(selectors) > 0 && selectors[len(selectors)-1].dead {
				return
			}
			emit(node)
		case "context":
			old := source
			if value, ok := node.context["source"].(string); ok {
				source = value
			}
			for _, child := range node.Nodes {
				walk(child)
			}
			source = old
		case "at-root":
		}
	}
	for _, n := range input {
		walk(n)
	}
	for nodes := range dedupe {
		seen := map[string]bool{}
		for i := len(*nodes) - 1; i >= 0; i-- {
			n := (*nodes)[i]
			if n.Kind != "declaration" {
				continue
			}
			key := n.Name + "\x00" + n.Value
			if n.Important {
				key += "\x01"
			}
			if seen[key] {
				*nodes = append((*nodes)[:i], (*nodes)[i+1:]...)
			} else {
				seen[key] = true
			}
		}
	}
	return result
}

func wrapScopeDeclarations(nodes []*Node) []*Node {
	var result []*Node
	var group *Node
	for _, n := range nodes {
		if n.Kind == "declaration" || n.Kind == "comment" {
			if group == nil {
				group = &Node{Kind: "rule", Name: "&"}
				result = append(result, group)
			}
			group.Nodes = append(group.Nodes, n)
		} else {
			group = nil
			if n.Kind == "context" {
				r := *n
				r.Nodes = wrapScopeDeclarations(n.Nodes)
				n = &r
			}
			result = append(result, n)
		}
	}
	return result
}

func hoistable(name string) bool {
	return containsWord("@container @layer @media @page @scope @starting-style @supports @view-transition", name)
}
func droppable(name string) bool { return name != "@layer" && hoistable(name) }

func flattenContexts(nodes []*Node, source string) []*Node {
	var result []*Node
	for _, n := range nodes {
		if n.Kind == "context" {
			childSource := source
			if value, ok := n.context["source"].(string); ok {
				childSource = value
			}
			result = append(result, flattenContexts(n.Nodes, childSource)...)
			continue
		}
		r := *n
		if source != "" {
			r.variant = source == "variant"
		}
		if n.Nodes != nil {
			r.Nodes = flattenContexts(n.Nodes, source)
		}
		result = append(result, &r)
	}
	return result
}
