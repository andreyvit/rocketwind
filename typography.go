package rocketwind

import "strings"

// TypographyStyle contains declarations and nested selectors for a prose
// modifier. Name "DEFAULT" defines prose; "lg" defines prose-lg, for example.
type TypographyStyle struct {
	Name  string
	Nodes []*Node
}

// TypographyOptions configures @tailwindcss/typography. ClassName defaults to
// "prose". Legacy disables :where() specificity reduction. A non-nil Styles
// slice replaces the defaults; Extend merges declarations and nested rules.
type TypographyOptions struct {
	ClassName string
	Legacy    bool
	Styles    []TypographyStyle
	Extend    []TypographyStyle
}

var typographyStyles = typographyStylesFromConfig(typographyTheme)
var typographyV3Styles = typographyStylesFromConfig(typographyV3Theme)

func typographyStylesFromConfig(config ConfigObject) []TypographyStyle {
	styles := make([]TypographyStyle, 0, len(config))
	for _, entry := range config {
		var nodes []*Node
		css := configObject(entry.Value).Get("css")
		if array, ok := css.([]any); ok {
			for _, part := range array {
				nodes = mergeTypographyNodes(nodes, configStyleNodes(part))
			}
		} else {
			nodes = configStyleNodes(css)
		}
		styles = append(styles, TypographyStyle{Name: entry.Key, Nodes: nodes})
	}
	return styles
}

// Typography is the Go port of @tailwindcss/typography v0.5.20.
func Typography(options TypographyOptions) Plugin {
	return func(api *PluginAPI) error {
		className := options.ClassName
		if className == "" {
			className = "prose"
		}
		for name := range strings.FieldsSeq("headings h1 h2 h3 h4 h5 h6 p a blockquote figure figcaption strong em kbd code pre ol ul li dl dt dd table thead tr th td img picture video hr lead") {
			selector := name
			switch name {
			case "headings":
				selector = "h1, h2, h3, h4, h5, h6, th"
			case "lead":
				selector = `[class~="lead"]`
			}
			where := func(selector, modifier string) string {
				result := typographyWhere(selector, className, modifier)
				if api.compiler.config.Compatibility == Tailwind3 {
					result = strings.ReplaceAll(result, `"not-`+className+`"`, `"`+strings.TrimPrefix(api.Prefix(".not-"+className), ".")+`"`)
				}
				return result
			}
			var template []*Node
			if options.Legacy {
				for item := range Segments(selector, ',') {
					template = append(template, &Node{Kind: "rule", Name: "& " + strings.TrimSpace(item), Nodes: []*Node{{Kind: "at-rule", Name: "@slot"}}})
				}
			} else {
				selector = "& :is(" + where(selector, "") + ")"
				template = []*Node{{Kind: "rule", Name: selector, Nodes: []*Node{{Kind: "at-rule", Name: "@slot"}}}}
			}
			if err := api.AddVariant(className+"-"+name, template); err != nil {
				return err
			}
		}
		styles := options.Styles
		extraStyles := options.Extend
		if styles == nil {
			styles = typographyStyles
			if api.compiler.config.Compatibility == Tailwind3 {
				styles = typographyV3Styles
			}
			if configured := configObject(api.ThemeValue("typography")); configured != nil {
				configuredStyles := typographyStylesFromConfig(configured)
				if api.compiler.legacyReplaced["typography"] {
					styles = configuredStyles
				} else {
					extraStyles = append(configuredStyles, extraStyles...)
				}
			}
		}
		styles = append([]TypographyStyle(nil), styles...)
		for _, extra := range extraStyles {
			found := false
			for i := range styles {
				if styles[i].Name == extra.Name {
					styles[i].Nodes = mergeTypographyNodes(styles[i].Nodes, extra.Nodes)
					found = true
					break
				}
			}
			if !found {
				styles = append(styles, extra)
			}
		}
		var rules []*Node
		for _, style := range styles {
			class := className
			if style.Name != "DEFAULT" {
				class += "-" + style.Name
			}
			nodes := cloneNodes(style.Nodes, "", false)
			if !options.Legacy {
				for _, node := range nodes {
					if node.Kind == "rule" {
						node.Name = typographyWhere(node.Name, className, style.Name)
						if api.compiler.config.Compatibility == Tailwind3 {
							node.Name = strings.ReplaceAll(node.Name, `"not-`+className+`"`, `"`+strings.TrimPrefix(api.Prefix(".not-"+className), ".")+`"`)
						}
					}
				}
			}
			rules = append(rules, &Node{Kind: "rule", Name: "." + class, Nodes: nodes})
		}
		return api.AddComponents(rules)
	}
}

