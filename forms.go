package rocketwind

import "strings"

// FormsOptions selects both base styles and form-* classes by default. Strategy
// can be "base" or "class" to generate just one, as in @tailwindcss/forms.
type FormsOptions struct{ Strategy string }

type formsRule struct {
	Base, Classes []string
	Nodes         []*Node
}

// Forms is the Go port of @tailwindcss/forms v0.5.11.
func Forms(options FormsOptions) Plugin {
	return func(api *PluginAPI) error {
		base, classes := options.Strategy == "" || options.Strategy == "base", options.Strategy == "" || options.Strategy == "class"
		var baseRules, classRules []*Node
		for _, rule := range formsRules {
			if !base && !classes {
				break
			}
			nodes := resolveFormsNodes(api, rule.Nodes)
			if base {
				baseRules = append(baseRules, &Node{Kind: "rule", Name: strings.Join(rule.Base, ","), Nodes: nodes})
			}
			if classes && len(rule.Classes) > 0 {
				classRules = append(classRules, &Node{Kind: "rule", Name: strings.Join(rule.Classes, ","), Nodes: nodes})
			}
		}
		if len(baseRules) > 0 {
			api.AddBase(baseRules)
		}
		if len(classRules) > 0 {
			return api.AddComponents(classRules)
		}
		return nil
	}
}

func resolveFormsNodes(api *PluginAPI, nodes []*Node) []*Node {
	result := make([]*Node, len(nodes))
	for i, node := range nodes {
		r := *node
		r.Value = resolveFormsValue(api, r.Value)
		if r.Nodes != nil {
			r.Nodes = resolveFormsNodes(api, r.Nodes)
		}
		result[i] = &r
	}
	return result
}

func resolveFormsValue(api *PluginAPI, value string) string {
	if strings.HasPrefix(value, `url("RW_SVG:`) {
		svg := strings.TrimSuffix(strings.TrimPrefix(value, `url("RW_SVG:`), `")`)
		return `url("` + svgDataURI(resolveFormsValue(api, svg)) + `")`
	}
	var out strings.Builder
	for {
		start := strings.Index(value, "RW_")
		if start < 0 {
			if out.Len() == 0 {
				return value
			}
			out.WriteString(value)
			return out.String()
		}
		open := strings.IndexByte(value[start:], '(') + start
		end := cssBracketEnd(value, open)
		var args []string
		for arg := range Segments(value[open+1:end-1], ',') {
			args = append(args, strings.TrimSpace(arg))
		}
		name := value[start:open]
		resolved := ""
		switch name {
		case "RW_THEME":
			resolved = api.Theme(args[0])
			if resolved == "" {
				resolved = args[1]
			}
		case "RW_COLOR":
			resolved = strings.Replace(resolveFormsValue(api, args[0]), "<alpha-value>", "var("+args[1]+", 1)", 1)
		case "RW_CHEVRON":
			resolved = api.Theme(args[0])
			if resolved == "" || strings.Contains(resolved, "var(") {
				resolved = args[1]
			}
			resolved = strings.Replace(resolved, "<alpha-value>", "1", 1)
		}
		out.WriteString(value[:start])
		out.WriteString(resolved)
		value = value[end:]
	}
}

// svgDataURI follows mini-svg-data-uri's encoding used by the forms plugin.
// Its MIT license is in testdata/plugins/forms/mini-svg-data-uri-LICENSE.
func svgDataURI(svg string) string {
	svg = strings.Join(strings.Fields(strings.TrimPrefix(svg, "\ufeff")), " ")
	svg = strings.ReplaceAll(svg, `"`, `'`)
	var normalized strings.Builder
	for i := 0; i < len(svg); {
		if svg[i] != '#' {
			normalized.WriteByte(svg[i])
			i++
			continue
		}
		end := i + 1
		for end < len(svg) && (asciiLetter(svg[end]) || digit(svg[end]) || svg[end] == '_') {
			end++
		}
		code := strings.ToLower(svg[i+1 : end])
		if len(code) == 4 && code[3] == 'f' {
			code = code[:3]
		} else if len(code) == 8 && code[6:] == "ff" {
			code = code[:6]
		}
		if len(code) == 3 {
			code = string([]byte{code[0], code[0], code[1], code[1], code[2], code[2]})
		}
		if name, found := svgColorNames[code]; found {
			normalized.WriteString(name)
		} else {
			normalized.WriteString(svg[i:end])
		}
		i = end
	}
	const hex = "0123456789abcdef"
	var out strings.Builder
	out.Grow(normalized.Len() + 32)
	out.WriteString("data:image/svg+xml,")
	for _, c := range []byte(normalized.String()) {
		if asciiLetter(c) || digit(c) || strings.ContainsRune(" -_.!~*'()=:/", rune(c)) {
			out.WriteByte(c)
		} else {
			out.WriteByte('%')
			out.WriteByte(hex[c>>4])
			out.WriteByte(hex[c&15])
		}
	}
	return out.String()
}

func standardModule(name string) (Module, bool) {
	if name == "@tailwindcss/typography" {
		return Module{Configure: func(options map[string]any) (Plugin, error) {
			className, _ := options["className"].(string)
			target, _ := options["target"].(string)
			return Typography(TypographyOptions{ClassName: className, Legacy: target == "legacy"}), nil
		}}, true
	}
	if name == "@tailwindcss/forms" {
		return Module{Configure: func(options map[string]any) (Plugin, error) {
			strategy, _ := options["strategy"].(string)
			if _, ok := options["strategy"]; ok && strategy == "" {
				strategy = "invalid"
			}
			return Forms(FormsOptions{Strategy: strategy}), nil
		}}, true
	}
	return Module{}, false
}
