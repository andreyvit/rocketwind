// Declaration expansion follows Tailwind CSS v4.3.4 under the MIT license.
package rocketwind

import (
	"slices"
	"strings"
)

type signatureFeatures uint8

const (
	signatureExpand signatureFeatures = 1 << iota
	signatureLogicalToPhysical
)

func expandDeclaration(node *Node, options signatureFeatures) []*Node {
	var properties []string
	if options&signatureLogicalToPhysical != 0 {
		name := node.Name
		if prefix, axis, ok := strings.Cut(name, "-"); ok && prefix == "border" && (strings.HasPrefix(axis, "block") || strings.HasPrefix(axis, "inline")) {
			base, rest, _ := strings.Cut(axis, "-")
			if (base == "block" || base == "inline") && (rest == "" || rest == "color" || rest == "style" || rest == "width") {
				sides := []string{"bottom", "top"}
				if base == "inline" {
					sides = []string{"left", "right"}
				}
				result := make([]*Node, 0, 2)
				for _, side := range sides {
					property := "border-" + side
					if rest != "" {
						property += "-" + rest
					}
					result = append(result, &Node{Kind: "declaration", Name: property, Value: node.Value, Important: node.Important})
				}
				return result
			}
		}
		for _, axis := range []string{"block", "inline"} {
			prefix, ok := strings.CutSuffix(name, "-"+axis)
			if !ok || !containsWord("inset margin padding scroll-margin scroll-padding", prefix) {
				continue
			}
			sides := []string{"top", "bottom"}
			if axis == "inline" {
				sides = []string{"left", "right"}
			}
			for _, side := range sides {
				if prefix != "inset" {
					side = prefix + "-" + side
				}
				properties = append(properties, side)
			}
			break
		}
	}
	if properties == nil {
		switch node.Name {
		case "inset", "margin", "padding", "scroll-margin", "scroll-padding", "border-width", "border-style", "border-color":
			for _, side := range []string{"top", "right", "bottom", "left"} {
				property := side
				if suffix, border := strings.CutPrefix(node.Name, "border-"); border {
					property = "border-" + side + "-" + suffix
				} else if node.Name != "inset" {
					property = node.Name + "-" + side
				}
				properties = append(properties, property)
			}
		case "gap":
			properties = []string{"row-gap", "column-gap"}
		case "overflow", "overscroll-behavior":
			properties = []string{node.Name + "-x", node.Name + "-y"}
		default:
			return nil
		}
	}
	args := slices.Collect(Segments(node.Value, ' '))
	if len(args) < 1 || len(args) > len(properties) {
		return nil
	}
	indices := [4][4]int{{0, 0, 0, 0}, {0, 1, 0, 1}, {0, 1, 2, 1}, {0, 1, 2, 3}}[len(args)-1]
	result := make([]*Node, len(properties))
	for i, property := range properties {
		result[i] = &Node{Kind: "declaration", Name: property, Value: args[indices[i]], Important: node.Important}
	}
	return result
}
