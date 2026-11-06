package rocketwind

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

func dimension(value string) (float64, string, bool) {
	i := 0
	if len(value) > 0 && (value[0] == '+' || value[0] == '-') {
		i++
	}
	digits, dot := 0, false
	for i < len(value) {
		c := value[i]
		if c >= '0' && c <= '9' {
			digits++
			i++
			continue
		}
		if c == '.' && !dot {
			dot = true
			i++
			continue
		}
		break
	}
	if digits == 0 || value[i-1] == '.' {
		return 0, "", false
	}
	for j := i; j < len(value); j++ {
		c := value[j]
		if c == '%' && j == i && j == len(value)-1 {
			break
		}
		if c < 'a' || c > 'z' {
			if c < 'A' || c > 'Z' {
				return 0, "", false
			}
		}
	}
	n, err := strconv.ParseFloat(value[:i], 64)
	return n, value[i:], err == nil || math.IsInf(n, 0)
}

func (b *buildContext) evaluateFunctions(nodes []*valueNode, atRule bool, depth int) bool {
	if depth > 100 {
		b.err = fmt.Errorf("Circular theme function reference")
		return false
	}
	for _, n := range nodes {
		if n.kind != 'f' {
			continue
		}
		switch n.value {
		case "--spacing", "--alpha", "--theme", "theme":
			b.features |= FeatureThemeFunction
			args := slices.Collect(Segments(strings.TrimSpace(valueCSS(n.nodes)), ','))
			for i := range args {
				args[i] = strings.TrimSpace(args[i])
			}
			value, err := b.evaluateFunction(n.value, args, atRule)
			if err != nil {
				b.err = err
				return false
			}
			replacement := parseValue(value)
			if !b.evaluateFunctions(replacement, atRule, depth+1) {
				return false
			}
			n.kind, n.value, n.nodes = 'w', valueCSS(replacement), nil
		default:
			if !b.evaluateFunctions(n.nodes, atRule, depth+1) {
				return false
			}
		}
	}
	return true
}

func (b *buildContext) evaluateFunction(name string, args []string, atRule bool) (string, error) {
	arg := args[0]
	switch name {
	case "--spacing":
		if arg == "" {
			return "", fmt.Errorf("The --spacing(…) function requires an argument, but received none.")
		}
		if len(args) != 1 {
			return "", fmt.Errorf("The --spacing(…) function only accepts a single argument, but received %d.", len(args))
		}
		spacing := b.theme("--spacing")
		if spacing == "" {
			return "", fmt.Errorf("The --spacing(…) function requires that the `--spacing` theme variable exists, but it was not found.")
		}
		if value, _, ok := dimension(arg); ok {
			if value == 0 {
				return "0px", nil
			}
			if value == 1 {
				return spacing, nil
			}
		}
		return "calc(" + spacing + " * " + arg + ")", nil
	case "--alpha":
		parts := slices.Collect(Segments(arg, '/'))
		color, alpha := strings.TrimSpace(parts[0]), ""
		if len(parts) > 1 {
			alpha = strings.TrimSpace(parts[1])
		}
		if color == "" || alpha == "" {
			if color == "" {
				color = "var(--my-color)"
			}
			if alpha == "" {
				alpha = "50%"
			}
			return "", fmt.Errorf("The --alpha(…) function requires a color and an alpha value, e.g.: `--alpha(%s / %s)`", color, alpha)
		}
		if len(args) > 1 {
			return "", fmt.Errorf("The --alpha(…) function only accepts one argument, e.g.: `--alpha(%s / %s)`", color, alpha)
		}
		return b.colorAlpha(color, alpha, true), nil
	}
	inline := name == "theme" || atRule
	if name == "theme" {
		arg = unquoteThemePath(arg)
	} else {
		if !strings.HasPrefix(arg, "--") {
			return "", fmt.Errorf("The --theme(…) function can only be used with CSS variables from your theme.")
		}
		if strings.HasSuffix(arg, " inline") {
			inline = true
			arg = strings.TrimSuffix(arg, " inline")
		}
	}
	path, opacity := arg, ""
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path, opacity = strings.TrimSpace(path[:i]), strings.TrimSpace(path[i+1:])
	}
	key := path
	if !strings.HasPrefix(path, "--") {
		key = legacyThemeKey(path)
	}
	var value string
	if inline {
		value = b.themeValue(key)
	} else {
		value = b.theme(key)
	}
	if name == "theme" && !strings.HasPrefix(path, "--") {
		if b.compiler.legacyTheme != nil {
			value = themeString(b.compiler.themeLookup(path))
		} else {
			if value == "" || strings.Contains(b.compiler.themeFlags[key], "default") {
				if legacy := b.legacyThemeValue(path, key); legacy != "" {
					value = legacy
				}
			}
		}
	}
	if opacity != "" && value != "" {
		value = b.colorAlpha(value, opacity, true)
	}
	if value == "" {
		if len(args) > 1 {
			return strings.Join(args[1:], ", "), nil
		}
		description := "variable name"
		if name == "theme" {
			description = "path"
		}
		return "", fmt.Errorf("Could not resolve value for theme function: `theme(%s)`. Consider checking if the %s is correct or provide a fallback value to silence this error.", arg, description)
	}
	if name == "--theme" && len(args) > 1 {
		fallback := strings.Join(args[1:], ", ")
		if fallback == "initial" {
			return value, nil
		}
		if value == "initial" {
			return fallback, nil
		}
		if strings.HasPrefix(value, "var(") || strings.HasPrefix(value, "theme(") || strings.HasPrefix(value, "--theme(") {
			nodes := parseValue(value)
			injectThemeFallback(nodes, fallback)
			value = valueCSS(nodes)
		}
	}
	return value, nil
}

