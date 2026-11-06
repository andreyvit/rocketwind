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
	var mappings *[]cssMapping
	if b.track {
		mappings = &b.mappings
	}
	nodes = flattenContexts(nodes, "")
	clear(b.used)
	for _, key := range b.externalVariables {
		b.used[key] = true
	}
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
			_, defined := b.compiler.config.Theme[name]
			if defined || b.themeValue(name) != "" {
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
			fallback := withLocation(&Node{Kind: "declaration", Name: n.Value, Value: initial}, location(n))
			if inherits {
				inheritedDefaults = append(inheritedDefaults, fallback)
			} else {
				propertyDefaults = append(propertyDefaults, fallback)
			}
		}
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
			themeNodes = append(themeNodes, withLocation(&Node{Kind: "declaration", Name: Escape(name), Value: b.themeValue(key)}, b.compiler.themeSources[key]))
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
		themeNodes = []*Node{withLocation(&Node{Kind: "rule", Name: ":root, :host", Nodes: themeNodes}, b.compiler.themeSource)}
	}
	if !b.compiler.hasInput || b.standalone {
		nodes = append(themeNodes, nodes...)
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
	if len(propertyDefaults)+len(inheritedDefaults) > 0 {
		first := propertyDefaults
		if len(inheritedDefaults) > 0 {
			first = inheritedDefaults
		}
		// Imports and charset must precede every layer statement. Keep license
		// comments in place, as Tailwind does when inserting this fallback layer.
		at := 0
		for at < len(nodes) && (nodes[at].Kind == "comment" || nodes[at].Name == "@charset" || nodes[at].Name == "@import") {
			at++
		}
		nodes = slices.Insert(nodes, at, withLocation(&Node{Kind: "at-rule", Name: "@layer", Value: "properties"}, location(first[0])))
	}
	b.writeOutput(&out, nodes, mappings)
	b.writeOutput(&out, globals, mappings)
	if len(propertyDefaults)+len(inheritedDefaults) > 0 {
		var defaults []*Node
		if len(inheritedDefaults) > 0 {
			defaults = append(defaults, withLocation(&Node{Kind: "rule", Name: ":root, :host", Nodes: inheritedDefaults}, location(inheritedDefaults[0])))
		}
		if len(propertyDefaults) > 0 {
			defaults = append(defaults, withLocation(&Node{Kind: "rule", Name: "*, ::before, ::after, ::backdrop", Nodes: propertyDefaults}, location(propertyDefaults[0])))
		}
		fallback := []*Node{{Kind: "at-rule", Name: "@layer", Value: "properties", Nodes: []*Node{{Kind: "at-rule", Name: "@supports", Value: "((-webkit-hyphens: none) and (not (margin-trim: block))) or ((-moz-orient: inline) and (not (color:rgb(from red r g b))))", Nodes: defaults}}}}
		withLocation(fallback[0], location(defaults[0]))
		withLocation(fallback[0].Nodes[0], location(defaults[0]))
		b.writeOutput(&out, fallback, mappings)
	}
	return out.String()
}

func (b *buildContext) writeOutput(out *strings.Builder, nodes []*Node, mappings *[]cssMapping) {
	if b.outputAST != nil {
		*b.outputAST = append(*b.outputAST, nodes...)
	} else {
		writeMappedCSS(out, nodes, 0, mappings)
	}
}

func writeCSS(out *strings.Builder, nodes []*Node, depth int) {
	writeMappedCSS(out, nodes, depth, nil)
}

func writeMappedCSS(out *strings.Builder, nodes []*Node, depth int, mappings *[]cssMapping) {
	indent := strings.Repeat("  ", depth)
	for _, n := range nodes {
		if n.Kind == "at-root" || n.Kind == "context" || n.Kind == "declaration" && n.Name == "--tw-sort" {
			continue
		}
		out.WriteString(indent)
		start, end := out.Len(), 0
		switch n.Kind {
		case "declaration":
			out.WriteString(n.Name)
			out.WriteString(": ")
			out.WriteString(n.Value)
			if n.Important {
				out.WriteString(" !important")
			}
			end = out.Len()
			out.WriteString(";\n")
		case "comment":
			out.WriteString("/*" + n.Value + "*/\n")
			end = out.Len() - 1
		default:
			out.WriteString(n.Name)
			if n.Kind == "at-rule" && (n.Value != "" || len(n.Nodes) == 0) {
				out.WriteByte(' ')
				out.WriteString(n.Value)
			}
			if len(n.Nodes) == 0 && n.Kind == "at-rule" {
				end = out.Len()
				out.WriteString(";\n")
				break
			}
			out.WriteString(" {\n")
			end = out.Len() - 2
			if mappings != nil && n.source != nil {
				*mappings = append(*mappings, cssMapping{location(n), start, end})
			}
			writeMappedCSS(out, n.Nodes, depth+1, mappings)
			out.WriteString(indent)
			out.WriteString("}\n")
			continue
		}
		if mappings != nil && n.source != nil {
			*mappings = append(*mappings, cssMapping{location(n), start, end})
		}
	}
}
