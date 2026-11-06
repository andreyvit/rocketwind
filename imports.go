package rocketwind

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed data/theme.css data/preflight.css
var builtins embed.FS

// Stylesheet is an imported stylesheet. Base is used for its relative imports.
type Stylesheet struct{ Path, Base, Content string }
type StylesheetLoader func(id, base string) (Stylesheet, error)

func loadBuiltinStylesheet(id string) (Stylesheet, bool) {
	name := strings.TrimSuffix(id, ".css")
	switch name {
	case "tailwindcss":
		return Stylesheet{Base: "rocketwind:tailwindcss", Content: "@layer theme, base, components, utilities;\n@import 'tailwindcss/theme' layer(theme);\n@import 'tailwindcss/preflight' layer(base);\n@import 'tailwindcss/utilities' layer(utilities);"}, true
	case "tailwindcss/theme", "tailwindcss/preflight":
		data, _ := builtins.ReadFile("data/" + strings.TrimPrefix(name, "tailwindcss/") + ".css")
		return Stylesheet{Base: "rocketwind:tailwindcss", Content: string(data)}, true
	case "tailwindcss/utilities":
		return Stylesheet{Content: "@tailwind utilities;"}, true
	}
	return Stylesheet{}, false
}
func resolveImports(nodes []*Node, config Config, base string, depth int) ([]*Node, error) {
	var result []*Node
	for _, n := range nodes {
		n.base = base
		if n.Kind == "at-rule" && (strings.EqualFold(n.Name, "@import") || n.Name == "@reference") {
			uri, layer, media, supports, valid, err := parseImportParams(n.Value)
			if err != nil {
				return nil, err
			}
			if !valid || strings.HasPrefix(uri, "data:") || strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
				result = append(result, n)
				continue
			}
			if n.Name == "@reference" {
				media = "reference"
			}
			if depth > 100 {
				return nil, fmt.Errorf("Exceeded maximum recursion depth while resolving `%s` in `%s`)", uri, base)
			}
			var source Stylesheet
			if config.LoadStylesheet != nil {
				source, err = config.LoadStylesheet(uri, base)
			} else {
				var found bool
				source, found = loadBuiltinStylesheet(uri)
				if !found {
					err = fmt.Errorf("No `loadStylesheet` function provided to `compile`")
				}
			}
			if err != nil {
				return nil, err
			}
			ast, err := ParseCSS(source.Content, "")
			if err != nil {
				return nil, err
			}
			ast, err = resolveImports(ast, config, source.Base, depth+1)
			if err != nil {
				return nil, err
			}
			if layer != nil {
				ast = []*Node{{Kind: "at-rule", Name: "@layer", Value: *layer, Nodes: ast}}
			}
			if media != "" {
				ast = []*Node{{Kind: "at-rule", Name: "@media", Value: media, Nodes: ast}}
			}
			if supports != "" {
				if !strings.HasPrefix(supports, "(") {
					supports = "(" + supports + ")"
				}
				ast = []*Node{{Kind: "at-rule", Name: "@supports", Value: supports, Nodes: ast}}
			}
			result = append(result, ast...)
			continue
		}
		if n.Nodes != nil {
			var err error
			n.Nodes, err = resolveImports(n.Nodes, config, base, depth)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, n)
	}
	return result, nil
}
func parseImportParams(s string) (uri string, layer *string, media, supports string, valid bool, err error) {
	nodes := parseValue(s)
	for i, n := range nodes {
		if n.kind == 's' {
			continue
		}
		if n.kind == 'w' && uri == "" {
			if len(n.value) < 2 || n.value[0] != '\'' && n.value[0] != '"' {
				return
			}
			uri = n.value[1 : len(n.value)-1]
			continue
		}
		if n.kind == 'f' && strings.EqualFold(n.value, "url") {
			return
		}
		if uri == "" {
			return
		}
		if (n.kind == 'w' || n.kind == 'f') && strings.EqualFold(n.value, "layer") {
			if layer != nil && *layer != "" {
				return
			}
			if supports != "" {
				err = fmt.Errorf("`layer(…)` in an `@import` should come before any other functions or conditions")
				return
			}
			v := valueCSS(n.nodes)
			layer = &v
			continue
		}
		if n.kind == 'f' && strings.EqualFold(n.value, "supports") {
			if supports != "" {
				return
			}
			supports = valueCSS(n.nodes)
			continue
		}
		media = valueCSS(nodes[i:])
		break
	}
	valid = uri != ""
	return
}
