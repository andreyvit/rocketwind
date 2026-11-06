package rocketwind

import "strings"

func validUtilityName(s string) bool {
	functional := strings.HasSuffix(s, "-*")
	if functional {
		s = s[:len(s)-2]
	}
	if strings.HasPrefix(s, "-") {
		s = s[1:]
	}
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	i := 1
	for i < len(s) && (asciiLetter(s[i]) || digit(s[i]) || s[i] == '_' || s[i] == '-') {
		i++
	}
	if functional {
		return i == len(s)
	}
	if i == len(s) {
		return s[len(s)-1] != '-'
	}
	slash := false
	for ; i < len(s); i++ {
		c := s[i]
		switch c {
		case '%':
			if i != len(s)-1 || i == 0 || !digit(s[i-1]) {
				return false
			}
		case '/':
			if slash || i == len(s)-1 {
				return false
			}
			slash = true
		case '.':
			if i == 0 || !digit(s[i-1]) || i == len(s)-1 || !digit(s[i+1]) {
				return false
			}
		default:
			if !asciiLetter(c) && !digit(c) && c != '_' && c != '-' {
				return false
			}
		}
	}
	return true
}

type customValue struct {
	value, dataType, fraction string
	arbitrary                 bool
}
type customResolution struct {
	value     string
	ratio, ok bool
}

func (b *buildContext) customUtility(definition []*Node, value, mod, dataType string, arbitrary, modArbitrary bool) []*Node {
	val := customValue{value: value, dataType: dataType, arbitrary: arbitrary}
	modifier := customValue{value: mod, arbitrary: modArbitrary}
	if mod != "" && !modArbitrary {
		val.fraction = value + "/" + mod
	}
	usedValue, resolvedValue, usedModifier, resolvedModifier, resolvedRatio := false, false, false, false, false
	nonRatio := map[*Node]bool{}
	var transform func([]*Node) []*Node
	transform = func(nodes []*Node) []*Node {
		var result []*Node
		for _, n := range nodes {
			r := *n
			if n.Kind == "declaration" && (strings.Contains(n.Value, "--value(") || strings.Contains(n.Value, "--modifier(")) {
				ast := parseValue(n.Value)
				var resolve func([]*valueNode) bool
				resolve = func(nodes []*valueNode) bool {
					for _, fn := range nodes {
						if fn.kind != 'f' {
							continue
						}
						switch fn.value {
						case "--value", "--modifier":
							isValue := fn.value == "--value"
							target := modifier
							if isValue {
								usedValue = true
								target = val
							} else {
								usedModifier = true
							}
							resolved := b.resolveCustomValue(target, fn.nodes)
							if !resolved.ok {
								return false
							}
							if isValue {
								resolvedValue = true
								if resolved.ratio {
									resolvedRatio = true
								} else {
									nonRatio[&r] = true
								}
							} else {
								resolvedModifier = true
							}
							fn.kind, fn.value, fn.nodes = 'w', resolved.value, nil
						default:
							if !resolve(fn.nodes) {
								return false
							}
						}
					}
					return true
				}
				if !resolve(ast) {
					continue
				}
				r.Value = valueCSS(ast)
			}
			if n.Nodes != nil {
				r.Nodes = transform(n.Nodes)
				if len(r.Nodes) == 0 {
					continue
				}
			}
			result = append(result, &r)
		}
		return result
	}
	result := transform(definition)
	if !usedValue || !resolvedValue || usedModifier && !resolvedModifier && mod != "" || resolvedRatio && resolvedModifier || mod != "" && !resolvedRatio && !resolvedModifier {
		return nil
	}
	if resolvedRatio {
		var remove func([]*Node) []*Node
		remove = func(nodes []*Node) []*Node {
			var out []*Node
			for _, n := range nodes {
				if nonRatio[n] {
					continue
				}
				if n.Nodes != nil {
					n.Nodes = remove(n.Nodes)
				}
				out = append(out, n)
			}
			return out
		}
		result = remove(result)
	}
	return result
}

