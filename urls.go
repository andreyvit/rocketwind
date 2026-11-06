// URL behavior follows Tailwind CSS 4.3.4's rewriter, adapted from Vite.
// Copyright (c) 2019-present, VoidZero Inc. and Vite contributors.
// MIT license: testdata/upstream/VITE-LICENSE.
package rocketwind

import (
	"path"
	"path/filepath"
	"strings"
)

// RewriteURLs rebases relative asset URLs from base to root, including
// image-set() candidates. External URLs, fragments and CSS functions remain.
func RewriteURLs(input, base, root string) (string, error) {
	if !strings.Contains(input, "url(") && !strings.Contains(input, "image-set(") {
		return input, nil
	}
	// Node's path.relative resolves both paths against the working directory.
	base, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	nodes, err := ParseCSS(input, "")
	if err != nil {
		return "", err
	}
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, node := range nodes {
			if node.Kind == "declaration" {
				node.Value = rewriteAssetValue(node.Value, base, root)
			}
			walk(node.Nodes)
		}
	}
	walk(nodes)
	var out strings.Builder
	writeCSS(&out, nodes, 0)
	return out.String(), nil
}

func rewriteAssetValue(value, base, root string) string {
	if !strings.Contains(value, "url(") && !strings.Contains(value, "image-set(") {
		return value
	}
	nodes := parseValue(value)
	var walk func([]*valueNode)
	walk = func(nodes []*valueNode) {
		for _, node := range nodes {
			if node.kind != 'f' {
				continue
			}
			switch node.value {
			case "url":
				raw := strings.TrimSpace(valueCSS(node.nodes))
				if replacement, ok := rewriteAssetURL(raw, base, root); ok {
					node.kind = 'w'
					node.value = replacement
					node.nodes = nil
				}
			case "image-set", "-webkit-image-set":
				var items []string
				for part := range Segments(valueCSS(node.nodes), ',') {
					part = strings.TrimSpace(part)
					values := parseValue(part)
					if len(values) == 0 {
						continue
					}
					if values[0].kind == 'f' {
						walk(values)
					} else {
						end := 0
						for end < len(values) && values[end].kind != 's' {
							end++
						}
						if replacement, ok := rewriteAssetURL(valueCSS(values[:end]), base, root); ok {
							values = append([]*valueNode{{kind: 'w', value: replacement}}, values[end:]...)
						}
					}
					items = append(items, valueCSS(values))
				}
				node.kind = 'w'
				node.value += "(" + strings.Join(items, ", ") + ")"
				node.nodes = nil
			default:
				walk(node.nodes)
			}
		}
	}
	walk(nodes)
	return valueCSS(nodes)
}

func rewriteAssetURL(raw, base, root string) (string, bool) {
	quote := byte(0)
	if len(raw) > 1 && (raw[0] == '\'' || raw[0] == '"') {
		quote = raw[0]
		raw = raw[1 : len(raw)-1]
	}
	external := strings.HasPrefix(raw, "//")
	if at := strings.Index(raw, "://"); at > 0 {
		external = true
		for _, ch := range raw[:at] {
			if ch < 'a' || ch > 'z' {
				external = false
				break
			}
		}
	}
	if raw == "" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:") || external || raw[0] != '.' && raw[0] != '_' && !asciiLetter(raw[0]) && !digit(raw[0]) {
		return "", false
	}
	if at := strings.IndexByte(raw, '('); at > 0 {
		function := true
		for _, ch := range raw[:at] {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '.' || ch == '-') {
				function = false
				break
			}
		}
		if function {
			return "", false
		}
	}
	absolute := path.Join(strings.ReplaceAll(base, `\`, "/"), raw)
	relative, err := filepath.Rel(strings.ReplaceAll(root, `\`, "/"), absolute)
	if err != nil {
		return "", false
	}
	relative = filepath.ToSlash(relative)
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	needsQuote := false
	for _, ch := range relative {
		if ch > 127 || !asciiLetter(byte(ch)) && !digit(byte(ch)) && !strings.ContainsRune(";,/?:@&=+$-_.!~*'()#", ch) {
			needsQuote = true
			break
		}
	}
	if quote == 0 && needsQuote || quote == '\'' && strings.ContainsRune(relative, '\'') {
		quote = '"'
	}
	if quote == '"' {
		var escaped strings.Builder
		for i, ch := range relative {
			if ch == '"' && (i == 0 || relative[i-1] != '\\') {
				escaped.WriteByte('\\')
			}
			escaped.WriteRune(ch)
		}
		relative = escaped.String()
	}
	if quote != 0 {
		relative = string(quote) + relative + string(quote)
	}
	return "url(" + relative + ")", true
}
