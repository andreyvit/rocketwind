package rocketwind

import (
	"fmt"
	"math"
	"strings"
)

// Tailwind 4.3 targets Safari/iOS 16.4, Firefox 128 and Chrome 111. Keep
// browser lowering separate from compilation: unoptimized utility output must
// retain Tailwind's original syntax, including range queries and nesting.
func optimizeBrowserCSS(nodes []*Node) ([]*Node, error) {
	custom := make(map[string]string)
	for _, node := range nodes {
		if node.Name == "@custom-media" {
			name, value, _ := strings.Cut(node.Value, " ")
			custom[name] = strings.TrimSpace(value)
		}
	}
	return lowerBrowserNodes(nodes, custom)
}

func lowerBrowserNodes(nodes []*Node, custom map[string]string) ([]*Node, error) {
	var output []*Node
	for _, node := range nodes {
		if node.Kind == "at-rule" && node.Name == "@custom-media" {
			continue
		}
		if node.Name == "@media" || node.Name == "@container" {
			value, err := lowerBrowserQuery(node.Value, node.Name == "@media", custom)
			if err != nil {
				return nil, err
			}
			node.Value = value
		}
		if len(node.Nodes) > 0 {
			var err error
			node.Nodes, err = lowerBrowserNodes(node.Nodes, custom)
			if err != nil {
				return nil, err
			}
		}
		if node.Kind == "declaration" {
			output = prefixBrowserDeclaration(output, node)
			continue
		}
		output = append(output, node)
	}
	return output, nil
}