func injectThemeFallback(nodes []*valueNode, fallback string) {
	for _, n := range nodes {
		if n.kind != 'f' {
			continue
		}
		if n.value == "var" || n.value == "theme" || n.value == "--theme" {
			if len(n.nodes) == 1 {
				n.nodes = append(n.nodes, &valueNode{kind: 'w', value: ", " + fallback})
			} else if len(n.nodes) > 1 {
				last := n.nodes[len(n.nodes)-1]
				if last.kind == 'w' && last.value == "initial" {
					last.value = fallback
				}
			}
		}
		injectThemeFallback(n.nodes, fallback)
	}
}

func unquoteThemePath(value string) string {
	if len(value) < 2 || value[0] != '\'' && value[0] != '"' {
		return value
	}
	quote := value[0]
	var out strings.Builder
	for i := 1; i < len(value)-1; i++ {
		if value[i] == '\\' && i+1 < len(value)-1 && (value[i+1] == quote || value[i+1] == '\\') {
			i++
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

var legacyNamespaces = map[string]string{
	"animation": "animate", "aspectRatio": "aspect", "borderRadius": "radius", "boxShadow": "shadow", "colors": "color", "containers": "container", "fontFamily": "font", "fontSize": "text", "letterSpacing": "tracking", "lineHeight": "leading", "maxWidth": "container", "screens": "breakpoint", "transitionTimingFunction": "ease",
}

func legacyThemeKey(path string) string {
	return legacyThemeProperty(themeKeyPath(path))
}

func legacyThemeProperty(parts []string) string {
	if len(parts) == 0 || parts[0] == "container" {
		return ""
	}
	parts = slices.Clone(parts)
	if len(parts) > 1 && parts[1] == "DEFAULT" {
		switch parts[0] {
		case "borderWidth", "outlineWidth", "ringColor", "ringWidth", "transitionDuration", "transitionTimingFunction":
			return "--default-" + kebabCase(parts[0])
		}
	}
	for _, part := range parts {
		if part == "" {
			return ""
		}
		for _, ch := range part {
			if ch > 127 || !asciiLetter(byte(ch)) && !digit(byte(ch)) && !strings.ContainsRune("-_%/.", ch) {
				return ""
			}
		}
	}
	if ns := legacyNamespaces[parts[0]]; ns != "" {
		parts[0] = ns
	}
	for i, part := range parts {
		if part == "1" && i < len(parts)-1 {
			parts[i] = ""
			continue
		}
		part = strings.ReplaceAll(part, ".", "_")
		if i == 0 || strings.HasPrefix(part, "-") || part == "lineHeight" {
			part = kebabCase(part)
		}
		parts[i] = part
	}
	if parts[len(parts)-1] == "DEFAULT" {
		parts = parts[:len(parts)-1]
	}
	return "--" + strings.Join(parts, "-")
}

func kebabCase(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if i > 0 && value[i-1] >= 'a' && value[i-1] <= 'z' && ch >= 'A' && ch <= 'Z' {
			out.WriteByte('-')
			ch += 'a' - 'A'
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func (b *buildContext) legacyThemeValue(path, key string) string {
	// These legacy configuration namespaces inherit the color palette.
	for _, namespace := range []string{"backgroundColor", "borderColor", "textColor", "outlineColor", "ringColor", "fill", "stroke", "accentColor", "caretColor", "divideColor", "placeholderColor", "textDecorationColor", "gradientColorStops"} {
		if strings.HasPrefix(path, namespace+".") {
			return b.themeValue(legacyThemeKey("colors." + path[len(namespace)+1:]))
		}
	}
	return legacyDefaultTheme[path]
}

func themeKeyPath(path string) []string {
	keys := []string{}
	for part := range Segments(path, '.') {
		if !strings.Contains(part, "[") {
			keys = append(keys, part)
			continue
		}
		start := 0
		for {
			left := strings.IndexByte(part[start:], '[')
			if left < 0 {
				break
			}
			left += start
			right := strings.IndexByte(part[left:], ']')
			if right < 0 {
				break
			}
			right += left
			if left > start {
				keys = append(keys, part[start:left])
			}
			keys = append(keys, part[left+1:right])
			start = right + 1
		}
		if start < len(part) {
			keys = append(keys, part[start:])
		}
	}
	return keys
}
