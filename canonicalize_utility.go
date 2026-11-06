package rocketwind

import (
	"iter"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

func copyCandidate(c Candidate) Candidate {
	if c.Value != nil {
		v := *c.Value
		c.Value = &v
	}
	if c.Modifier != nil {
		m := *c.Modifier
		c.Modifier = &m
	}
	return c
}

func (k *Canonicalizer) canonicalUtility(c Candidate) Candidate {
	c = copyCandidate(c)
	if c.Kind == "static" && strings.HasPrefix(c.Root, "bg-gradient-to-") && slices.Contains([]string{"t", "tr", "r", "br", "b", "bl", "l", "tl"}, strings.TrimPrefix(c.Root, "bg-gradient-to-")) {
		c.Root = strings.Replace(c.Root, "bg-gradient-to-", "bg-linear-to-", 1)
	}
	if c.Value != nil && (c.Kind == "arbitrary" || c.Value.Kind == "arbitrary") {
		value, modifier := k.convertTheme(c.Value.Value, c.Modifier == nil, false)
		if value != c.Value.Value {
			c.Value.Value = value
			if modifier != nil {
				c.Modifier = modifier
			}
		}
		c.Value.Value = k.spacingFunction(c.Value.Value)
	}
	c = k.optimizeExpression(c)
	if c.Kind == "arbitrary" || c.Kind == "functional" && c.Value != nil {
		k.indexUtilities()
		arbitrary := c.Kind == "arbitrary" || c.Value.Kind == "arbitrary"
		if sig := k.signature(k.print(c)).css; sig != "" {
			for replacement := range k.replacements(sig, c, arbitrary) {
				if k.same(c, replacement) && (!arbitrary || k.variablesPreserved(c, replacement)) {
					c = replacement
					break
				}
			}
		}
	}
	name := k.unprefixed(c)
	replacement := ""
	switch name {
	case "order-none":
		replacement = "order-0"
	case "break-words":
		replacement = "wrap-break-word"
	case "overflow-ellipsis":
		replacement = "text-ellipsis"
	}
	for _, root := range []string{"start", "end"} {
		prefix := ""
		base := name
		if strings.HasPrefix(base, "-") {
			prefix = "-"
			base = base[1:]
		}
		if rest, ok := strings.CutPrefix(base, root+"-"); ok {
			replacement = prefix + "inset-" + root[:1] + "-" + rest
		}
	}
	if replacement != "" && k.sameNames(k.print(c), replacement) {
		for r := range k.parse(replacement) {
			c = r
			break
		}
	}
	if c.Kind == "functional" && c.Value != nil && c.Value.Kind == "arbitrary" {
		if c.Value.DataType != "" {
			r := copyCandidate(c)
			r.Value.DataType = ""
			if k.same(c, r) {
				c = r
			}
		}
		for value := range canonicalBareValues(c.Value.Value, map[string]bool{}) {
			r := c
			r.Value = &value
			if k.same(c, r) {
				c = r
				break
			}
		}
	}
	if c.Modifier != nil && (c.Kind == "functional" || c.Kind == "arbitrary") {
		r := c
		r.Modifier = nil
		if k.same(c, r) {
			return r
		}
		value := c.Modifier.Value
		if strings.HasSuffix(value, "%") {
			value = strings.TrimSuffix(value, "%")
			if strings.Contains(value, ".") {
				n, err := strconv.ParseFloat(value, 64)
				if err != nil {
					n = math.NaN()
				}
				value = jsFloat(n)
			}
		}
		r.Modifier = &CandidateModifier{Kind: "named", Value: value}
		if k.same(c, r) {
			return r
		}
		n := canonicalParseFloat(c.Modifier.Value)
		r.Modifier = &CandidateModifier{Kind: "named", Value: jsFloat(n * 100)}
		if k.same(c, r) {
			return r
		}
	}
	return c
}

func canonicalParseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	for end := len(s); end > 0; end-- {
		if n, err := strconv.ParseFloat(s[:end], 64); err == nil {
			return n
		}
	}
	return math.NaN()
}

