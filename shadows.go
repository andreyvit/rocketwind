// Shadow color substitution follows Tailwind CSS v4.3.4. See testdata/upstream/LICENSE.
package rocketwind

import (
	"regexp"
	"strconv"
	"strings"
)

var shadowLength = regexp.MustCompile(`^-?(\d+|\.\d+)`)

func replaceShadowColors(input string, replacement func(string) string) string {
	var shadows []string
	for shadow := range Segments(input, ',') {
		shadow = strings.TrimSpace(shadow)
		ast := parseValue(shadow)
		unknown, unknowns, lengths := -1, 0, 0
		replaced := false
		for i, n := range ast {
			value := strings.ToLower(n.value)
			if n.kind == 's' {
				continue
			}
			if n.kind == 'w' {
				if containsWord("inset inherit initial revert unset", value) {
					continue
				}
				if shadowLength.MatchString(value) {
					lengths++
					continue
				}
				if strings.HasPrefix(value, "#") || namedColors[value] {
					n.value = replacement(n.value)
					replaced = true
					break
				}
			} else if n.kind == 'f' {
				if containsWord("color color-mix contrast-color device-cmyk hsl hsla hwb lab lch light-dark oklab oklch rgb rgba --alpha", value) {
					n.kind, n.value, n.nodes = 'w', replacement(valueCSS([]*valueNode{n})), nil
					replaced = true
					break
				}
				if containsWord("calc clamp max min --spacing", value) {
					lengths++
					continue
				}
			}
			unknown = i
			unknowns++
		}
		if !replaced && lengths >= 2 {
			if unknowns == 0 {
				shadows = append(shadows, shadow+" "+replacement("currentcolor"))
				continue
			}
			if unknowns == 1 {
				n := ast[unknown]
				n.kind, n.value, n.nodes = 'w', replacement(valueCSS([]*valueNode{n})), nil
				replaced = true
			}
		}
		if replaced {
			shadow = valueCSS(ast)
		}
		shadows = append(shadows, shadow)
	}
	return strings.Join(shadows, ", ")
}

func (b *buildContext) shadow(root, value, mod, dataType string, arbitrary, modArbitrary bool) []*Node {
	drop := root == "drop-shadow"
	property := "--tw-" + root
	if root == "text-shadow" {
		property = "text-shadow"
	} else if drop {
		property = "--tw-drop-shadow-size"
	}
	props := manualTemplate(root, "color", "")[0]
	colorProperty := "--tw-" + root + "-color"
	alphaProperty := "--tw-" + root + "-alpha"
	filterValue := builtinFunctional["blur"][0].Template[2].Value
	boxValue := "var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)"
	color := func(v string) []*Node {
		if v == "" {
			return nil
		}
		nodes := []*Node{props}
		if v != "inherit" {
			v = b.colorAlpha(v, "var("+alphaProperty+")", true)
		}
		nodes = append(nodes, declarations(colorProperty, v)...)
		if drop {
			nodes = append(nodes, declarations("--tw-drop-shadow", "var(--tw-drop-shadow-size)")...)
		}
		return nodes
	}
	alpha := ""
	if mod != "" {
		if modArbitrary {
			alpha = mod
		} else if quarterNumber(mod) {
			alpha = mod + "%"
		}
	}
	shadow, resolved := "", ""
	prefix := ""
	if arbitrary {
		if dataType == "" {
			dataType = inferType(value, "color")
		}
		if dataType == "color" {
			return color(b.colorAlpha(value, mod, modArbitrary))
		}
		shadow = value
		if root == "inset-shadow" {
			prefix = "inset"
		}
	} else {
		if value == "inherit" {
			if mod != "" {
				return nil
			}
			return color(value)
		}
		if value == "none" {
			if drop {
				return nil
			}
			if mod != "" {
				return nil
			}
			v := "0 0 #0000"
			if root == "inset-shadow" {
				v = "inset " + v
			}
			if root == "text-shadow" {
				v = "none"
			}
			nodes := append([]*Node{props}, declarations(property, v)...)
			if root == "shadow" || root == "inset-shadow" {
				nodes = append(nodes, declarations("box-shadow", boxValue)...)
			}
			return nodes
		}
		key := "--" + root
		if value != "" {
			key += "-" + value
		}
		shadow = b.themeValue(key)
		if drop && shadow != "" {
			resolved = b.theme(key)
		}
		if shadow == "" {
			namespace := "--" + root + "-color"
			if root == "shadow" || root == "inset-shadow" {
				namespace = "--box-shadow-color"
			}
			if value == "" {
				return nil
			}
			return color(b.colorAlpha(b.resolveColor(value, []string{namespace, "--color"}), mod, modArbitrary))
		}
	}
	if shadow == "" || mod != "" && alpha == "" {
		return nil
	}
	nodes := []*Node{props}
	if alpha != "" {
		nodes = append(nodes, declarations(alphaProperty, alpha)...)
	}
	injector := func(c string) string { return "var(" + colorProperty + ", " + c + ")" }
	needsFallback := false
	replacement := func(c string) string {
		if alpha == "" {
			return injector(c)
		}
		if strings.HasPrefix(c, "current") {
			return injector(b.colorAlpha(c, alpha, true))
		}
		if strings.HasPrefix(c, "var(") || strings.HasPrefix(alpha, "var(") {
			needsFallback = true
		}
		a := alpha
		if n, err := strconv.ParseFloat(a, 64); err == nil {
			a = strconv.FormatFloat(n*100, 'f', -1, 64) + "%"
		}
		return injector("oklab(from " + c + " l a b / " + a + ")")
	}
	transform := func(replace func(string) string) string {
		var out []string
		for part := range Segments(shadow, ',') {
			v := replaceShadowColors(part, replace)
			if drop {
				v = "drop-shadow(" + v + ")"
			} else if prefix != "" {
				v = prefix + " " + strings.TrimSpace(v)
			}
			out = append(out, v)
		}
		separator := ", "
		if drop {
			separator = " "
		}
		return strings.Join(out, separator)
	}
	replaced := transform(replacement)
	if needsFallback {
		nodes = append(nodes, declarations(property, transform(injector))...)
		nodes = append(nodes, &Node{Kind: "at-rule", Name: "@supports", Value: "(color: lab(from red l a b))", Nodes: declarations(property, replaced)})
	} else {
		nodes = append(nodes, declarations(property, replaced)...)
	}
	if drop {
		v := "var(--tw-drop-shadow-size)"
		if resolved != "" && (alpha == "" || value == "") {
			var out []string
			for part := range Segments(resolved, ',') {
				out = append(out, "drop-shadow("+part+")")
			}
			v = strings.Join(out, " ")
		}
		nodes = append(nodes, declarations("--tw-drop-shadow", v)...)
		nodes = append(nodes, declarations("filter", filterValue)...)
	} else if root == "shadow" || root == "inset-shadow" {
		nodes = append(nodes, declarations("box-shadow", boxValue)...)
	}
	return nodes
}
