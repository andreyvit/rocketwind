package rocketwind

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// Compatibility selects the Tailwind generation whose defaults and CSS apply.
type Compatibility uint8

const (
	Tailwind4 Compatibility = iota // Tailwind CSS 4.3.4, the default.
	Tailwind3                      // Tailwind CSS 3.4.17.
)

type v3StaticDefinition struct {
	Order int
	Nodes []*Node
}
type v3UtilityDefinition struct {
	Namespace               string
	Order                   int
	Types                   []string
	Negative, FilterDefault bool
	Template                []*Node
}

func v3Escape(value string) string {
	escaped := strings.ReplaceAll(Escape(value), `\,`, `\2c `)
	if len(value) > 1 && digit(value[0]) && !strings.ContainsRune("0123456789abcdefABCDEF \t\r\n\f", rune(value[1])) {
		escaped = strings.Replace(escaped, " ", "", 1)
	}
	return escaped
}

func (b *buildContext) v3Static(name string) [][]*Node {
	if name == "container" {
		return [][]*Node{v3Ordered(b.v3Container(), -20000)}
	}
	var result [][]*Node
	for _, d := range v3Static[name] {
		result = append(result, v3Ordered(d.Nodes, d.Order))
	}
	return result
}

func (b *buildContext) v3Container() []*Node {
	c := b.compiler
	result := declarations("width", "100%")
	if c.themeLookup("container.center") == true {
		result = append(result, declarations("margin-right", "auto")...)
		result = append(result, declarations("margin-left", "auto")...)
	}
	padding := c.themeLookup("container.padding")
	appendPadding := func(nodes []*Node, value any) []*Node {
		if value != nil {
			nodes = append(nodes, declarations("padding-right", themeString(value))...)
			nodes = append(nodes, declarations("padding-left", themeString(value))...)
		}
		return nodes
	}
	basePadding := padding
	if object := configObject(padding); object != nil {
		basePadding = object.Get("DEFAULT")
	}
	result = appendPadding(result, basePadding)
	var widths []NamedValue
	var collect func(string, any)
	collect = func(name string, value any) {
		switch value := value.(type) {
		case string:
			widths = append(widths, NamedValue{name, value})
		case []any:
			for _, v := range value {
				collect(name, v)
			}
		default:
			if min := configObject(value).Get("min"); min != nil {
				widths = append(widths, NamedValue{name, themeString(min)})
			}
		}
	}
	for _, screen := range configObject(c.themeLookup("container.screens", c.themeLookup("screens"))) {
		collect(screen.Key, screen.Value)
	}
	slices.SortStableFunc(widths, func(a, b NamedValue) int {
		an, _, _ := dimension(a.Value)
		bn, _, _ := dimension(b.Value)
		return cmp.Compare(an, bn)
	})
	seen := map[string]bool{}
	for _, width := range widths {
		if seen[width.Value] {
			continue
		}
		seen[width.Value] = true
		nodes := appendPadding(declarations("max-width", width.Value), configObject(padding).Get(width.Name))
		result = append(result, &Node{Kind: "at-rule", Name: "@media", Value: "(min-width: " + width.Value + ")", Nodes: nodes})
	}
	return result
}