func (k *Canonicalizer) replacements(signature string, c Candidate, dynamic bool) iter.Seq[Candidate] {
	return func(yield func(Candidate) bool) {
		found := k.utilities[signature]
		if len(found) > 1 {
			positive := ""
			for _, name := range found {
				if strings.HasPrefix(name, "-") {
					continue
				}
				if positive != "" {
					return
				}
				positive = name
			}
			if positive != "" {
				for r := range k.parse(positive) {
					if !yield(r) {
						return
					}
				}
			}
			return
		}
		if len(found) == 0 && c.Modifier != nil {
			r := c
			r.Modifier = nil
			if sig := k.signature(k.print(r)).css; sig != "" {
				for replacement := range k.replacements(sig, r, dynamic) {
					replacement.Modifier = c.Modifier
					if !yield(replacement) {
						return
					}
				}
			}
		}
		if len(found) == 1 {
			for r := range k.parse(found[0]) {
				if !yield(r) {
					return
				}
			}
			return
		}
		if !dynamic || c.Value == nil {
			return
		}
		value := c.Value.Value
		spacing, hasSpacing := k.spacing(value)
		if k.options.Rem != nil && c.Kind == "functional" && c.Value.Kind == "arbitrary" && hasSpacing && quarterNumber(jsFloat(spacing)) && k.reasonableSpacing(spacing) {
			r := c
			r.Value = &CandidateValue{Kind: "named", Value: jsFloat(spacing)}
			if !yield(r) {
				return
			}
		}
		rootPrefix := ""
		if hasSpacing && spacing < 0 {
			rootPrefix = "-"
			spacing = -spacing
		}
		for _, negative := range []bool{false, true} {
			for _, root := range k.roots {
				if strings.HasPrefix(root, "-") != negative {
					continue
				}
				root = rootPrefix + root
				tries := []string{root + "-" + value}
				if c.Modifier != nil {
					tries = append(tries, root+"-"+value+"[object Object]")
				}
				if hasSpacing && quarterNumber(jsFloat(spacing)) && k.reasonableSpacing(spacing) {
					tries = append(tries, root+"-"+jsFloat(spacing))
					if c.Modifier != nil {
						tries = append(tries, root+"-"+jsFloat(spacing)+printModifier(c.Modifier))
					}
				}
				tries = append(tries, root+"-["+value+"]")
				if c.Modifier != nil {
					tries = append(tries, root+"-["+value+"]"+printModifier(c.Modifier))
				}
				for _, name := range tries {
					for r := range k.parse(name) {
						if !yield(r) {
							return
						}
					}
				}
			}
		}
	}
}

func (k *Canonicalizer) spacing(value string) (float64, bool) {
	base, unit, ok := dimension(constantFoldDeclaration(k.themeValue("--spacing"), k.options.Rem, true))
	if !ok || base == 0 {
		return 0, false
	}
	n, u, ok := dimension(constantFoldDeclaration(value, k.options.Rem, true))
	return n / base, ok && u == unit
}
func (k *Canonicalizer) reasonableSpacing(value float64) bool {
	n, unit, ok := dimension(constantFoldDeclaration(k.themeValue("--spacing"), k.options.Rem, true))
	return ok && (unit == "px" && value*n <= 1536 || unit == "rem" && value*n <= 96)
}

func (k *Canonicalizer) variablesPreserved(c, replacement Candidate) bool {
	if c.Value == nil || !strings.Contains(c.Value.Value, "var(--") {
		return true
	}
	css := k.signatureCompiler.CandidateCSS([]string{k.print(replacement)})[0]
	var valid func([]*valueNode) bool
	valid = func(nodes []*valueNode) bool {
		for _, n := range nodes {
			if n.kind == 'f' && n.value == "var" && len(n.nodes) > 0 {
				key := n.nodes[0].value
				if !strings.Contains(css, "var("+key+",") && !strings.Contains(css, "var("+key+")") || strings.Contains(css, key+":") {
					return false
				}
			}
			if !valid(n.nodes) {
				return false
			}
		}
		return true
	}
	return valid(parseValue(c.Value.Value))
}