func normalizeCustomArg(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "--default(") {
		return s
	}
	s = strings.ReplaceAll(s, `\*`, "*")
	if strings.HasPrefix(s, "--") {
		// Normalize formatter-friendly namespace and sub-namespace spellings.
		parts := strings.Fields(s)
		for i := 1; i < len(parts); i++ {
			if strings.HasPrefix(parts[i], "--") && !strings.Contains(strings.Join(parts[:i], ""), "-*") {
				parts[i] = "-*" + parts[i]
			}
		}
		s = strings.Join(parts, "")
		for strings.Contains(s, "-*-*") {
			s = strings.ReplaceAll(s, "-*-*", "-*")
		}
		if !strings.Contains(s, "(") && !strings.Contains(s, "-*") {
			s += "-*"
		}
	} else {
		s = strings.Join(strings.Fields(s), "")
	}
	return s
}

func (b *buildContext) resolveCustomValue(value customValue, args []*valueNode) customResolution {
	for arg := range Segments(valueCSS(args), ',') {
		arg = normalizeCustomArg(arg)
		if value.value == "" {
			if strings.HasPrefix(arg, "--default(") && strings.HasSuffix(arg, ")") {
				return customResolution{value: arg[10 : len(arg)-1], ok: true}
			}
			continue
		}
		if !value.arbitrary {
			if len(arg) > 1 && (arg[0] == '\'' || arg[0] == '"') && arg[len(arg)-1] == arg[0] && arg[1:len(arg)-1] == value.value {
				return customResolution{value: value.value, ok: true}
			}
			if strings.HasPrefix(arg, "--") {
				namespace, sub, found := strings.Cut(arg, "-*")
				if !found {
					continue
				}
				resolved := b.resolve(value.value, []string{namespace})
				if resolved == "" {
					continue
				}
				if sub != "" {
					resolved = b.theme(namespace + "-" + value.value + sub)
				}
				if resolved != "" {
					return customResolution{value: resolved, ok: true}
				}
				continue
			}
			if !containsWord("number integer ratio percentage", arg) {
				continue
			}
			v := value.value
			if arg == "ratio" {
				v = value.fraction
			}
			if inferType(v, arg) == "" {
				continue
			}
			if arg == "number" && !quarterNumber(v) || arg == "percentage" && !positiveInteger(strings.TrimSuffix(v, "%")) {
				continue
			}
			if arg == "ratio" {
				a, z, found := strings.Cut(v, "/")
				if !found || !positiveInteger(a) || !positiveInteger(z) {
					continue
				}
				return customResolution{value: a + " / " + z, ok: true, ratio: true}
			}
			return customResolution{value: v, ok: true}
		}
		if len(arg) > 2 && arg[0] == '[' && arg[len(arg)-1] == ']' {
			kind := arg[1 : len(arg)-1]
			if kind == "*" || value.dataType != "" && value.dataType == kind || value.dataType == "" && inferType(value.value, kind) != "" {
				return customResolution{value: value.value, ok: true}
			}
		}
	}
	return customResolution{}
}

// Resolve each definition once. The combined view is used for parsing and
// @apply dependency discovery; groups preserve separate plugin registrations.
type staticUtilityGroup struct {
	nodes    []*Node
	deferred bool // addUtilities validates only classes that are used.
}

func (c *Compiler) transformStaticUtility(name string, transform func([]*Node) ([]*Node, error), applies bool) error {
	groups := c.customGroups[name]
	if len(groups) == 0 {
		nodes, err := transform(c.custom[name])
		if err != nil {
			return err
		}
		c.custom[name] = nodes
		return nil
	}
	var combined []*Node
	for i, group := range groups {
		nodes, err := transform(group.nodes)
		if err != nil {
			if !group.deferred {
				return err
			}
			// Invalid plugin variants reject this definition. An invalid
			// @apply becomes an error only when the utility is requested.
			if applies {
				if c.customErrors == nil {
					c.customErrors = map[string]error{}
				}
				c.customErrors[name] = err
			}
			nodes = nil
		}
		groups[i].nodes = nodes
		combined = append(combined, nodes...)
	}
	c.custom[name] = combined
	return nil
}