func prefixBrowserDeclaration(output []*Node, node *Node) []*Node {
	add := func(name, value string) {
		// Preserve distinct fallback values, including explicit vendor values.
		// Re-optimizing our output must not accumulate identical prefixes.
		for i := len(output) - 1; i >= 0 && output[i].Kind == "declaration"; i-- {
			previous := output[i]
			if previous.Name == name && previous.Important == node.Important {
				if previous.Value == value || compactCSSValue(previous.Value) == compactCSSValue(value) {
					return
				}
				break
			}
		}
		copy := *node
		copy.Name, copy.Value = name, value
		output = append(output, &copy)
	}
	name, value := node.Name, node.Value
	switch name {
	case "backdrop-filter", "mask-image", "mask-position", "mask-size", "mask-repeat", "mask-clip", "mask-origin":
		add("-webkit-"+name, value)
	case "user-select", "hyphens", "box-decoration-break", "print-color-adjust", "text-decoration-skip-ink":
		if !browserUnparsedValue(value) {
			add("-webkit-"+name, value)
		}
	case "text-size-adjust":
		if !browserUnparsedValue(value) {
			add("-webkit-"+name, value)
			add("-moz-"+name, value)
		}
	case "background-clip":
		if containsWord(strings.ReplaceAll(value, ",", " "), "text") {
			add("-webkit-"+name, value)
		}
	case "text-decoration-color", "text-decoration-line", "text-decoration-style":
		if browserUnparsedValue(value) {
			add("-webkit-"+name, value)
		}
	case "text-decoration":
		if !browserUnparsedValue(value) {
			var parts []string
			thickness := ""
			for part := range Segments(value, ' ') {
				if number, unit, ok := dimension(part); ok && unit == "%" {
					thickness = browserDecorationLength(number)
				} else {
					parts = append(parts, part)
				}
			}
			if thickness != "" {
				node.Value = strings.Join(parts, " ")
				output = prefixBrowserDeclaration(output, node)
				copy := *node
				copy.Name, copy.Value = "text-decoration-thickness", thickness
				return append(output, &copy)
			}
		}
		if browserUnparsedValue(value) || browserDecorationShorthand(value) {
			add("-webkit-"+name, value)
		}
	case "mask-composite":
		if !browserUnparsedValue(value) {
			add("-webkit-mask-composite", replaceBrowserKeywords(value, map[string]string{"add": "source-over", "subtract": "source-out", "intersect": "source-in", "exclude": "xor"}))
		}
	case "mask-mode":
		if !browserUnparsedValue(value) {
			add("-webkit-mask-source-type", replaceBrowserKeywords(value, map[string]string{"match-source": "auto"}))
		}
	case "text-decoration-thickness":
		if number, unit, ok := dimension(value); ok && unit == "%" {
			node.Value = browserDecorationLength(number)
		}
	case "mask":
		webkit, composites, modes := browserMaskShorthand(value)
		if composites != "" {
			add("-webkit-mask-composite", composites)
		}
		if modes != "" {
			add("-webkit-mask-source-type", modes)
		}
		add("-webkit-mask", webkit)
	case "mask-border", "mask-border-source", "mask-border-slice", "mask-border-width", "mask-border-outset", "mask-border-repeat":
		if name == "mask-border" && !browserUnparsedValue(value) {
			var parts []string
			for part := range Segments(value, ' ') {
				if part != "alpha" && part != "luminance" {
					parts = append(parts, part)
				}
			}
			value = strings.Join(parts, " ")
		}
		add("-webkit-mask-box-image"+strings.TrimPrefix(name, "mask-border"), value)
	case "width", "height", "min-width", "max-width", "min-height", "max-height", "inline-size", "block-size", "min-inline-size", "max-inline-size", "min-block-size", "max-block-size":
		if value == "stretch" {
			if n := len(output); n >= 2 && output[n-2].Name == name && output[n-2].Value == "-webkit-fill-available" && output[n-2].Important == node.Important && output[n-1].Name == name && output[n-1].Value == "-moz-available" && output[n-1].Important == node.Important {
				return append(output, node)
			}
			add(name, "-webkit-fill-available")
			add(name, "-moz-available")
		}
	case "background", "background-image":
		if strings.Contains(value, "image-set(") && !browserUnparsedValue(value) {
			parsed := parseValue(value)
			if prefixBrowserImages(parsed) {
				var buffer strings.Builder
				writeValue(&buffer, parsed)
				add(name, buffer.String())
			}
		}
		if name == "background" && !browserUnparsedValue(value) {
			var backgrounds, clips []string
			hasText := false
			for layer := range Segments(value, ',') {
				var parts []string
				clip := "border-box"
				for part := range Segments(strings.TrimSpace(layer), ' ') {
					if part == "text" {
						clip, hasText = "text", true
					} else {
						parts = append(parts, part)
					}
				}
				backgrounds, clips = append(backgrounds, strings.Join(parts, " ")), append(clips, clip)
			}
			if hasText {
				node.Value = strings.Join(backgrounds, ", ")
				if node.Value == "" {
					node.Value = "0 0"
				}
				output = append(output, node)
				copy := *node
				copy.Name, copy.Value = "background-clip", strings.Join(clips, ", ")
				return prefixBrowserDeclaration(output, &copy)
			}
		}
	}
	return append(output, node)
}

func browserDecorationLength(percentage float64) string {
	if percentage == 0 {
		// The pinned optimizer emits a subnormal value for zero percentages.
		// Retain that spelling for compatibility with its generated CSS.
		if math.Signbit(percentage) {
			return "-2.93874e-39em"
		}
		return "2.93874e-39em"
	}
	return jsFloat(percentage/100) + "em"
}

func browserMaskShorthand(value string) (string, string, string) {
	if browserUnparsedValue(value) {
		return value, "", ""
	}
	var layers, composites, modes []string
	hasComposite, hasMode := false, false
	for layer := range Segments(value, ',') {
		var parts []string
		composite, mode := "source-over", "auto"
		for word := range Segments(strings.TrimSpace(layer), ' ') {
			switch word {
			case "add":
			case "subtract":
				composite, hasComposite = "source-out", true
			case "intersect":
				composite, hasComposite = "source-in", true
			case "exclude":
				composite, hasComposite = "xor", true
			case "match-source":
			case "alpha", "luminance":
				mode, hasMode = word, true
			default:
				parts = append(parts, word)
			}
		}
		layers = append(layers, strings.Join(parts, " "))
		composites, modes = append(composites, composite), append(modes, mode)
	}
	composite, mode := "", ""
	if hasComposite {
		composite = strings.Join(composites, ", ")
	}
	if hasMode {
		mode = strings.Join(modes, ", ")
	}
	return strings.Join(layers, ", "), composite, mode
}

