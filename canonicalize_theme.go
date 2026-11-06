package rocketwind

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var canonicalIntegerPercent = regexp.MustCompile(`^\d+%$`)
var canonicalDecimalOpacity = regexp.MustCompile(`^0?\.\d+$`)

func (k *Canonicalizer) convertTheme(input string, migrateModifier, themeOnly bool) (string, *CandidateModifier) {
	ast := parseValue(input)
	uses, modifiers := 0, 0
	var analyze func([]*valueNode)
	analyze = func(nodes []*valueNode) {
		for _, n := range nodes {
			if n.kind == 'f' && n.value == "theme" {
				uses++
				for _, child := range n.nodes {
					if child.kind == 's' && strings.Contains(child.value, ",") {
						break
					}
					if child.kind == 'w' && child.value == "/" {
						modifiers++
						break
					}
				}
			}
			analyze(n.nodes)
		}
	}
	analyze(ast)
	if uses == 0 {
		return input, nil
	}
	var modifier *CandidateModifier
	convert := func(path, fallback string) string {
		if themeOnly || modifiers > 1 {
			return k.themeFunction(path, fallback)
		}
		if modifiers == 0 {
			return k.themeVariable(path, fallback)
		}
		parts := slices.Collect(Segments(path, '/'))
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) > 2 {
			return ""
		}
		if len(ast) == 1 && len(parts) == 2 && migrateModifier {
			value := parts[1]
			if canonicalIntegerPercent.MatchString(value) {
				n, _ := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
				modifier = k.opacityModifier(n)
			} else if canonicalDecimalOpacity.MatchString(value) {
				n, _ := strconv.ParseFloat(value, 64)
				modifier = k.opacityModifier(canonicalPercentage(n))
			} else {
				modifier = &CandidateModifier{Kind: "arbitrary", Value: value}
			}
			path = parts[0]
		}
		if replacement := k.themeVariable(path, fallback); replacement != "" {
			return replacement
		}
		return k.themeFunction(path, fallback)
	}
	var walk func([]*valueNode) []*valueNode
	walk = func(nodes []*valueNode) []*valueNode {
		var result []*valueNode
		for i, n := range nodes {
			if n.kind == 'f' && n.value == "theme" && len(n.nodes) > 0 {
				if n.nodes[0].kind == 's' && strings.TrimSpace(n.nodes[0].value) == "" {
					n.nodes = n.nodes[1:]
				}
				if len(n.nodes) > 0 && n.nodes[0].kind == 'w' {
					path := n.nodes[0].value
					skip := 1
					for j := 1; j < len(n.nodes); j++ {
						if strings.Contains(n.nodes[j].value, ",") {
							break
						}
						path += valueCSS(n.nodes[j : j+1])
						skip = j + 1
					}
					path = canonicalUnquote(path)
					fallback := ""
					if skip+1 < len(n.nodes) {
						fallback = valueCSS(n.nodes[skip+1:])
					}
					if replacement := convert(path, fallback); replacement != "" {
						for j := i - 1; j >= 0; j-- {
							previous := nodes[j]
							if previous.kind == 's' && strings.TrimSpace(previous.value) == "" {
								continue
							}
							v := strings.TrimSpace(previous.value)
							if len(v) == 1 && strings.Contains("-+*/", v) {
								replacement = "(" + replacement + ")"
							}
							break
						}
						result = append(result, walk(parseValue(replacement))...)
						continue
					}
				}
			}
			if n.nodes != nil {
				n.nodes = walk(n.nodes)
			}
			result = append(result, n)
		}
		return result
	}
	return valueCSS(walk(ast)), modifier
}

func canonicalUnquote(value string) string {
	if len(value) < 2 || value[0] != '\'' && value[0] != '"' {
		return value
	}
	var b strings.Builder
	for i := 1; i < len(value)-1; i++ {
		if value[i] == '\\' && (value[i+1] == value[0] || value[i+1] == '\\') {
			i++
		}
		b.WriteByte(value[i])
	}
	return b.String()
}

func canonicalPercentage(value float64) float64 {
	s := strconv.FormatFloat(value*100, 'f', 10, 64)
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

func (k *Canonicalizer) opacityModifier(percentage float64) *CandidateModifier {
	if math.Trunc(percentage) == percentage {
		value := k.themeValue("--opacity-" + jsFloat(percentage))
		if value == "" {
			return &CandidateModifier{Kind: "named", Value: jsFloat(percentage)}
		}
		n, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if !strings.HasSuffix(value, "%") {
			n = canonicalPercentage(n)
		}
		if err == nil && n == percentage {
			return &CandidateModifier{Kind: "named", Value: jsFloat(percentage)}
		}
	}
	return &CandidateModifier{Kind: "arbitrary", Value: jsFloat(percentage) + "%"}
}

func (k *Canonicalizer) themeVariable(path, fallback string) string {
	key := legacyThemeKey(path)
	if k.themeValue(key) != "" {
		if p := k.compiler.config.Prefix; p != "" {
			key = "--" + p + "-" + key[2:]
		}
		if fallback != "" {
			return "var(" + key + ", " + fallback + ")"
		}
		return "var(" + key + ")"
	}
	parts := themeKeyPath(path)
	if len(parts) > 1 && parts[0] == "spacing" && k.themeValue("--spacing") != "" && quarterNumber(parts[1]) {
		return "--spacing(" + parts[1] + ")"
	}
	return ""
}

func (k *Canonicalizer) themeFunction(path, fallback string) string {
	parts := slices.Collect(Segments(path, '/'))
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	key := legacyThemeKey(parts[0])
	if k.themeValue(key) == "" {
		return ""
	}
	if len(parts) > 1 {
		key += "/" + strings.Join(parts[1:], "/")
	}
	if fallback != "" {
		key += ", " + fallback
	}
	return "--theme(" + key + ")"
}
