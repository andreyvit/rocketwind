package rocketwind

import (
	"strings"
)

func (c *Compiler) parseV3Into(parser candidateParser, class string, yield func(Candidate) bool) {
	var local [8]string
	parts := appendSegments(local[:0], class, ':')
	base := parts[len(parts)-1]
	if strings.HasSuffix(base, "!") {
		return
	}
	important := strings.HasPrefix(base, "!")
	base = strings.TrimPrefix(base, "!")
	if strings.HasPrefix(base, "[") {
		if !strings.HasSuffix(base, "]") {
			return
		}
		property, value, ok := strings.Cut(base[1:len(base)-1], ":")
		if !ok || property == "" || value == "" || !(property[0] >= 'a' && property[0] <= 'z' || property[0] == '-' || property[0] == '_') || !v3ValidValue(value) {
			return
		}
		for i := range len(property) {
			if !asciiLetter(property[i]) && !digit(property[i]) && property[i] != '-' && property[i] != '_' {
				return
			}
		}
		if strings.HasPrefix(value, "//") {
			return
		}
		candidate := Candidate{Kind: "arbitrary", Raw: class, Property: property, Value: &CandidateValue{Kind: "arbitrary", Value: v3Normalize(value, property)}, Important: important}
		for i := len(parts) - 2; i >= 0; i-- {
			v := v3ParseVariant(parts[i], parser.variant)
			if v == nil {
				return
			}
			candidate.Variants = append(candidate.Variants, v)
		}
		yield(candidate)
		return
	}
	if prefix := c.v3Prefix; prefix != "" && !strings.HasPrefix(base, "[") {
		negative := strings.HasPrefix(base, "-")
		base = strings.TrimPrefix(base, "-")
		var ok bool
		base, ok = strings.CutPrefix(base, prefix)
		if !ok || strings.HasPrefix(base, "[") {
			return
		}
		if negative {
			base = "-" + base
		}
	}
	for segment := range Segments(base, '/') {
		if strings.HasPrefix(segment, "(") || strings.HasSuffix(segment, ")") && !strings.HasSuffix(segment, "]") {
			return
		}
	}
	if important {
		base = "!" + base
	}
	parts[len(parts)-1] = base
	for i := range len(parts) - 1 {
		variant := appendSegments(nil, parts[i], '/')
		if len(variant) == 2 {
			if d, ok := parser.variant(variant[0]); ok && d.Kind == "static" || strings.HasPrefix(variant[0], "has-") {
				parts[i] = variant[0]
			}
		}
	}
	parser.parseInto(strings.Join(parts, ":"), func(candidate Candidate) bool {
		candidate.Raw = class
		valid := true
		for i := range candidate.Variants {
			candidate.Variants[i] = v3ParseVariant(parts[len(parts)-2-i], parser.variant)
			if candidate.Variants[i] == nil {
				valid = false
				break
			}
		}
		if !valid {
			return true
		}
		if candidate.Value != nil && candidate.Value.Kind == "arbitrary" {
			body := appendSegments(nil, strings.TrimPrefix(base, "!"), '/')[0]
			_, body, _ = strings.Cut(body, "-[")
			body = body[:len(body)-1]
			if candidate.Value.DataType != "" {
				_, body, _ = strings.Cut(body, ":")
			}
			if !v3ValidValue(body) {
				return true
			}
			candidate.Value.Value = v3Normalize(body, "")
		}
		return yield(candidate)
	})
}

func v3ParseVariant(raw string, lookup func(string) (variantDefinition, bool)) *CandidateVariant {
	v := parseVariantSyntax(raw, lookup)
	if v == nil {
		return nil
	}
	if v.Kind == "arbitrary" && !strings.Contains(raw, "&") && !strings.HasPrefix(raw, "[@") {
		return nil
	}
	if (v.Root == "group" || v.Root == "peer") && v.Variant != nil {
		if order, ok := v3VariantOrder[v.Root+"-"+v.Variant.Root]; ok {
			v.definition.Order = order
		}
	}
	if (v.Root == "group" || v.Root == "peer") && v.Variant != nil && v.Variant.Kind == "arbitrary" {
		body := appendSegments(nil, strings.TrimPrefix(raw, v.Root+"-"), '/')[0]
		if !strings.Contains(body, "&") {
			v.Variant.Selector = "&" + v3Normalize(body[1:len(body)-1], "")
		}
	}
	return v
}

func v3ValidValue(value string) bool {
	var local [32]byte
	stack := local[:0]
	quoted := false
	for i := range len(value) {
		ch := value[i]
		if ch == ':' && !quoted && len(stack) == 0 {
			return false
		}
		escaped := i > 0 && value[i-1] == '\\'
		if strings.ContainsRune("\"'`", rune(ch)) && !escaped {
			quoted = !quoted
		}
		if quoted || escaped {
			continue
		}
		if at := strings.IndexByte("{[(", ch); at >= 0 {
			stack = append(stack, "}])"[at])
		} else if strings.ContainsRune("}])", rune(ch)) {
			if len(stack) == 0 || stack[len(stack)-1] != ch {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) == 0
}