func mergeTypographyNodes(base, extra []*Node) []*Node {
	result := cloneNodes(base, "", false)
	for i := 0; i < len(extra); {
		node := extra[i]
		end := i + 1
		for end < len(extra) && node.Kind == "declaration" && extra[end].Kind == node.Kind && extra[end].Name == node.Name {
			end++
		}
		found := false
		for j, previous := range result {
			if previous.Kind != node.Kind || previous.Name != node.Name || previous.Kind == "at-rule" && previous.Value != node.Value {
				continue
			}
			found = true
			if node.Kind == "declaration" {
				last := j + 1
				for last < len(result) && result[last].Kind == node.Kind && result[last].Name == node.Name {
					last++
				}
				replacement := cloneNodes(extra[i:end], "", false)
				result = append(result[:j], append(replacement, result[last:]...)...)
			} else {
				previous.Nodes = mergeTypographyNodes(previous.Nodes, node.Nodes)
			}
			break
		}
		if !found {
			result = append(result, cloneNodes(extra[i:end], "", false)...)
		}
		i = end
	}
	return result
}

func typographyWhere(selector, className, modifier string) string {
	prefix := ""
	if strings.HasPrefix(selector, ">") {
		prefix = "." + className
		if modifier != "DEFAULT" {
			prefix += "-" + modifier
		}
		prefix += " "
	}
	pseudo, rebuilt := typographyTrailingPseudos(selector)
	return ":where(" + prefix + rebuilt + `):not(:where([class~="not-` + className + `"],[class~="not-` + className + `"] *))` + pseudo
}

// Keep selector spelling while lifting only a common trailing pseudo-element
// suffix. Pseudo functions compare by name, matching the upstream selector API.
func typographyTrailingPseudos(selector string) (string, string) {
	type pseudo struct {
		start, end int
		name       string
	}
	type part struct {
		text    string
		pseudos []pseudo
		cut     int
	}
	var parts []part
	for text := range Segments(selector, ',') {
		p := part{text: text, cut: len(text)}
		for i := 0; i < len(text); {
			if text[i] == ':' && i+1 < len(text) && text[i+1] == ':' {
				end := cssIdentifierEnd(text, i+2)
				name := text[i:end]
				if end < len(text) && text[end] == '(' {
					end = cssBracketEnd(text, end)
				}
				p.pseudos = append(p.pseudos, pseudo{i, end, name})
				i = end
				continue
			}
			if strings.TrimSpace(text[i:]) == "" {
				if len(p.pseudos) > 0 {
					p.pseudos[len(p.pseudos)-1].end = len(text)
				}
				break
			}
			p.pseudos = p.pseudos[:0]
			switch text[i] {
			case '[', '(':
				i = cssBracketEnd(text, i)
			case '\\':
				i = cssEscapeEnd(text, i)
			default:
				i++
			}
		}
		parts = append(parts, p)
	}
	lifted := ""
	for depth := 0; ; depth++ {
		last := -1
		for i, p := range parts {
			if len(p.pseudos) > depth {
				last = i
			}
		}
		if last < 0 {
			break
		}
		name, same := "", true
		for _, p := range parts[:last+1] {
			if len(p.pseudos) <= depth {
				same = false
				break
			}
			v := p.pseudos[len(p.pseudos)-1-depth].name
			if name != "" && name != v {
				same = false
				break
			}
			name = v
		}
		if !same {
			break
		}
		for i := 0; i <= last; i++ {
			p := &parts[i]
			pseudo := p.pseudos[len(p.pseudos)-1-depth]
			if i == 0 {
				lifted = p.text[pseudo.start:pseudo.end] + lifted
			}
			p.cut = pseudo.start
		}
	}
	if lifted == "" {
		return "", selector
	}
	var result strings.Builder
	for i, p := range parts {
		if i > 0 {
			result.WriteByte(',')
		}
		result.WriteString(p.text[:p.cut])
	}
	return lifted, result.String()
}