func prefixBrowserImages(nodes []*valueNode) bool {
	changed := false
	for _, node := range nodes {
		if node.kind != 'f' {
			continue
		}
		if node.value == "image-set" {
			changed = true
			node.value = "-webkit-image-set"
			for _, arg := range node.nodes {
				if arg.kind == 'w' && len(arg.value) > 1 && (arg.value[0] == '\'' || arg.value[0] == '"') {
					arg.kind, arg.nodes, arg.value = 'f', []*valueNode{{kind: 'w', value: arg.value}}, "url"
				}
			}
		} else {
			if prefixBrowserImages(node.nodes) {
				changed = true
			}
		}
	}
	return changed
}

func browserUnparsedValue(value string) bool {
	return strings.Contains(value, "var(") || strings.Contains(value, "env(") ||
		strings.Contains(value, "color-mix(") && strings.Contains(strings.ToLower(value), "currentcolor") ||
		containsWord("inherit initial unset revert revert-layer", value)
}

func browserDecorationShorthand(value string) bool {
	for part := range Segments(value, ' ') {
		if part == "" || containsWord("none underline overline line-through blink solid auto from-font currentcolor", strings.ToLower(part)) {
			continue
		}
		if part[0] >= '0' && part[0] <= '9' || part[0] == '.' || part[0] == '-' || strings.HasPrefix(part, "calc(") {
			continue
		}
		return true
	}
	return false
}

func replaceBrowserKeywords(value string, replacements map[string]string) string {
	var output strings.Builder
	for item := range Segments(value, ',') {
		if output.Len() > 0 {
			output.WriteString(", ")
		}
		word := strings.TrimSpace(item)
		if replacement, ok := replacements[word]; ok {
			word = replacement
		}
		output.WriteString(word)
	}
	return output.String()
}

// Query expressions retain grouping instead of substituting strings. A strict
// range becomes a negated inclusive range; in a conjunction that negation needs
// its own parentheses. Values inside functions are opaque to this parser.
type browserQuery struct {
	kind       byte
	name       string
	comparison string
	value      string
	parts      []*browserQuery
}

func lowerBrowserQuery(value string, media bool, custom map[string]string) (string, error) {
	prefix := ""
	if !media {
		custom = nil
	}
	if !media && len(value) > 0 && value[0] != '(' && !strings.HasPrefix(value, "not ") {
		if at := strings.IndexByte(value, ' '); at >= 0 {
			prefix, value = value[:at+1], strings.TrimSpace(value[at+1:])
		}
	}
	var output strings.Builder
	for part := range Segments(value, ',') {
		query, err := parseBrowserQuery(strings.TrimSpace(part), media, custom, nil)
		if err != nil {
			return "", err
		}
		if output.Len() > 0 {
			output.WriteString(", ")
		}
		if media {
			query = simplifyBrowserQuery(query)
		}
		output.WriteString(query.css(0, media))
	}
	return prefix + output.String(), nil
}