func (b *buildContext) v3Functional(root, value, modifier, hint string, arbitrary, arbitraryModifier bool) [][]*Node {
	negative := strings.HasPrefix(root, "-")
	root = strings.TrimPrefix(root, "-")
	var result [][]*Node
	definitions := v3Functional[root]
	if arbitrary {
		definitions = v3SelectDefinitions(definitions, value, hint)
	}
	for _, d := range definitions {
		if negative && !d.Negative {
			continue
		}
		var resolved any
		mod := modifier
		color := slices.Contains(d.Types, "color")
		if arbitrary {
			resolved = value
		} else {
			key := value
			if key == "" {
				key = "DEFAULT"
			}
			if key == "DEFAULT" && d.FilterDefault {
				continue
			}
			namespace := configObject(b.compiler.themeLookup(d.Namespace))
			if color {
				namespace = FlattenColors(namespace)
			}
			if modifier != "" && !arbitraryModifier {
				resolved = namespace.Get(key + "/" + modifier)
				if resolved != nil {
					mod = ""
				}
			}
			if resolved == nil {
				resolved = namespace.Get(key)
			}
			if resolved == nil {
				continue
			}
		}
		text := themeString(resolved)
		if negative {
			text = v3Negate(text)
			if text == "" {
				continue
			}
			resolved = text
		}
		if mod != "" {
			if color {
				if !arbitraryModifier {
					mod = themeString(configObject(b.compiler.themeLookup("opacity")).Get(mod))
					if mod == "" {
						continue
					}
				}
				text = v3WithAlpha(text, mod, "")
				if text == "" {
					continue
				}
			} else if d.Namespace != "fontSize" {
				continue
			}
		}
		nodes := cloneNodes(d.Template, text, false)
		switch d.Namespace {
		case "gridTemplateColumns", "gridTemplateRows", "objectPosition":
			parts := appendSegments(nil, text, ',')
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			nodes = cloneNodes(d.Template, strings.Join(parts, " "), false)
		case "blur", "backdropBlur":
			if text == "" {
				nodes[0].Value = " "
			}
		case "dropShadow":
			if values, ok := resolved.([]any); ok {
				parts := make([]string, 0, len(values))
				for _, value := range values {
					parts = append(parts, "drop-shadow("+themeString(value)+")")
				}
				nodes[0].Value = strings.Join(parts, " ")
			}
		case "fontSize":
			nodes = b.v3FontSize(resolved, mod, arbitraryModifier)
		case "fontFamily":
			nodes = v3FontFamily(resolved)
		case "animation":
			for _, frame := range configObject(b.compiler.themeLookup("keyframes")) {
				if slices.Contains(strings.Fields(text), frame.Key) {
					nodes = append([]*Node{{Kind: "at-root", Nodes: []*Node{{Kind: "at-rule", Name: "@keyframes", Value: frame.Key, Nodes: configStyleNodes(frame.Value)}}}}, nodes...)
				}
			}
		case "space", "divideWidth":
			if text == "0" {
				nodes = cloneNodes(d.Template, "0px", false)
			}
		case "boxShadow":
			if text == "none" {
				text = "0 0 #0000"
			}
			nodes[0].Value = text
			nodes[1].Value = replaceShadowColors(text, func(string) string { return "var(--tw-shadow-color)" })
			if themeString(resolved) == "none" {
				nodes[1].Value = text
			}
		case "gradientColorStops":
			if root == "from" || root == "via" {
				transparent := v3WithAlpha(text, "0", "rgb(255 255 255 / 0)")
				for _, n := range nodes {
					n.Value = strings.ReplaceAll(n.Value, "rgb(255 255 255 / 0)", transparent)
				}
			}
		case "transitionProperty":
			if text == "none" {
				nodes = nodes[:1]
			} else {
				nodes[1].Value = themeString(b.compiler.themeLookup("transitionTimingFunction.DEFAULT"))
				nodes[2].Value = themeString(b.compiler.themeLookup("transitionDuration.DEFAULT"))
			}
		}
		if color && mod == "" {
			variable := map[string]string{"backgroundColor": "--tw-bg-opacity", "textColor": "--tw-text-opacity", "borderColor": "--tw-border-opacity", "divideColor": "--tw-divide-opacity", "placeholderColor": "--tw-placeholder-opacity", "ringColor": "--tw-ring-opacity"}[d.Namespace]
			if variable != "" {
				nodes = v3Opacity(nodes, text, variable)
			}
		}
		if len(nodes) > 0 {
			result = append(result, v3Ordered(nodes, d.Order))
		}
	}
	return result
}

func v3SelectDefinitions(definitions []v3UtilityDefinition, value, hint string) []v3UtilityDefinition {
	var matches []v3UtilityDefinition
	for _, d := range definitions {
		valid := false
		if hint != "" {
			valid = slices.Contains(d.Types, hint) || len(definitions) == 1 && containsWord("any color url image length percentage position lookup generic-name family-name number line-width absolute-size relative-size shadow size", hint)
		} else {
			for _, kind := range d.Types {
				if v3MatchesType(value, kind) {
					valid = true
					break
				}
			}
		}
		if valid {
			matches = append(matches, d)
		}
	}
	if len(matches) <= 1 {
		return matches
	}
	for _, allowAny := range []bool{false, true} {
		var candidates []v3UtilityDefinition
		for _, d := range matches {
			if slices.Contains(d.Types, "any") == allowAny {
				candidates = append(candidates, d)
			}
		}
		if len(candidates) == 1 {
			return candidates
		}
		for _, d := range candidates {
			if d.Namespace == "backgroundPosition" && v3MatchesType(value, "position") {
				return []v3UtilityDefinition{d}
			}
		}
	}
	return nil
}

