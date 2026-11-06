// Candidate syntax is adapted from Tailwind CSS v4.3.4. See testdata/upstream/LICENSE.
package rocketwind

import (
	"iter"
	"strings"
)

// Candidate is one interpretation of a utility class. A name may match more
// than one registered utility. Variants are stored from innermost to outermost.
type Candidate struct {
	Kind, Root, Raw, Property string
	Value                     *CandidateValue
	Modifier                  *CandidateModifier
	Variants                  []*CandidateVariant
	Important                 bool
}

type CandidateValue struct{ Kind, Value, DataType, Fraction string }
type CandidateModifier struct{ Kind, Value string }
type CandidateVariant struct {
	Kind, Root, Selector string
	Value                *CandidateValue
	Modifier             *CandidateModifier
	Variant              *CandidateVariant
	Relative             bool
	definition           variantDefinition
}

type candidateParser struct {
	prefix     string
	hasUtility func(string, string) bool
	variant    func(string) (variantDefinition, bool)
}

// Parse yields the registered interpretations of a class without compiling CSS.
func (c *Compiler) Parse(class string) iter.Seq[Candidate] {
	return func(yield func(Candidate) bool) { c.parseInto(class, yield) }
}

func (c *Compiler) parseInto(class string, yield func(Candidate) bool) {
	if len(c.config.Aliases) == 0 {
		c.parseUnaliased(class, yield)
		return
	}
	resolved, kind := c.aliased(class)
	c.parseUnaliased(resolved, func(candidate Candidate) bool {
		candidate.Raw = class
		if candidate.Value != nil && kind != "" {
			value := *candidate.Value
			value.DataType = string(kind)
			candidate.Value = &value
		}
		return yield(candidate)
	})
}

func (c *Compiler) parseUnaliased(class string, yield func(Candidate) bool) {
	parser := candidateParser{c.config.Prefix, c.hasUtility, c.variantDefinition}
	if c.config.Compatibility == Tailwind3 {
		c.parseV3Into(parser, class, yield)
	} else {
		parser.parseInto(class, yield)
	}
}

func (c *Compiler) hasUtility(name, kind string) bool {
	if c.config.Compatibility == Tailwind3 {
		if kind == "static" {
			return v3Static[name] != nil || c.custom[name] != nil || c.customErrors[name] != nil || name == "container"
		}
		if c.customFunctional[name] != nil || c.matchingUtilities[name] != nil {
			return true
		}
		for _, d := range v3Functional[strings.TrimPrefix(name, "-")] {
			if name[0] != '-' || d.Negative {
				return true
			}
		}
		return false
	}
	if kind == "static" {
		return builtinStatic[name] != nil || c.custom[name] != nil || c.customErrors[name] != nil || name == "container"
	}
	if c.customFunctional[name] != nil || c.matchingUtilities[name] != nil || manualTemplateIndex[name] != nil {
		return true
	}
	negative := strings.HasPrefix(name, "-")
	for _, d := range builtinFunctional[strings.TrimPrefix(name, "-")] {
		if !negative || d.Negative {
			return true
		}
	}
	return false
}

type candidateRoot struct {
	name, value string
	hasValue    bool
}

func candidateRoots(input string, exists func(string) bool) iter.Seq[candidateRoot] {
	return func(yield func(candidateRoot) bool) { visitCandidateRoots(input, exists, yield) }
}

func visitCandidateRoots(input string, exists func(string) bool, yield func(candidateRoot) bool) {
	if exists(input) && !yield(candidateRoot{name: input}) {
		return
	}
	for index := strings.LastIndexByte(input, '-'); index > 0; index = strings.LastIndexByte(input[:index], '-') {
		name, value := input[:index], input[index+1:]
		if !exists(name) {
			continue
		}
		if value == "" || name == "@" {
			break
		}
		if !yield(candidateRoot{name, value, true}) {
			return
		}
	}
	if strings.HasPrefix(input, "@") && exists("@") {
		yield(candidateRoot{"@", input[1:], true})
	}
}

func (p candidateParser) parse(input string) iter.Seq[Candidate] {
	return func(yield func(Candidate) bool) { p.parseInto(input, yield) }
}

