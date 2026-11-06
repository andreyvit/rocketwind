package rocketwind

import (
	"cmp"
	"slices"
	"strings"
)

type variantDefinition struct {
	Kind             string
	Order, Compounds int
	Nodes            []*Node
}

func (b *buildContext) applyVariants(nodes []*Node, variants []string) ([]*Node, bool) {
	for i := len(variants) - 1; i >= 0; i-- {
		var ok bool
		nodes, ok = b.applyVariant(nodes, variants[i])
		if !ok {
			return nil, false
		}
	}
	return nodes, true
}

func slotNodes(template, content []*Node) []*Node {
	var result []*Node
	for _, n := range template {
		if n.Kind == "slot" || n.Name == "@slot" {
			result = append(result, content...)
			continue
		}
		r := *n
		if n.Nodes != nil {
			r.Nodes = slotNodes(n.Nodes, content)
		}
		result = append(result, &r)
	}
	return result
}

func (b *buildContext) applyVariant(nodes []*Node, variant string) ([]*Node, bool) {
	if v, ok := builtinVariants[variant]; ok && v.Kind == "static" {
		return slotNodes(v.Nodes, nodes), true
	}
	if value := b.themeValue("--breakpoint-" + variant); value != "" {
		return []*Node{{Kind: "at-rule", Name: "@media", Value: "(width >= " + value + ")", Nodes: nodes}}, true
	}
	if len(variant) > 2 && variant[0] == '[' && variant[len(variant)-1] == ']' {
		selector := DecodeArbitraryValue(variant[1 : len(variant)-1])
		if !validArbitrary(selector) {
			return nil, false
		}
		if strings.HasPrefix(selector, "@") {
			name, params, _ := strings.Cut(selector, " ")
			return []*Node{{Kind: "at-rule", Name: name, Value: params, Nodes: nodes}}, true
		}
		if strings.HasPrefix(selector, ">") || strings.HasPrefix(selector, "+") || strings.HasPrefix(selector, "~") {
			return nil, false
		}
		if !strings.Contains(selector, "&") {
			selector = "&:is(" + selector + ")"
		}
		return []*Node{{Kind: "rule", Name: selector, Nodes: nodes}}, true
	}
	for _, prefix := range []string{"min-", "max-"} {
		if strings.HasPrefix(variant, prefix) {
			value := strings.TrimPrefix(variant, prefix)
			if strings.HasPrefix(value, "[") {
				value, _, _ = parseArbitrary(value)
			} else {
				value = b.themeValue("--breakpoint-" + value)
			}
			if value == "" || strings.Contains(value, "var(") {
				return nil, false
			}
			op := ">="
			if prefix == "max-" {
				op = "<"
			}
			return []*Node{{Kind: "at-rule", Name: "@media", Value: "(width " + op + " " + value + ")", Nodes: nodes}}, true
		}
	}
	return nil, false
}

func (b *buildContext) variantOrder(v string) (int, string) {
	if d, ok := builtinVariants[v]; ok {
		return d.Order, ""
	}
	if value := b.themeValue("--breakpoint-" + v); value != "" {
		return builtinVariants["min"].Order, value
	}
	if strings.HasPrefix(v, "[") {
		return 10000, v
	}
	for i := len(v); i > 0; i = strings.LastIndexByte(v[:i], '-') {
		if d, ok := builtinVariants[v[:i]]; ok {
			return d.Order, v[i+1:]
		}
	}
	return 0, v
}
func (b *buildContext) compareVariant(a, z string) int {
	ar, av := b.variantOrder(a)
	zr, zv := b.variantOrder(z)
	if v := cmp.Compare(ar, zr); v != 0 {
		return v
	}
	if ar == builtinVariants["min"].Order {
		return compareBreakpoints(av, zv)
	}
	if ar == builtinVariants["max"].Order {
		return -compareBreakpoints(av, zv)
	}
	return naturalCompare(av, zv)
}
func (b *buildContext) compareVariantSets(a, z []string) int {
	a = slices.Clone(a)
	z = slices.Clone(z)
	reverse := func(a, z string) int { return -b.compareVariant(a, z) }
	slices.SortFunc(a, reverse)
	slices.SortFunc(z, reverse)
	a = slices.Compact(a)
	z = slices.Compact(z)
	for i := 0; i < min(len(a), len(z)); i++ {
		if v := b.compareVariant(a[i], z[i]); v != 0 {
			return v
		}
	}
	return cmp.Compare(len(a), len(z))
}
