// URL behavior follows Tailwind CSS 4.3.4's rewriter, adapted from Vite.
// Copyright (c) 2019-present, VoidZero Inc. and Vite contributors.
// MIT license: testdata/upstream/VITE-LICENSE.
package rocketwind

import (
	"fmt"
	"net/url"
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
	return rewriteURLs(input, func(raw string) (string, bool) { return rewriteAssetURL(raw, base, root) })
}

// ResolveURLs makes relative asset URLs absolute against the stylesheet's
// original URL. Use it when moving CSS to a different route or origin.
// External URLs, root-relative paths, fragments and data URLs are preserved.
func ResolveURLs(input, baseURL string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if !base.IsAbs() {
		return "", fmt.Errorf("stylesheet URL must be absolute: %q", baseURL)
	}
	return rewriteURLs(input, func(raw string) (string, bool) {
		return rewriteAssetURLUsing(raw, func(raw string) (string, error) {
			ref, err := url.Parse(raw)
			if err != nil {
				return "", err
			}
			return base.ResolveReference(ref).String(), nil
		})
	})
}

func rewriteURLs(input string, rewrite func(string) (string, bool)) (string, error) {
	nodes, err := ParseCSS(input, "")
	if err != nil {
		return "", err
	}
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, node := range nodes {
			if node.Kind == "declaration" {
				node.Value = rewriteAssetValue(node.Value, rewrite)
			}
			walk(node.Nodes)
		}
	}
	walk(nodes)
	var out strings.Builder
	writeCSS(&out, nodes, 0)
	return out.String(), nil
}

func rewriteAssetValue(value string, rewrite func(string) (string, bool)) string {
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
				if replacement, ok := rewrite(raw); ok {
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
						if replacement, ok := rewrite(valueCSS(values[:end])); ok {
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
	return rewriteAssetURLUsing(raw, func(raw string) (string, error) {
		absolute := path.Join(strings.ReplaceAll(base, `\`, "/"), raw)
		relative, err := filepath.Rel(strings.ReplaceAll(root, `\`, "/"), absolute)
		if err != nil {
			return "", err
		}
		relative = filepath.ToSlash(relative)
		if !strings.HasPrefix(relative, ".") {
			relative = "./" + relative
		}
		return relative, nil
	})
}

func rewriteAssetURLUsing(raw string, resolve func(string) (string, error)) (string, bool) {
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
	relative, err := resolve(raw)
	if err != nil {
		return "", false
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
