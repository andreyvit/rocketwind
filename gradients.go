package rocketwind

import (
	"cmp"
	"strconv"
	"strings"
)

func compareBreakpoints(a, z string) int {
	return compareBreakpointValues(a, z, false)
}

func compareBreakpointValues(a, z string, descending bool) int {
	unit := func(s string) string {
		if i := strings.IndexByte(s, '('); i >= 0 {
			return s[:i]
		}
		i := 0
		for i < len(s) && (digit(s[i]) || s[i] == '.') {
			i++
		}
		return s[i:]
	}
	if v := strings.Compare(unit(a), unit(z)); v != 0 {
		return v
	}
	number := func(s string) (float64, error) {
		i := 0
		for i < len(s) && (digit(s[i]) || strings.ContainsRune("+-.", rune(s[i]))) {
			i++
		}
		return strconv.ParseFloat(s[:i], 64)
	}
	x, err := number(a)
	y, err2 := number(z)
	if err != nil || err2 != nil {
		return strings.Compare(a, z)
	}
	v := cmp.Compare(x, y)
	if descending {
		v = -v
	}
	return v
}

func (b *buildContext) gradient(root, value, mod, dataType string, arbitrary, modArbitrary bool) []*Node {
	negative := strings.HasPrefix(root, "-")
	root = strings.TrimPrefix(root, "-")
	kind := strings.TrimPrefix(root, "bg-")
	interpolation := "in oklab"
	if mod != "" {
		if modArbitrary {
			interpolation = mod
		} else if containsWord("longer shorter increasing decreasing", mod) {
			interpolation = "in oklch " + mod + " hue"
		} else {
			interpolation = "in " + mod
		}
	}
	if arbitrary {
		if mod != "" {
			return nil
		}
		if kind == "linear" && negative {
			if dataType == "" {
				dataType = inferType(value, "angle")
			}
			if dataType != "angle" {
				return nil
			}
			value = "calc(" + value + " * -1)"
		}
		nodes := declarations("--tw-gradient-position", value)
		return append(nodes, declarations("background-image", kind+"-gradient(var(--tw-gradient-stops,"+value+"))")...)
	}
	if kind == "radial" {
		if value != "" {
			return nil
		}
		value = interpolation
	} else if kind == "conic" && value == "" {
		value = interpolation
	} else {
		direction := ""
		if kind == "linear" && !negative {
			direction = map[string]string{"to-t": "to top", "to-tr": "to top right", "to-r": "to right", "to-br": "to bottom right", "to-b": "to bottom", "to-bl": "to bottom left", "to-l": "to left", "to-tl": "to top left"}[value]
		}
		if direction != "" {
			value = direction
		} else {
			if !positiveInteger(value) {
				return nil
			}
			value += "deg"
			if negative {
				value = "calc(" + value + " * -1)"
			}
		}
		if kind == "conic" {
			value = "from " + value + " " + interpolation
		}
	}
	nodes := declarations("--tw-gradient-position", value)
	if kind == "linear" {
		nodes = append(nodes, &Node{Kind: "at-rule", Name: "@supports", Value: "(background-image: linear-gradient(in lab, red, red))", Nodes: declarations("--tw-gradient-position", value+" "+interpolation)})
	}
	return append(nodes, declarations("background-image", kind+"-gradient(var(--tw-gradient-stops))")...)
}