func v3MatchesType(value, kind string) bool {
	switch kind {
	case "any":
		return true
	case "lookup":
		return false
	case "length":
		return value == "0" || isLength(value)
	case "line-width":
		return containsWord("thin medium thick", value)
	case "color":
		mode, _, _ := v3Color(value)
		return mode != ""
	case "shadow":
		for shadow := range Segments(value, ',') {
			count := 0
			for part := range Segments(strings.TrimSpace(shadow), ' ') {
				if part == "0" || isLength(part) {
					count++
				}
			}
			if count < 2 {
				return false
			}
		}
		return true
	case "size":
		return matchesType(value, "bg-size")
	case "position":
		count := 0
		for part := range Segments(value, ' ') {
			if strings.HasPrefix(part, "var(") {
				continue
			}
			if !containsWord("center top right bottom left", part) && !v3MatchesType(part, "length") && !isPercentage(part) {
				return false
			}
			count++
		}
		return count > 0
	case "family-name":
		count := 0
		for part := range Segments(value, ',') {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "var(") {
				continue
			}
			if part == "" || digit(part[0]) {
				return false
			}
			if strings.Contains(part, " ") && !(len(part) > 1 && (part[0] == '\'' || part[0] == '"') && part[len(part)-1] == part[0]) {
				return false
			}
			count++
		}
		return count > 0
	}
	return inferType(value, kind) != ""
}

func v3Negate(value string) string {
	if value == "0" {
		return value
	}
	if !strings.Contains(value, "(") && (isLength(value) || isNumber(value) || isPercentage(value) || isDimension(value, "deg rad grad turn")) {
		if strings.HasPrefix(value, "-") {
			return value[1:]
		}
		return "-" + strings.TrimPrefix(value, "+")
	}
	for _, f := range []string{"var(", "calc(", "min(", "max(", "clamp("} {
		if strings.Contains(value, f) {
			return "calc(" + value + " * -1)"
		}
	}
	return ""
}

func (b *buildContext) v3FontSize(value any, modifier string, arbitrary bool) []*Node {
	var options ConfigObject
	if tuple, ok := value.([]any); ok && len(tuple) > 0 {
		value = tuple[0]
		if len(tuple) > 1 {
			options = configObject(tuple[1])
			if options == nil {
				options = ConfigObject{{"lineHeight", tuple[1]}}
			}
		}
	}
	result := declarations("font-size", themeString(value))
	if modifier != "" {
		if !arbitrary {
			modifier = themeString(configObject(b.compiler.themeLookup("lineHeight")).Get(modifier))
		}
		if modifier == "" {
			return nil
		}
		return append(result, declarations("line-height", modifier)...)
	}
	for _, pair := range []NamedValue{{"lineHeight", "line-height"}, {"letterSpacing", "letter-spacing"}, {"fontWeight", "font-weight"}} {
		if v := options.Get(pair.Name); v != nil {
			result = append(result, declarations(pair.Value, themeString(v))...)
		}
	}
	return result
}

func v3FontFamily(value any) []*Node {
	var options ConfigObject
	if tuple, ok := value.([]any); ok && len(tuple) == 2 && configObject(tuple[1]) != nil {
		value, options = tuple[0], configObject(tuple[1])
	}
	result := declarations("font-family", themeString(value))
	for _, pair := range []NamedValue{{"fontFeatureSettings", "font-feature-settings"}, {"fontVariationSettings", "font-variation-settings"}} {
		if v := options.Get(pair.Name); v != nil {
			result = append(result, declarations(pair.Value, themeString(v))...)
		}
	}
	return result
}

func (b *buildContext) renderV3(nodes []*Node) string {
	var globals []*Node
	var strip func([]*Node) []*Node
	strip = func(nodes []*Node) []*Node {
		var result []*Node
		for _, n := range nodes {
			if n.Kind == "v3-order" {
				continue
			}
			if n.Kind == "at-root" {
				globals = append(globals, n.Nodes...)
				continue
			}
			copy := *n
			if n.Nodes != nil {
				copy.Nodes = strip(n.Nodes)
			}
			result = append(result, &copy)
		}
		return result
	}
	nodes = strip(nodes)
	nodes = v3Flatten(append(globals, nodes...))
	if b.outputAST != nil {
		*b.outputAST = nodes
	}
	w := optimizedCSSWriter{track: b.track}
	w.write(nodes, 0)
	b.mappings = w.mappings
	return w.out.String()
}

