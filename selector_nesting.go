// Selector composition follows Tailwind CSS v4.3.4. See testdata/upstream/LICENSE.
package rocketwind

import "strings"

func replaceNesting(nodes []*selectorNode, parent string) bool {
	changed := false
	for _, n := range nodes {
		if n.Kind == "selector" && n.Value == "&" {
			n.Value = parent
			changed = true
		}
		if replaceNesting(n.Nodes, parent) {
			changed = true
		}
	}
	return changed
}

func isUniversal(n *selectorNode) bool {
	return n.Kind == "selector" && strings.HasPrefix(n.Value, "*")
}
func isTypeSelector(n *selectorNode) bool {
	return n.Kind == "selector" && (n.Value == "" || !strings.ContainsRune("*&.#:[", rune(n.Value[0])))
}

func nestSelector(selector, parent string) string {
	parentAST := parseSelector(parent)
	list := len(parentAST) == 1 && parentAST[0].Kind == "list"
	if strings.Contains(selector, "&") {
		nodes := parseSelector(selector)
		changed := false
		var walk func([]*selectorNode, *selectorNode, *selectorNode) []*selectorNode
		walk = func(nodes []*selectorNode, container, grandparent *selectorNode) []*selectorNode {
			var result []*selectorNode
			for i, n := range nodes {
				index := len(result)
				if n.Kind == "selector" && n.Value == "&" {
					changed = true
					n.Value = ":is(" + parent + ")"
					if container != nil && !list && len(parentAST) > 0 {
						p := parentAST[0]
						switch container.Kind {
						case "complex":
							if index == 0 || p.Kind != "complex" {
								n.Value = parent
							}
						case "compound":
							keep := p.Kind == "complex" && grandparent != nil && grandparent.Kind == "complex" && len(grandparent.Nodes) > 0 && grandparent.Nodes[0] != container
							for _, sibling := range nodes[i+1:] {
								if isUniversal(sibling) || isTypeSelector(sibling) {
									keep = true
									break
								}
							}
							if !keep && (index == 0 || p.Kind != "complex" && !isUniversal(p) && !isTypeSelector(p)) {
								n.Value = parent
							}
						case "function":
							n.Value = parent
						}
					}
				}
				n.Nodes = walk(n.Nodes, n, container)
				if i == 0 && len(nodes) > 1 && container != nil && container.Kind == "compound" && isUniversal(n) {
					next := nodes[1]
					if next.Kind != "selector" || !strings.HasPrefix(next.Value, "|") {
						continue
					}
				}
				result = append(result, n)
			}
			return result
		}
		nodes = walk(nodes, nil, nil)
		if changed {
			return selectorCSS(nodes)
		}
	}
	if list {
		parent = ":is(" + parent + ")"
	}
	return parent + " " + selector
}

const standardPseudoElements = "::after ::backdrop ::before ::checkmark ::column ::cue ::cue-region ::details-content ::file-selector-button ::first-letter ::first-line ::grammar-error ::highlight ::marker ::part ::picker ::picker-icon ::placeholder ::scroll-button ::scroll-marker ::scroll-marker-group ::search-text ::selection ::slotted ::spelling-error ::target-text ::view-transition ::view-transition-group ::view-transition-group-children ::view-transition-image-pair ::view-transition-new ::view-transition-old"

func standardPseudo(n *selectorNode) bool {
	if n.Kind != "selector" && n.Kind != "function" {
		return false
	}
	value := strings.ToLower(n.Value)
	return strings.HasPrefix(value, "::-") || containsWord(standardPseudoElements, value) || containsWord(":before :after :first-line :first-letter", value)
}

func deadSelector(value string) bool { return deadSelectorList(parseSelector(value)) }
func deadSelectorList(nodes []*selectorNode) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, n := range nodes {
		if !deadSelectorNode(n) {
			return false
		}
	}
	return true
}
func deadSelectorNode(n *selectorNode) bool {
	switch n.Kind {
	case "list":
		for _, child := range n.Nodes {
			if !deadSelectorNode(child) {
				return false
			}
		}
		return true
	case "complex":
		for i, child := range n.Nodes {
			if child.Kind == "combinator" {
				if i == 0 {
					continue
				}
				previous := n.Nodes[i-1]
				if standardPseudo(previous) {
					return true
				}
				if previous.Kind == "compound" {
					for _, item := range previous.Nodes {
						if standardPseudo(item) {
							return true
						}
					}
				}
			} else if deadSelectorNode(child) {
				return true
			}
		}
	case "compound":
		for _, child := range n.Nodes {
			if deadSelectorNode(child) {
				return true
			}
		}
	case "function":
		if n.Value == ":is" || n.Value == ":where" {
			return deadSelectorList(n.Nodes)
		}
	}
	return false
}
