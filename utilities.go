package rocketwind

import (
	"slices"
	"strings"
)

func (b *buildContext) manual(root, value, mod, dataType string, arbitrary, modArbitrary bool) []*Node {
	if root == "start" || root == "end" || root == "-start" || root == "-end" {
		if value == "" {
			return nil
		}
		negative := root[0] == '-'
		property := "inset-inline-start"
		if strings.HasSuffix(root, "end") {
			property = "inset-inline-end"
		}
		resolved := ""
		if arbitrary {
			if mod != "" {
				return nil
			}
			resolved = value
		} else {
			key := value
			if mod != "" && !modArbitrary {
				key += "/" + mod
			}
			resolved = b.resolve(key, []string{"--inset", "--spacing"})
			if resolved == "" && mod != "" && !modArbitrary {
				if !positiveInteger(value) || !positiveInteger(mod) {
					return nil
				}
				resolved = "calc(" + value + "/" + mod + " * 100%)"
			}
			if resolved == "" && quarterNumber(value) {
				if multiplier := b.theme("--spacing"); multiplier != "" {
					if negative {
						value = "-" + value
					}
					return declarations(property, "calc("+multiplier+" * "+value+")")
				}
			}
		}
		if resolved == "" {
			return nil
		}
		if negative {
			resolved = "calc(" + resolved + " * -1)"
		}
		return declarations(property, resolved)
	}
	if root == "shadow" || root == "inset-shadow" || root == "text-shadow" || root == "drop-shadow" {
		return b.shadow(root, value, mod, dataType, arbitrary, modArbitrary)
	}
	if root == "bg-linear" || root == "-bg-linear" || root == "bg-conic" || root == "-bg-conic" || root == "bg-radial" {
		return b.gradient(root, value, mod, dataType, arbitrary, modArbitrary)
	}
	if nodes := b.typedUtility(root, value, mod, dataType, arbitrary, modArbitrary); nodes != nil {
		return nodes
	}
	switch root {
	case "@container":
		if value == "" {
			value = "inline-size"
		} else if !arbitrary && value != "normal" && value != "size" {
			return nil
		}
		nodes := declarations("container-type", value)
		if mod != "" {
			nodes = append(nodes, &Node{Kind: "declaration", Name: "container-name", Value: mod})
		}
		return nodes
	case "scale", "-scale", "rotate", "-rotate":
		if value == "" || mod != "" {
			return nil
		}
		if arbitrary {
			if dataType == "" {
				dataType = inferType(value, "angle", "vector")
			}
			return manualTemplate(root, dataType, value)
		}
		name := strings.TrimPrefix(root, "-")
		v := b.resolve(value, []string{"--" + name})
		if v == "" && positiveInteger(value) {
			v = value + "deg"
			if name == "scale" {
				v = value + "%"
			}
		}
		if v == "" {
			return nil
		}
		if strings.HasPrefix(root, "-") {
			v = "calc(" + v + " * -1)"
		}
		if name == "rotate" {
			return declarations("rotate", v)
		}
		nodes := manualTemplate("scale", "", v)
		nodes = append([]*Node{builtinFunctional["scale-x"][0].Template[0]}, declarations("--tw-scale-x", v)...)
		nodes = append(nodes, declarations("--tw-scale-y", v)...)
		nodes = append(nodes, declarations("--tw-scale-z", v)...)
		return append(nodes, declarations("scale", "var(--tw-scale-x) var(--tw-scale-y)")...)
	case "transform", "filter", "backdrop-filter":
		if mod != "" {
			return nil
		}
		if arbitrary || value == "none" && root != "transform" {
			return manualTemplate(root, "", value)
		}
		if value == "" {
			return manualTemplate(root, "default", "")
		}
	case "duration":
		if value == "" || mod != "" {
			return nil
		}
		if !arbitrary {
			v := b.resolve(value, []string{"--transition-duration"})
			if v == "" && positiveInteger(value) {
				v = value + "ms"
			}
			value = v
		}
		if value != "" {
			return manualTemplate(root, "", value)
		}
	case "flex-grow", "flex-shrink":
		if mod != "" {
			return nil
		}
		if value == "" {
			value = "1"
		}
		if arbitrary || positiveInteger(value) {
			return declarations(root, value)
		}
	case "max-w-screen":
		if !arbitrary && value != "" && mod == "" {
			if v := b.resolve(value, []string{"--breakpoint"}); v != "" {
				return declarations("max-width", v)
			}
		}
	case "flex":
		if arbitrary && mod == "" {
			return declarations("flex", value)
		}
		if positiveInteger(value) {
			if mod != "" {
				if !positiveInteger(mod) {
					return nil
				}
				value = "calc(" + value + "/" + mod + " * 100%)"
			}
			return declarations("flex", value)
		}
	case "bg":
		if arbitrary {
			if dataType == "" {
				dataType = inferType(value, "image", "color", "percentage", "position", "bg-size", "length", "url")
			}
			property := "background-color"
			switch dataType {
			case "image", "url":
				property = "background-image"
			case "percentage", "position":
				property = "background-position"
			case "bg-size", "length", "size":
				property = "background-size"
			}
			if property == "background-color" {
				value = b.colorAlpha(value, mod, modArbitrary)
			} else if mod != "" {
				return nil
			}
			if value != "" {
				return declarations(property, value)
			}
		} else if value != "" {
			if dataType == "" || dataType == "color" {
				if color := b.colorAlpha(b.resolveColor(value, []string{"--background-color", "--color"}), mod, modArbitrary); color != "" {
					return declarations("background-color", color)
				}
			}
			if mod == "" && (dataType == "" || dataType == "image" || dataType == "url") {
				if image := b.resolve(value, []string{"--background-image"}); image != "" {
					return declarations("background-image", image)
				}
			}
		}
	case "text":
		if arbitrary {
			if dataType == "" {
				dataType = inferType(value, "color", "length", "percentage", "absolute-size", "relative-size")
			}
			if dataType == "length" || dataType == "absolute-size" || dataType == "relative-size" || dataType == "percentage" {
				return b.fontSize(value, mod, modArbitrary, nil)
			}
			if color := b.colorAlpha(value, mod, modArbitrary); color != "" {
				return declarations("color", color)
			}
		} else if value != "" {
			if dataType == "" || dataType == "color" {
				if color := b.colorAlpha(b.resolveColor(value, []string{"--text-color", "--color"}), mod, modArbitrary); color != "" {
					return declarations("color", color)
				}
			}
			size := ""
			if dataType != "color" {
				size = b.resolve(value, []string{"--text"})
			}
			if size != "" {
				var extra []*Node
				for _, p := range []string{"line-height", "letter-spacing", "font-weight"} {
					if v := b.theme("--text-" + value + "--" + p); v != "" {
						tw := p
						if p == "line-height" {
							tw = "leading"
						}
						if p == "letter-spacing" {
							tw = "tracking"
						}
						extra = append(extra, &Node{Kind: "declaration", Name: p, Value: "var(--tw-" + tw + ", " + v + ")"})
					}
				}
				return b.fontSize(size, mod, modArbitrary, extra)
			}
		}
	case "font":
		if mod != "" || value == "" {
			return nil
		}
		if arbitrary {
			if dataType == "" {
				dataType = inferType(value, "number", "generic-name", "family-name")
			}
			return manualTemplate(root, dataType, value)
		}
		family := ""
		if dataType != "number" {
			family = b.resolve(value, []string{"--font"})
		}
		if family != "" {
			nodes := declarations("font-family", family)
			for _, p := range []string{"font-feature-settings", "font-variation-settings"} {
				if v := b.theme("--font-" + value + "--" + p); v != "" {
					nodes = append(nodes, declarations(p, v)...)
				}
			}
			return nodes
		}
		if dataType != "family-name" && dataType != "generic-name" {
			if weight := b.resolve(value, []string{"--font-weight"}); weight != "" {
				return manualTemplate(root, "number", weight)
			}
		}
	case "fill", "stroke":
		if arbitrary {
			if root == "stroke" && (dataType == "length" || dataType == "number" || dataType == "" && (isLength(value) || positiveInteger(value))) {
				return nil
			}
		} else {
			value = b.resolveColor(value, []string{"--" + root, "--color"})
		}
		if value = b.colorAlpha(value, mod, modArbitrary); value != "" {
			return declarations(root, value)
		}
	}
	return nil
}

