// Duplicate declaration behavior follows Tailwind CSS v3.4.17.
// Copyright Tailwind Labs. MIT license: testdata/upstream/LICENSE.
package rocketwind

import (
	"regexp"
	"slices"
	"strings"
)

var v3UnitPattern = regexp.MustCompile(`^-?\d*.?\d+([\w%]+)?$`)

func v3Collapse(nodes []*Node) []*Node {
	var result []*Node
	for _, n := range nodes {
		if len(result) > 0 && (n.Kind == "rule" || n.Kind == "at-rule" && n.Name != "@font-face") {
			previous := result[len(result)-1]
			if previous.Kind == n.Kind && strings.Join(strings.Fields(previous.Name), " ") == strings.Join(strings.Fields(n.Name), " ") && strings.Join(strings.Fields(previous.Value), " ") == strings.Join(strings.Fields(n.Value), " ") {
				previous.Nodes = append(previous.Nodes, n.Nodes...)
				continue
			}
		}
		result = append(result, n)
	}
	for _, n := range result {
		if n.Kind == "at-rule" {
			n.Nodes = v3Collapse(n.Nodes)
		}
		if n.Kind != "rule" {
			continue
		}
		seen := map[string]*Node{}
		drop := map[*Node]bool{}
		byProperty := map[string][]*Node{}
		for _, d := range n.Nodes {
			if d.Kind != "declaration" {
				continue
			}
			if previous := seen[d.Name]; previous != nil {
				if previous.Value == d.Value {
					drop[previous] = true
					seen[d.Name] = d
					continue
				}
				entries := byProperty[d.Name]
				if !slices.Contains(entries, previous) {
					entries = append(entries, previous)
				}
				if !slices.Contains(entries, d) {
					entries = append(entries, d)
				}
				byProperty[d.Name] = entries
			}
			seen[d.Name] = d
		}
		for _, entries := range byProperty {
			units := map[string]*Node{}
			for _, d := range entries {
				matches := v3UnitPattern.FindStringSubmatch(d.Value)
				if matches == nil {
					continue
				}
				if previous := units[matches[1]]; previous != nil {
					drop[previous] = true
				}
				units[matches[1]] = d
			}
		}
		n.Nodes = slices.DeleteFunc(n.Nodes, func(d *Node) bool { return drop[d] })
	}
	return result
}