func (p candidateParser) parseInto(input string, yield func(Candidate) bool) {
	var local [8]string
	parts := appendSegments(local[:0], input, ':')
	if p.prefix != "" {
		if len(parts) == 1 || parts[0] != p.prefix {
			return
		}
		parts = parts[1:]
	}
	base := parts[len(parts)-1]
	variants := make([]*CandidateVariant, 0, len(parts)-1)
	for i := len(parts) - 2; i >= 0; i-- {
		v := parseVariantSyntax(parts[i], p.variant)
		if v == nil {
			return
		}
		variants = append(variants, v)
	}
	important := false
	if strings.HasSuffix(base, "!") {
		base = base[:len(base)-1]
		important = true
	} else if strings.HasPrefix(base, "!") {
		base = base[1:]
		important = true
	}
	candidate := Candidate{Raw: input, Variants: variants, Important: important}
	if p.hasUtility(base, "static") && !strings.Contains(base, "[") {
		candidate.Kind, candidate.Root = "static", base
		if !yield(candidate) {
			return
		}
	}
	parts = appendSegments(parts[:0], base, '/')
	if len(parts) > 2 {
		return
	}
	base = parts[0]
	if len(parts) == 2 {
		candidate.Modifier = parseCandidateModifier(parts[1])
		if candidate.Modifier == nil {
			return
		}
	}
	if strings.HasPrefix(base, "[") {
		if !strings.HasSuffix(base, "]") || len(base) < 4 {
			return
		}
		if base[1] != '-' && (base[1] < 'a' || base[1] > 'z') {
			return
		}
		property, value, ok := strings.Cut(base[1:len(base)-1], ":")
		if !ok || property == "" || value == "" || !validArbitrary(value) {
			return
		}
		candidate.Kind, candidate.Root, candidate.Property = "arbitrary", "", property
		candidate.Value = &CandidateValue{Kind: "arbitrary", Value: DecodeArbitraryValue(value)}
		yield(candidate)
		return
	}
	exists := func(root string) bool { return p.hasUtility(root, "functional") }
	if strings.HasSuffix(base, "]") || strings.HasSuffix(base, ")") {
		index := strings.Index(base, "-[")
		if strings.HasSuffix(base, ")") {
			index = strings.Index(base, "-(")
		}
		if index < 0 || !exists(base[:index]) {
			return
		}
		root := candidateRoot{base[:index], base[index+1:], true}
		yieldCandidateRoot(candidate, root, parts, yield)
		return
	}
	visitCandidateRoots(base, exists, func(root candidateRoot) bool {
		return yieldCandidateRoot(candidate, root, parts, yield)
	})
}

func yieldCandidateRoot(candidate Candidate, root candidateRoot, parts []string, yield func(Candidate) bool) bool {
	candidate.Kind, candidate.Root, candidate.Value = "functional", root.name, nil
	if !root.hasValue {
		if !yield(candidate) {
			return false
		}
		return true
	}
	value := root.value
	if strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		body := value[1 : len(value)-1]
		var local [3]string
		parts := appendSegments(local[:0], body, ':')
		hint := ""
		if len(parts) == 2 {
			hint, body = parts[0]+":", parts[1]
		}
		if !strings.HasPrefix(body, "--") || !validArbitrary(body) {
			return false
		}
		value = "[" + hint + "var(" + body + ")]"
	}
	if start := strings.IndexByte(value, '['); start >= 0 {
		if !strings.HasSuffix(value, "]") {
			return false
		}
		decoded, hint, ok := parseArbitrary(value[start:])
		if !ok {
			return true
		}
		candidate.Value = &CandidateValue{Kind: "arbitrary", Value: decoded, DataType: hint}
	} else {
		if value == "" || !validNamed(value) {
			return true
		}
		v := &CandidateValue{Kind: "named", Value: value}
		if len(parts) == 2 && candidate.Modifier.Kind != "arbitrary" {
			v.Fraction = value + "/" + parts[1]
		}
		candidate.Value = v
	}
	if !yield(candidate) {
		return false
	}
	return true
}