func parseBrowserQuery(value string, media bool, custom map[string]string, path []string) (*browserQuery, error) {
	value = strings.TrimSpace(value)
	// A media-type qualifier negates the whole query. `not (condition)` is
	// instead a condition-level negation and can invert a range comparison.
	if media && value != "" && value[0] != '(' && !strings.HasPrefix(value, "not (") {
		name, rest, _ := strings.Cut(value, " and ")
		if len(splitQueryOperator(rest, " or ")) > 1 {
			// A media type allows only a conjunction. Recovery retains invalid
			// conditions verbatim instead of changing their interpretation.
			return &browserQuery{kind: 'v', value: value}, nil
		}
		if name == "all" || name == "not all" {
			if rest != "" {
				query, err := parseBrowserQuery(rest, false, custom, path)
				if name == "not all" && err == nil {
					query = negateBrowserQuery(query)
				}
				return query, err
			}
		}
		query := &browserQuery{kind: 'm', name: name}
		if rest != "" {
			condition, err := parseBrowserQuery(rest, false, custom, path)
			if err != nil {
				return nil, err
			}
			query.parts = []*browserQuery{condition}
		}
		return query, nil
	}
	for _, operator := range []struct {
		word string
		kind byte
	}{{" or ", '|'}, {" and ", '&'}} {
		parts := splitQueryOperator(value, operator.word)
		if len(parts) > 1 {
			query := &browserQuery{kind: operator.kind}
			for _, part := range parts {
				child, err := parseBrowserQuery(part, false, custom, path)
				if err != nil {
					return nil, err
				}
				if child.kind == query.kind {
					query.parts = append(query.parts, child.parts...)
				} else {
					query.parts = append(query.parts, child)
				}
			}
			return query, nil
		}
	}
	if rest, ok := strings.CutPrefix(value, "not "); ok {
		query, err := parseBrowserQuery(rest, false, custom, path)
		if err != nil {
			return nil, err
		}
		return negateBrowserQuery(query), nil
	}
	if len(value) > 1 && value[0] == '(' && queryGroupEnd(value, 0) == len(value) {
		inner := strings.TrimSpace(value[1 : len(value)-1])
		if custom != nil && strings.HasPrefix(inner, "--") && !strings.ContainsAny(inner, " :<>=()") {
			for _, previous := range path {
				if previous == inner {
					return nil, fmt.Errorf("Circular custom media query %s detected", inner)
				}
			}
			definition, ok := custom[inner]
			if !ok {
				return nil, fmt.Errorf("Custom media query %s is not defined", inner)
			}
			query := &browserQuery{kind: '|'}
			for part := range Segments(definition, ',') {
				child, err := parseBrowserQuery(strings.TrimSpace(part), true, custom, append(path, inner))
				if err != nil {
					return nil, err
				}
				query.parts = append(query.parts, child)
			}
			if len(query.parts) == 1 {
				return query.parts[0], nil
			}
			for _, part := range query.parts {
				if part.kind == 'm' {
					return nil, fmt.Errorf("Boolean logic with media types in @custom-media rules is not supported by Lightning CSS")
				}
			}
			return query, nil
		}
		if strings.HasPrefix(inner, "(") || strings.HasPrefix(inner, "not ") || len(splitQueryOperator(inner, " and ")) > 1 || len(splitQueryOperator(inner, " or ")) > 1 {
			return parseBrowserQuery(inner, false, custom, path)
		}
		if query := parseQueryRange(inner); query != nil {
			return query, nil
		}
	}
	return &browserQuery{kind: 'v', value: value}, nil
}

func splitQueryOperator(value, operator string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(value); {
		if value[i] == '(' || value[i] == '\'' || value[i] == '"' {
			i = queryGroupEnd(value, i)
		} else if strings.HasPrefix(value[i:], operator) {
			parts = append(parts, value[start:i])
			i += len(operator)
			start = i
		} else {
			i++
		}
	}
	return append(parts, value[start:])
}

func queryGroupEnd(value string, start int) int {
	end := value[start]
	if end == '(' {
		end = ')'
	}
	for i := start + 1; i < len(value); i++ {
		if value[i] == '\\' {
			i++
		} else if value[i] == end {
			return i + 1
		} else if end == ')' && (value[i] == '(' || value[i] == '\'' || value[i] == '"') {
			i = queryGroupEnd(value, i) - 1
		}
	}
	return len(value)
}

