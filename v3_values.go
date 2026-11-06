// Value normalization follows Tailwind CSS v3.4.17's dataTypes module.
// Copyright Tailwind Labs. MIT license: testdata/upstream/LICENSE.
package rocketwind

import (
	"regexp"
	"strings"
)

var v3MathPattern = regexp.MustCompile(`(calc|min|max|clamp)\(.+\)`)
var v3MathKeywords = strings.Fields("min-content max-content fit-content safe-area-inset-top safe-area-inset-right safe-area-inset-bottom safe-area-inset-left titlebar-area-x titlebar-area-y titlebar-area-width titlebar-area-height keyboard-inset-top keyboard-inset-right keyboard-inset-bottom keyboard-inset-left keyboard-inset-width keyboard-inset-height radial-gradient linear-gradient conic-gradient repeating-radial-gradient repeating-linear-gradient repeating-conic-gradient anchor-size")

func v3Normalize(value, property string) string {
	if strings.HasPrefix(value, "--") && !containsWord("scroll-timeline-name timeline-scope view-timeline-name font-palette anchor-name anchor-scope position-anchor position-try-options scroll-timeline animation-timeline view-timeline position-try", property) {
		return "var(" + value + ")"
	}
	var normalize func(string, bool) string
	normalize = func(value string, trim bool) string {
		if start := strings.Index(value, "url("); start >= 0 {
			if end := strings.IndexByte(value[start:], ')'); end >= 0 {
				end += start + 1
				return normalize(value[:start], false) + value[start:end] + normalize(value[end:], false)
			}
		}
		var out strings.Builder
		for i := 0; i < len(value); i++ {
			if value[i] == '\\' && i+1 < len(value) && value[i+1] == '_' {
				out.WriteByte('_')
				i++
			} else if value[i] == '_' {
				out.WriteByte(' ')
			} else {
				out.WriteByte(value[i])
			}
		}
		value = out.String()
		if trim {
			value = strings.TrimSpace(value)
		}
		return v3MathPattern.ReplaceAllStringFunc(value, v3MathSpacing)
	}
	return normalize(value, true)
}

func v3MathSpacing(value string) string {
	var out strings.Builder
	var last byte
	for i := 0; i < len(value); i++ {
		end := i + 1
		stop := ""
		switch {
		case strings.HasPrefix(value[i:], "var"):
			stop = "),"
		case strings.HasPrefix(value[i:], "theme"):
			stop = ")"
		case value[i] == '[':
			stop = "]"
		}
		if stop != "" {
			if at := strings.IndexAny(value[i:], stop); at >= 0 {
				end = i + at
			} else {
				end = len(value)
			}
		} else {
			for _, word := range v3MathKeywords {
				if strings.HasPrefix(value[i:], word) {
					end = i + len(word)
					break
				}
			}
		}
		if end > i+1 {
			out.WriteString(value[i:end])
			last = value[end-1]
			i = end - 1
		} else {
			ch := value[i]
			if strings.ContainsRune("+-*/", rune(ch)) && !strings.ContainsRune("(+-*/,", rune(last)) {
				out.WriteByte(' ')
				out.WriteByte(ch)
				out.WriteByte(' ')
			} else {
				out.WriteByte(ch)
			}
			if !cssSpace(ch) {
				last = ch
			}
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func v3ParsableNodes(nodes []*Node) bool {
	for _, n := range nodes {
		if n.Kind == "declaration" && strings.ContainsAny(n.Value, "{};") {
			parsed, err := ParseCSS("a{"+n.Name+":"+n.Value+"}", "")
			if err != nil || len(parsed) != 1 || len(parsed[0].Nodes) != 1 || parsed[0].Nodes[0].Kind != "declaration" {
				return false
			}
		}
		if !v3ParsableNodes(n.Nodes) {
			return false
		}
	}
	return true
}