func manualTemplate(root, dataType, value string) []*Node {
	if dataType == "" {
		dataType = "unknown"
	}
	i, ok := manualTemplateIndex[root][dataType]
	if !ok {
		// Every root dispatched here has a generated untyped fallback.
		i = manualTemplateIndex[root][""]
	}
	return cloneNodes(manualTemplates[i], value, false)
}

func (b *buildContext) container() []*Node {
	nodes := append(declarations("--tw-sort", "--tw-container-component"), declarations("width", "100%")...)
	var keys []string
	for key := range builtinTheme {
		if !b.compiler.config.NoDefaultTheme && strings.HasPrefix(key, "--breakpoint-") {
			keys = append(keys, key)
		}
	}
	for key := range b.compiler.config.Theme {
		if strings.HasPrefix(key, "--breakpoint-") && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	slices.SortFunc(keys, func(a, z string) int { return compareBreakpoints(b.themeValue(a), b.themeValue(z)) })
	for _, key := range keys {
		v := b.themeValue(key)
		nodes = append(nodes, &Node{Kind: "at-rule", Name: "@media", Value: "(width >= " + v + ")", Nodes: declarations("max-width", v)})
	}
	return nodes
}

func (b *buildContext) typedUtility(root, value, mod, dataType string, arbitrary, modArbitrary bool) []*Node {
	colorKeys, widthKeys := []string{}, []string{}
	colorFirst, hasDefault := true, false
	types := []string{"color", "length", "percentage"}
	widthType := "length"
	unit := "px"
	switch {
	case root == "border" || strings.HasPrefix(root, "border-") && manualTemplateIndex[root] != nil:
		colorKeys = []string{"--border-color", "--color"}
		widthKeys = []string{"--border-width"}
		hasDefault = true
		types = []string{"color", "line-width", "length"}
	case root == "outline":
		colorKeys = []string{"--outline-color", "--color"}
		widthKeys = []string{"--outline-width"}
		hasDefault = true
		types = []string{"color", "length", "number", "percentage"}
	case root == "stroke":
		colorKeys = []string{"--stroke", "--color"}
		widthKeys = []string{"--stroke-width"}
		unit = ""
		types = []string{"color", "number", "length", "percentage"}
	case root == "decoration":
		colorKeys = []string{"--text-decoration-color", "--color"}
		widthKeys = []string{"--text-decoration-thickness"}
		colorFirst = false
	case root == "ring" || root == "inset-ring":
		colorKeys = []string{"--ring-color", "--color"}
		widthKeys = []string{"--ring-width"}
		hasDefault = true
	case root == "ring-offset":
		colorKeys = []string{"--ring-offset-color", "--color"}
		widthKeys = []string{"--ring-offset-width"}
		colorFirst = false
	case root == "from" || root == "via" || root == "to":
		colorKeys = []string{"--background-color", "--color"}
		widthKeys = []string{"--gradient-color-stop-positions"}
		unit = "%"
	case strings.HasPrefix(root, "mask-") && (strings.HasSuffix(root, "-from") || strings.HasSuffix(root, "-to")) && manualTemplateIndex[root] != nil:
		colorKeys = []string{"--background-color", "--color"}
		unit = "spacing"
		types = []string{"length", "percentage", "color"}
	case root == "mask":
		if !arbitrary || mod != "" {
			return nil
		}
		if dataType == "" {
			dataType = inferType(value, "image", "percentage", "position", "bg-size", "length", "url")
		}
		return manualTemplate(root, dataType, value)
	default:
		return nil
	}
	if value == "" {
		if hasDefault && mod == "" {
			return manualTemplate(root, "default", "")
		}
		return nil
	}
	color := func(v string) []*Node {
		v = b.colorAlpha(v, mod, modArbitrary)
		if v == "" {
			return nil
		}
		return manualTemplate(root, "color", v)
	}
	width := func(v string) []*Node {
		if v == "" || mod != "" {
			return nil
		}
		return manualTemplate(root, widthType, v)
	}
	if arbitrary {
		if dataType == "" {
			dataType = inferType(value, types...)
		}
		isWidth := dataType == "length" || dataType == "line-width" || dataType == "number" || dataType == "percentage" || unit == "spacing" && dataType != "color"
		if isWidth {
			if unit == "spacing" && dataType == "percentage" && !positiveInteger(strings.TrimSuffix(value, "%")) {
				return nil
			}
			return width(value)
		}
		return color(value)
	}
	if dataType == "color" {
		return color(b.resolveColor(value, colorKeys))
	}
	if colorFirst && dataType == "" {
		if n := color(b.resolveColor(value, colorKeys)); n != nil {
			return n
		}
	}
	v := b.resolve(value, widthKeys)
	if v == "" {
		switch unit {
		case "%":
			if strings.HasSuffix(value, "%") && positiveInteger(value[:len(value)-1]) {
				v = value
			}
		case "spacing":
			if strings.HasSuffix(value, "%") && positiveInteger(value[:len(value)-1]) {
				v = value
			} else {
				v = b.bare("spacing", value, "", false)
			}
		default:
			if positiveInteger(value) {
				v = value + unit
			}
		}
	}
	if n := width(v); n != nil {
		return n
	}
	if !colorFirst && dataType == "" {
		return color(b.resolveColor(value, colorKeys))
	}
	return nil
}

func (b *buildContext) fontSize(value, mod string, arbitrary bool, extra []*Node) []*Node {
	nodes := declarations("font-size", value)
	if mod == "" {
		return append(nodes, extra...)
	}
	line := mod
	if !arbitrary {
		line = b.resolve(mod, []string{"--leading"})
		if mod == "none" && line == "" {
			line = "1"
		}
		if line == "" {
			line = b.bare("spacing", mod, "", false)
		}
	}
	if line == "" {
		return nil
	}
	return append(nodes, &Node{Kind: "declaration", Name: "line-height", Value: line})
}