var canonicalNumbers = regexp.MustCompile(`\d+/\d+|\d+\.?\d+`)

func canonicalBareValues(value string, seen map[string]bool) iter.Seq[CandidateValue] {
	return func(yield func(CandidateValue) bool) {
		if seen[value] {
			return
		}
		seen[value] = true
		if !yield(CandidateValue{Kind: "named", Value: value}) {
			return
		}
		if v, ok := strings.CutSuffix(value, "%"); ok && quarterNumber(v) {
			if !yield(CandidateValue{Kind: "named", Value: v}) {
				return
			}
		}
		if parts := strings.Split(value, "/"); len(parts) > 1 && positiveInteger(parts[0]) && positiveInteger(parts[1]) {
			if !yield(CandidateValue{Kind: "named", Value: parts[0], Fraction: parts[0] + "/" + parts[1]}) {
				return
			}
		}
		options := canonicalNumbers.FindAllString(value, -1)
		slices.SortStableFunc(options, func(a, b string) int { return len(a) - len(b) })
		for _, option := range options {
			for v := range canonicalBareValues(option, seen) {
				if !yield(v) {
					return
				}
			}
		}
	}
}

func (k *Canonicalizer) optimizeExpression(c Candidate) Candidate {
	if c.Kind != "functional" || c.Value == nil || c.Value.Kind != "arbitrary" {
		return c
	}
	nodes := parseValue(c.Value.Value)
	if len(nodes) == 1 && nodes[0].kind == 'f' && nodes[0].value == "calc" {
		if constantFoldNodes(nodes, false, nil, false) {
			r := copyCandidate(c)
			r.Value.Value = valueCSS(nodes)
			if k.same(c, r) {
				c = r
			}
		}
	}
	if strings.HasPrefix(c.Root, "-") {
		if len(nodes) == 1 && nodes[0].kind == 'f' && nodes[0].value == "var" {
			return c
		}
		expression := parseValue("calc(" + c.Value.Value + " * -1)")
		if constantFoldNodes(expression, false, nil, false) {
			r := copyCandidate(c)
			r.Root = r.Root[1:]
			r.Value.Value = valueCSS(expression)
			if k.same(c, r) {
				c = r
				nodes = expression
			}
		}
	}
	if len(nodes) == 1 && binaryCalculation(nodes[0]) && nodes[0].value == "calc" {
		a := nodes[0].nodes
		if a[1].kind == 's' && a[1].value == " " && a[2].value == "*" && a[3].kind == 's' && a[3].value == " " {
			var arg *valueNode
			if a[4].kind == 'w' && a[4].value == "-1" {
				arg = a[0]
			} else if a[0].kind == 'w' && a[0].value == "-1" {
				arg = a[4]
			}
			if arg != nil {
				r := copyCandidate(c)
				r.Root = "-" + r.Root
				r.Value.Value = valueCSS([]*valueNode{arg})
				if k.same(c, r) {
					c = r
				}
			}
		}
	}
	return c
}

func (k *Canonicalizer) spacingFunction(value string) string {
	spacing := "--spacing"
	if p := k.compiler.config.Prefix; p != "" {
		spacing = "--" + p + "-spacing"
	}
	var walk func([]*valueNode)
	walk = func(nodes []*valueNode) {
		for _, n := range nodes {
			if n.kind == 'f' && n.value == "calc" && len(n.nodes) == 5 && n.nodes[2].kind == 'w' && n.nodes[2].value == "*" {
				a := n.nodes[0]
				if a.kind == 'f' && a.value == "var" && len(a.nodes) == 1 && a.nodes[0].kind == 'w' && a.nodes[0].value == spacing {
					n.value = "--spacing"
					n.nodes = []*valueNode{n.nodes[4]}
				}
			}
			walk(n.nodes)
		}
	}
	nodes := parseValue(value)
	walk(nodes)
	return valueCSS(nodes)
}
