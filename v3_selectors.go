// Pseudo-element movement follows Tailwind CSS v3.4.17's pseudoElements module.
// Copyright Tailwind Labs. MIT license: testdata/upstream/LICENSE.
package rocketwind

import "strings"

func v3PseudoFlags(name string) (terminal, actionable, jumpable bool) {
	switch {
	case containsWord("::after ::backdrop ::before ::first-letter ::first-line ::marker ::placeholder ::selection :after :before :first-letter :first-line", name):
		return true, false, true
	case containsWord("::cue ::cue-region ::grammar-error ::slotted ::spelling-error ::target-text", name):
		return true, false, false
	case containsWord("::deep ::v-deep ::ng-deep", name):
		return false, true, false
	case strings.HasPrefix(name, "::"):
		return true, true, false
	}
	return false, false, false
}

func v3MovePseudos(selector string) string {
	if !strings.Contains(selector, ":") {
		return selector
	}
	var results []string
	for part := range Segments(selector, ',') {
		tree := parseSelectorMode(strings.TrimSpace(part), true)
		var flatten func([]*selectorNode) []*selectorNode
		flatten = func(nodes []*selectorNode) []*selectorNode {
			var result []*selectorNode
			for _, n := range nodes {
				if n.Kind == "compound" || n.Kind == "complex" {
					result = append(result, flatten(n.Nodes)...)
				} else {
					result = append(result, n)
				}
			}
			return result
		}
		var collect func([]*selectorNode) ([]*selectorNode, *selectorNode)
		collect = func(nodes []*selectorNode) ([]*selectorNode, *selectorNode) {
			var buffer []*selectorNode
			var last *selectorNode
			for _, n := range flatten(nodes) {
				if n.Kind == "combinator" {
					kept := buffer[:0]
					for _, p := range buffer {
						if _, _, jump := v3PseudoFlags(p.Value); jump {
							kept = append(kept, p)
						}
					}
					buffer, last = kept, nil
				} else if strings.HasPrefix(n.Value, ":") {
					terminal, _, _ := v3PseudoFlags(n.Value)
					if terminal {
						last = n
						buffer = append(buffer, n)
					} else {
						actionable := false
						if last != nil {
							_, actionable, _ = v3PseudoFlags(last.Value)
						}
						if actionable && !strings.HasPrefix(n.Value, "::") && !containsWord(":where :is :has :before :after :first-line :first-letter", n.Value) {
							buffer = append(buffer, n)
						} else {
							last = nil
						}
					}
					if len(n.Nodes) > 0 {
						moved, lastInside := collect(n.Nodes)
						buffer = append(buffer, moved...)
						if lastInside != nil {
							last = lastInside
						}
					}
				} else if n.Kind == "list" {
					for _, child := range n.Nodes {
						moved, lastInside := collect([]*selectorNode{child})
						buffer = append(buffer, moved...)
						if lastInside != nil {
							last = lastInside
						}
					}
				}
			}
			return buffer, last
		}
		moved, _ := collect(tree)
		if len(moved) == 0 {
			results = append(results, strings.TrimSpace(part))
			continue
		}
		drop := make(map[*selectorNode]bool, len(moved))
		for _, n := range moved {
			drop[n] = true
		}
		var remove func([]*selectorNode) []*selectorNode
		remove = func(nodes []*selectorNode) []*selectorNode {
			result := nodes[:0]
			for _, n := range nodes {
				if !drop[n] {
					if n.Nodes != nil {
						n.Nodes = remove(n.Nodes)
					}
					result = append(result, n)
				}
			}
			return result
		}
		tree = remove(tree)
		tree = append(tree, moved...)
		results = append(results, selectorCSS(tree))
	}
	return strings.Join(results, ", ")
}