func parseCandidateModifier(raw string) *CandidateModifier {
	if len(raw) > 1 && raw[0] == '[' && raw[len(raw)-1] == ']' {
		body := raw[1 : len(raw)-1]
		if !validArbitrary(body) {
			return nil
		}
		body = DecodeArbitraryValue(body)
		if strings.TrimSpace(body) == "" {
			return nil
		}
		return &CandidateModifier{"arbitrary", body}
	}
	if len(raw) > 1 && raw[0] == '(' && raw[len(raw)-1] == ')' {
		body := raw[1 : len(raw)-1]
		if !strings.HasPrefix(body, "--") || !validArbitrary(body) {
			return nil
		}
		return &CandidateModifier{"arbitrary", DecodeArbitraryValue("var(" + body + ")")}
	}
	if raw == "" || !validNamed(raw) {
		return nil
	}
	return &CandidateModifier{"named", raw}
}

func parseVariantSyntax(raw string, lookup func(string) (variantDefinition, bool)) *CandidateVariant {
	if len(raw) > 1 && raw[0] == '[' && raw[len(raw)-1] == ']' {
		body := raw[1 : len(raw)-1]
		if !validArbitrary(body) || strings.HasPrefix(body, "@") && strings.Contains(body, "&") {
			return nil
		}
		body = DecodeArbitraryValue(body)
		if strings.TrimSpace(body) == "" {
			return nil
		}
		relative := strings.ContainsRune(">+~", rune(body[0]))
		if !relative && body[0] != '@' && !strings.Contains(body, "&") {
			body = "&:is(" + body + ")"
		}
		compounds := 2
		if strings.HasPrefix(body, "@") {
			if strings.HasPrefix(body, "@media") || strings.HasPrefix(body, "@supports") || strings.HasPrefix(body, "@container") {
				compounds = 1
			} else {
				compounds = 0
			}
		} else if strings.Contains(body, "::") {
			compounds = 0
		}
		return &CandidateVariant{Kind: "arbitrary", Selector: body, Relative: relative, definition: variantDefinition{Kind: "arbitrary", Order: 10000, Compounds: compounds}}
	}
	var local [3]string
	parts := appendSegments(local[:0], raw, '/')
	if len(parts) > 2 {
		return nil
	}
	for root := range candidateRoots(parts[0], func(name string) bool { _, ok := lookup(name); return ok }) {
		d, _ := lookup(root.name)
		v := &CandidateVariant{Kind: d.Kind, Root: root.name, definition: d}
		modifier := ""
		if len(parts) == 2 {
			modifier = parts[1]
		}
		switch d.Kind {
		case "static":
			if root.hasValue || len(parts) == 2 {
				return nil
			}
			return v
		case "functional":
			if len(parts) == 2 {
				v.Modifier = parseCandidateModifier(modifier)
				if v.Modifier == nil {
					return nil
				}
			}
			if !root.hasValue {
				return v
			}
			value := root.value
			if strings.HasSuffix(value, "]") || strings.HasSuffix(value, ")") {
				first, last := value[0], value[len(value)-1]
				if first != '[' && last == ']' || first != '(' && last == ')' {
					continue
				}
				body := value[1 : len(value)-1]
				if !validArbitrary(body) {
					return nil
				}
				body = DecodeArbitraryValue(body)
				if strings.TrimSpace(body) == "" {
					return nil
				}
				if first == '(' {
					if !strings.HasPrefix(body, "--") {
						return nil
					}
					body = "var(" + body + ")"
				}
				v.Value = &CandidateValue{Kind: "arbitrary", Value: body}
				return v
			}
			if value == "" || !validNamed(value) {
				continue
			}
			v.Value = &CandidateValue{Kind: "named", Value: value}
			return v
		case "compound":
			if !root.hasValue {
				return nil
			}
			value := root.value
			if modifier != "" && (root.name == "not" || root.name == "has" || root.name == "in") {
				value += "/" + modifier
				modifier = ""
			} else if len(parts) == 2 {
				v.Modifier = parseCandidateModifier(modifier)
				if v.Modifier == nil {
					return nil
				}
			}
			v.Variant = parseVariantSyntax(value, lookup)
			if v.Variant == nil || d.CompoundsWith&v.Variant.definition.Compounds == 0 {
				return nil
			}
			return v
		}
	}
	return nil
}