func parseQueryRange(value string) *browserQuery {
	var values, operators []string
	start := 0
	for i := 0; i < len(value); {
		if value[i] == '(' || value[i] == '\'' || value[i] == '"' {
			i = queryGroupEnd(value, i)
		} else if strings.ContainsRune("<>=", rune(value[i])) {
			values = append(values, strings.TrimSpace(value[start:i]))
			end := i + 1
			if end < len(value) && value[end] == '=' {
				end++
			}
			operators = append(operators, value[i:end])
			i, start = end, end
		} else {
			i++
		}
	}
	if len(operators) == 0 {
		name, val, ok := strings.Cut(value, ":")
		name = strings.TrimSpace(name)
		if ok && (strings.HasPrefix(name, "min-") || strings.HasPrefix(name, "max-")) {
			op := ">="
			if strings.HasPrefix(name, "max-") {
				op = "<="
			}
			return &browserQuery{kind: 'r', name: name[4:], comparison: op, value: strings.TrimSpace(val)}
		}
		return nil
	}
	values = append(values, strings.TrimSpace(value[start:]))
	if len(operators) == 2 && queryFeatureName(values[1]) {
		return &browserQuery{kind: '&', parts: []*browserQuery{
			{kind: 'r', name: values[1], comparison: reverseQueryComparison(operators[0]), value: values[0]},
			{kind: 'r', name: values[1], comparison: operators[1], value: values[2]},
		}}
	}
	if len(operators) == 1 {
		if queryFeatureName(values[0]) {
			return &browserQuery{kind: 'r', name: values[0], comparison: operators[0], value: values[1]}
		}
		if queryFeatureName(values[1]) {
			return &browserQuery{kind: 'r', name: values[1], comparison: reverseQueryComparison(operators[0]), value: values[0]}
		}
	}
	return nil
}

func queryFeatureName(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-') {
			return false
		}
	}
	return true
}

func reverseQueryComparison(value string) string {
	switch value {
	case "<":
		return ">"
	case ">":
		return "<"
	case "<=":
		return ">="
	case ">=":
		return "<="
	default:
		return value
	}
}

func simplifyBrowserQuery(query *browserQuery) *browserQuery {
	for i, part := range query.parts {
		query.parts[i] = simplifyBrowserQuery(part)
	}
	if query.kind == 'n' {
		child := query.parts[0]
		if child.kind == 'n' {
			return child.parts[0]
		}
		if child.kind == 'r' && child.comparison != "=" {
			child.comparison = reverseQueryComparison(child.comparison)
			if strings.HasSuffix(child.comparison, "=") {
				child.comparison = strings.TrimSuffix(child.comparison, "=")
			} else {
				child.comparison += "="
			}
			return child
		}
	}
	return query
}

func negateBrowserQuery(query *browserQuery) *browserQuery {
	if query.kind == 'm' && len(query.parts) > 0 {
		query.parts[0] = negateBrowserQuery(query.parts[0])
		return query
	}
	return &browserQuery{kind: 'n', parts: []*browserQuery{query}}
}

func (q *browserQuery) css(parent byte, media bool) string {
	result := ""
	switch q.kind {
	case 'm':
		result = q.name
		if len(q.parts) > 0 {
			result += " and " + q.parts[0].css(0, false)
		}
	case 'r':
		prefix, negative := "", false
		switch q.comparison {
		case ">=":
			prefix = "min-"
		case "<=":
			prefix = "max-"
		case ">":
			prefix, negative = "max-", true
		case "<":
			prefix, negative = "min-", true
		}
		result = "(" + prefix + q.name + ": " + q.value + ")"
		if negative {
			if parent == 0 && media {
				result = "not all and " + result
			} else {
				result = "not " + result
				if parent != 0 {
					result = "(" + result + ")"
				}
			}
		}
	case 'n':
		result = q.parts[0].css('n', false)
		if parent == 0 && media {
			result = "not all and " + result
		} else {
			result = "not " + result
			if parent != 0 {
				result = "(" + result + ")"
			}
		}
	case '&', '|':
		operator := " and "
		if q.kind == '|' {
			operator = " or "
		}
		var output strings.Builder
		for i, part := range q.parts {
			if i > 0 {
				output.WriteString(operator)
			}
			output.WriteString(part.css(q.kind, false))
		}
		result = output.String()
		if parent != 0 && parent != q.kind {
			result = "(" + result + ")"
		}
	default:
		result = q.value
	}
	return result
}