// Tailwind 3 expands selector lists as a Cartesian product. It does not use
// CSS nesting's :is() specificity or simplify universal selectors.
func v3Flatten(input []*Node) []*Node {
	var output []*Node
	var walk func([]*Node, string, []*Node)
	emit := func(node *Node, wrappers []*Node) {
		for i := len(wrappers) - 1; i >= 0; i-- {
			copy := *wrappers[i]
			copy.Nodes = []*Node{node}
			node = &copy
		}
		output = append(output, node)
	}
	walk = func(nodes []*Node, parent string, wrappers []*Node) {
		var declarations []*Node
		flush := func() {
			if len(declarations) > 0 {
				if parent != "" {
					emit(&Node{Kind: "rule", Name: parent, Nodes: declarations}, wrappers)
				} else {
					for _, n := range declarations {
						emit(n, wrappers)
					}
				}
				declarations = nil
			}
		}
		for _, n := range nodes {
			if n.Kind == "declaration" {
				declarations = append(declarations, n)
				continue
			}
			flush()
			switch {
			case n.Kind == "rule":
				selector := n.Name
				if parent != "" {
					var selectors []string
					for p := range Segments(parent, ',') {
						for s := range Segments(selector, ',') {
							tree := parseSelectorMode(strings.TrimSpace(s), true)
							if replaceNesting(tree, strings.TrimSpace(p)) {
								selectors = append(selectors, selectorCSS(tree))
							} else {
								selectors = append(selectors, strings.TrimSpace(p)+" "+strings.TrimSpace(s))
							}
						}
					}
					selector = strings.Join(selectors, ", ")
				}
				walk(n.Nodes, selector, wrappers)
			case n.Kind == "at-rule" && n.Nodes != nil && n.Name != "@keyframes" && n.Name != "@font-face":
				walk(n.Nodes, parent, append(slices.Clone(wrappers), n))
			default:
				emit(n, wrappers)
			}
		}
		flush()
	}
	walk(input, "", nil)
	return output
}

func v3Opacity(nodes []*Node, value, variable string) []*Node {
	mode, components, alpha := v3Color(value)
	placeholder := strings.Contains(value, "<alpha-value>")
	if !placeholder && (mode == "" || alpha != "") {
		return nodes
	}
	var visit func([]*Node) []*Node
	visit = func(nodes []*Node) []*Node {
		var result []*Node
		inserted := false
		for _, n := range nodes {
			if n.Kind == "declaration" && n.Value == value {
				if !inserted {
					result = append(result, declarations(variable, "1")...)
					inserted = true
				}
				if placeholder {
					n.Value = strings.ReplaceAll(value, "<alpha-value>", "var("+variable+", 1)")
				} else {
					n.Value = v3FormatColor(mode, components, "var("+variable+", 1)")
				}
			}
			if n.Nodes != nil {
				n.Nodes = visit(n.Nodes)
			}
			result = append(result, n)
		}
		return result
	}
	return visit(nodes)
}

func v3WithAlpha(value, alpha, fallback string) string {
	if strings.Contains(value, "<alpha-value>") {
		return strings.ReplaceAll(value, "<alpha-value>", alpha)
	}
	mode, components, _ := v3Color(value)
	if mode == "" {
		return fallback
	}
	return v3FormatColor(mode, components, alpha)
}

func v3FormatColor(mode, components, alpha string) string {
	if strings.HasSuffix(mode, "a") {
		return mode + "(" + strings.ReplaceAll(components, " ", ", ") + ", " + alpha + ")"
	}
	return mode + "(" + components + " / " + alpha + ")"
}

func v3Color(value string) (mode, components, alpha string) {
	value = strings.TrimSpace(value)
	if value == "transparent" {
		return "rgb", "0 0 0", "0"
	}
	if c := v3ColorNames[value]; c != "" {
		return "rgb", c, ""
	}
	if strings.HasPrefix(value, "#") {
		hex := value[1:]
		if len(hex) == 3 || len(hex) == 4 {
			var b strings.Builder
			for _, c := range hex {
				b.WriteRune(c)
				b.WriteRune(c)
			}
			hex = b.String()
		}
		if len(hex) != 6 && len(hex) != 8 {
			return
		}
		var parts []string
		for i := 0; i < len(hex); i += 2 {
			n, err := strconv.ParseUint(hex[i:i+2], 16, 8)
			if err != nil {
				return "", "", ""
			}
			if i == 6 {
				alpha = strconv.FormatFloat(float64(n)/255, 'f', -1, 64)
			} else {
				parts = append(parts, strconv.Itoa(int(n)))
			}
		}
		return "rgb", strings.Join(parts, " "), alpha
	}
	i := strings.IndexByte(value, '(')
	if i < 0 || !strings.HasSuffix(value, ")") || !containsWord("rgb rgba hsl hsla", value[:i]) {
		return
	}
	mode = value[:i]
	body := value[i+1 : len(value)-1]
	parts := appendSegments(nil, body, ',')
	if len(parts) == 1 {
		parts = appendSegments(nil, body, ' ')
	}
	var componentsList []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "/" {
			continue
		}
		componentsList = append(componentsList, part)
	}
	if len(componentsList) == 4 {
		alpha = componentsList[3]
		componentsList = componentsList[:3]
	}
	if len(componentsList) != 3 {
		return "", "", ""
	}
	return mode, strings.Join(componentsList, " "), alpha
}
