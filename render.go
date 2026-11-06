package rocketwind

import (
	"maps"
	"slices"
	"strings"
)

type Polyfills uint8

const (
	PolyfillAtProperty Polyfills = 1 << iota
	PolyfillColorMix
	PolyfillAll = PolyfillAtProperty | PolyfillColorMix
)

func (b *buildContext) render(nodes []*Node) string {
	nodes = flattenContexts(nodes, "")
	clear(b.used)
	animations := make(map[string]bool)
	markAnimation := func(value string) {
		for _, word := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
			animations[word] = true
		}
	}
	for key, flags := range b.compiler.themeFlags {
		if strings.Contains(flags, "static") {
			b.used[key] = true
		}
	}
	var mark func(string)
	mark = func(s string) {
		for {
			_, tail, ok := strings.Cut(s, "var(")
			if !ok {
				return
			}
			end := strings.IndexAny(tail, ",)")
			if end < 0 {
				return
			}
			name := Unescape(strings.TrimSpace(tail[:end]))
			if p := b.compiler.config.Prefix; p != "" && strings.HasPrefix(name, "--"+p+"-") {
				name = "--" + name[len(p)+3:]
			}
			if b.themeValue(name) != "" {
				b.used[name] = true
			}
			s = tail[end+1:]
		}
	}
	var scan func([]*Node)
	scan = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Kind == "declaration" {
				mark(n.Value)
				if n.Name == "animation" {
					markAnimation(n.Value)
				}
			}
			scan(n.Nodes)
		}
	}
	scan(nodes)
	var globals []*Node
	seen := make(map[string]bool)
	var hoist func([]*Node)
	hoist = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Kind == "at-root" {
				for _, g := range n.Nodes {
					key := g.Name + " " + g.Value
					if !seen[key] {
						seen[key] = true
						globals = append(globals, g)
					}
				}
			} else {
				hoist(n.Nodes)
			}
		}
	}
	hoist(nodes)
	var out strings.Builder
	var propertyDefaults, inheritedDefaults []*Node
	properties := append(slices.Clone(globals), nodes...)
	for _, n := range properties {
		if n.Name == "@property" && b.compiler.config.DisablePolyfills&PolyfillAtProperty == 0 {
			initial := "initial"
			inherits := false
			for _, d := range n.Nodes {
				if d.Name == "inherits" && d.Value == "true" {
					inherits = true
				}
				if d.Name == "initial-value" {
					initial = d.Value
				}
			}
			fallback := &Node{Kind: "declaration", Name: n.Value, Value: initial}
			if inherits {
				inheritedDefaults = append(inheritedDefaults, fallback)
			} else {
				propertyDefaults = append(propertyDefaults, fallback)
			}
		}
	}
	if len(propertyDefaults)+len(inheritedDefaults) > 0 {
		out.WriteString("@layer properties;\n")
	}
	var themeNodes []*Node
	if len(b.used) > 0 {
		for {
			before := len(b.used)
			for key := range b.used {
				mark(b.themeValue(key))
			}
			if before == len(b.used) {
				break
			}
		}
		for key := range b.used {
			if strings.HasPrefix(key, "--animate-") {
				markAnimation(b.themeValue(key))
			}
		}
		keys := slices.Sorted(maps.Keys(b.used))
		if len(b.compiler.themeOrder) > 0 {
			keys = b.compiler.themeOrder
		}
		seen := make(map[string]bool)
		for _, key := range keys {
			if seen[key] || !b.used[key] || b.themeValue(key) == "initial" || strings.Contains(b.compiler.themeFlags[key], "reference") {
				continue
			}
			seen[key] = true
			name := key
			if p := b.compiler.config.Prefix; p != "" {
				name = "--" + p + "-" + key[2:]
			}
			themeNodes = append(themeNodes, &Node{Kind: "declaration", Name: Escape(name), Value: b.themeValue(key)})
			if strings.HasPrefix(key, "--animate-") {
				markAnimation(b.themeValue(key))
			}
		}
	}
	for _, frame := range b.compiler.keyframes {
		if animations[strings.TrimSpace(frame.Value)] {
			globals = append(globals, frame)
		}
	}
	if len(themeNodes) > 0 {
		themeNodes = []*Node{{Kind: "rule", Name: ":root, :host", Nodes: themeNodes}}
	}
	if !b.compiler.hasInput {
		writeCSS(&out, themeNodes, 0)
	} else {
		var expand func([]*Node) []*Node
		expand = func(input []*Node) []*Node {
			var result []*Node
			for _, n := range input {
				if n.Kind == "theme" {
					result = append(result, themeNodes...)
				} else {
					r := *n
					if n.Nodes != nil {
						r.Nodes = expand(n.Nodes)
						if len(r.Nodes) == 0 {
							continue
						}
					}
					result = append(result, &r)
				}
			}
			return result
		}
		nodes = expand(nodes)
	}
	if b.compiler.config.DisablePolyfills&PolyfillColorMix == 0 {
		nodes = b.polyfillColors(nodes, false)
	}
	nodes = flattenRules(nodes, "")
	writeCSS(&out, nodes, 0)
	writeCSS(&out, globals, 0)
	if len(propertyDefaults)+len(inheritedDefaults) > 0 {
		var defaults []*Node
		if len(inheritedDefaults) > 0 {
			defaults = append(defaults, &Node{Kind: "rule", Name: ":root, :host", Nodes: inheritedDefaults})
		}
		if len(propertyDefaults) > 0 {
			defaults = append(defaults, &Node{Kind: "rule", Name: "*, ::before, ::after, ::backdrop", Nodes: propertyDefaults})
		}
		fallback := []*Node{{Kind: "at-rule", Name: "@layer", Value: "properties", Nodes: []*Node{{Kind: "at-rule", Name: "@supports", Value: "((-webkit-hyphens: none) and (not (margin-trim: block))) or ((-moz-orient: inline) and (not (color:rgb(from red r g b))))", Nodes: defaults}}}}
		writeCSS(&out, fallback, 0)
	}
	return out.String()
}

func writeCSS(out *strings.Builder, nodes []*Node, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, n := range nodes {
		if n.Kind == "at-root" || n.Kind == "context" || n.Kind == "declaration" && n.Name == "--tw-sort" {
			continue
		}
		out.WriteString(indent)
		switch n.Kind {
		case "declaration":
			out.WriteString(n.Name)
			out.WriteString(": ")
			out.WriteString(n.Value)
			if n.Important {
				out.WriteString(" !important")
			}
			out.WriteString(";\n")
		case "comment":
			out.WriteString("/*" + n.Value + "*/\n")
		default:
			out.WriteString(n.Name)
			if n.Kind == "at-rule" && n.Value != "" {
				out.WriteByte(' ')
				out.WriteString(n.Value)
			}
			if len(n.Nodes) == 0 && n.Kind == "at-rule" {
				out.WriteString(";\n")
				continue
			}
			out.WriteString(" {\n")
			writeCSS(out, n.Nodes, depth+1)
			out.WriteString(indent)
			out.WriteString("}\n")
		}
	}
}
