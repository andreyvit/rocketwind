package rocketwind

import (
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
)

// ValueType disambiguates a setting before its value is available. CSS types use
// Tailwind's arbitrary-value type names. TypeClasses denotes an inline template.
type ValueType string

const (
	TypeAny         ValueType = "any"
	TypeColor       ValueType = "color"
	TypeLength      ValueType = "length"
	TypeNumber      ValueType = "number"
	TypeInteger     ValueType = "integer"
	TypePercentage  ValueType = "percentage"
	TypeAngle       ValueType = "angle"
	TypeImage       ValueType = "image"
	TypeURL         ValueType = "url"
	TypeFamilyName  ValueType = "family-name"
	TypeGenericName ValueType = "generic-name"
	TypePosition    ValueType = "position"
	TypeSize        ValueType = "bg-size"
	TypeRatio       ValueType = "ratio"
	TypeVector      ValueType = "vector"
	TypeShadow      ValueType = "shadow"
	TypeClasses     ValueType = "classes"
)

// Parameter is an opaque client expression and its declared type. Rocketwind
// never interprets the expression or assumes a settings namespace.
type Parameter struct {
	Expression string
	Type       ValueType
}

// ValueMode controls how a parameter becomes a CSS value.
type ValueMode uint8

const (
	// ValueModeAuto tries the utility's named values first, then literal CSS.
	// Inside explicit brackets, such as p-[{gap}], it uses literal CSS.
	ValueModeAuto ValueMode = iota
	// ValueModeToken requires a named value, including Tailwind's numeric scales.
	// The parameter must fill a whole utility value, modifier, or variant value.
	ValueModeToken
	// ValueModeLiteral uses CSS directly, preserving spaces and underscores.
	ValueModeLiteral
)

// Value supplies CSS or another prepared class list. An empty CSS string with
// no Classes omits the containing utility. Resolve gives an inline class list
// its own settings scope; nil inherits the caller's resolver. Mode applies to CSS.
type Value struct {
	CSS     string
	Mode    ValueMode
	Classes *Template
	Resolve ResolveFunc
}

type ResolveFunc func(Parameter) (Value, error)

type TemplateOptions struct {
	TypeOf func(expression string) (ValueType, error)
	// DollarReferences accepts $name and prefix-$name as shorthand for {name}.
	DollarReferences bool
}

// Template is an immutable class list. Prepare once, then resolve it for each
// account or settings change. Concurrent rendering is safe when resolvers are.
type Template struct {
	compiler   *Compiler
	parameters []Parameter
	entries    []templateEntry
	marker     string
}

type templateEntry struct {
	candidates []Candidate
	reference  int
	variants   []*CandidateVariant
	important  bool
	raw        string
}

// Rule generates declarations for a named selector, including nested variants.
// Templates retain source order so later declarations can override earlier ones.
type Rule struct {
	Selector string
	Template *Template
	Resolve  ResolveFunc
}

// Prepare parses classes and looks up parameter types without resolving values.
// Parameters use {expression}; whole-class parameters must have TypeClasses.
// Literal braces can be escaped with a backslash.
func (c *Compiler) Prepare(classes string, options TemplateOptions) (*Template, error) {
	t := &Template{compiler: c, marker: "RWparam"}
	for strings.Contains(classes, t.marker) {
		t.marker += "X"
	}
	indices := map[string]int{}
	parameter := func(expression string) (int, error) {
		if expression == "" {
			return 0, fmt.Errorf("empty expression in class template")
		}
		if index, ok := indices[expression]; ok {
			return index, nil
		}
		if options.TypeOf == nil {
			return 0, fmt.Errorf("parameter {%s} needs a type lookup", expression)
		}
		kind, err := options.TypeOf(expression)
		if err != nil {
			return 0, fmt.Errorf("parameter {%s}: %w", expression, err)
		}
		if kind == "" {
			return 0, fmt.Errorf("missing type for parameter {%s}", expression)
		}
		index := len(t.parameters)
		indices[expression] = index
		t.parameters = append(t.parameters, Parameter{expression, kind})
		return index, nil
	}
	for raw := range classWords(classes) {
		word := raw
		// Accept the legacy spelling with ! before the variants too.
		leadingImportant := strings.HasPrefix(word, "!")
		if leadingImportant {
			word = word[1:]
		}
		parts := slices.Collect(Segments(word, ':'))
		base := parts[len(parts)-1]
		baseStart := len(word) - len(base)
		important := leadingImportant || strings.HasSuffix(base, "!") || strings.HasPrefix(base, "!")
		base = strings.TrimSuffix(strings.TrimPrefix(base, "!"), "!")
		word = word[:baseStart] + base
		if options.DollarReferences {
			word = dollarParameters(word)
			parts = slices.Collect(Segments(word, ':'))
			base = parts[len(parts)-1]
			baseStart = len(word) - len(base)
		}
		entry := templateEntry{reference: -1, raw: raw}
		if len(base) > 0 && base[0] == '{' {
			end, err := expressionEnd(base, 0)
			if err != nil {
				return nil, err
			}
			if end == len(base)-1 {
				index, err := parameter(base[1:end])
				if err != nil {
					return nil, err
				}
				if t.parameters[index].Type != TypeClasses {
					return nil, fmt.Errorf("parameter {%s} must be a class list", t.parameters[index].Expression)
				}
				entry.reference, entry.important = index, important
				word = word[:baseStart] + "flex"
			}
		}
		var rewritten strings.Builder
		start, bracketDepth := 0, 0
		for i := 0; i < len(word); i++ {
			switch word[i] {
			case '\\':
				i++
			case '[':
				bracketDepth++
			case ']':
				bracketDepth--
			case '{':
				end, err := expressionEnd(word, i)
				if err != nil {
					return nil, err
				}
				index, err := parameter(word[i+1 : end])
				if err != nil {
					return nil, err
				}
				p := t.parameters[index]
				if p.Type == TypeClasses {
					return nil, fmt.Errorf("parameter {%s} must be a CSS value here", p.Expression)
				}
				rewritten.WriteString(word[start:i])
				if bracketDepth == 0 {
					rewritten.WriteByte('[')
				}
				typedValue := bracketDepth == 0 && i > 0 && word[i-1] == '-' ||
					bracketDepth == 1 && i > 1 && word[i-2:i] == "-[" && end+1 < len(word) && word[end+1] == ']'
				if i >= baseStart && typedValue && p.Type != TypeAny {
					rewritten.WriteString(string(p.Type))
					rewritten.WriteByte(':')
				}
				rewritten.WriteString(t.marker)
				rewritten.WriteString(strconv.Itoa(index))
				if bracketDepth == 0 {
					rewritten.WriteByte('Z')
					rewritten.WriteByte(']')
				} else {
					rewritten.WriteByte('L')
				}
				start, i = end+1, end
			}
		}
		if start > 0 {
			rewritten.WriteString(word[start:])
			word = rewritten.String()
		}
		if important {
			if c.config.Compatibility == Tailwind3 {
				parts := slices.Collect(Segments(word, ':'))
				parts[len(parts)-1] = "!" + parts[len(parts)-1]
				word = strings.Join(parts, ":")
			} else {
				word += "!"
			}
		}
		for candidate := range c.Parse(word) {
			candidate.Raw = raw
			entry.candidates = append(entry.candidates, candidate)
		}
		if len(entry.candidates) == 0 {
			return nil, fmt.Errorf("unknown utility %q", raw)
		}
		if entry.reference >= 0 {
			entry.variants = entry.candidates[0].Variants
			entry.candidates = nil
		}
		t.entries = append(t.entries, entry)
	}
	return t, nil
}

// Parameters returns the distinct parameters in first-use order.
func (t *Template) Parameters() []Parameter { return slices.Clone(t.parameters) }

// UtilityRoots returns parsed utility roots after alias expansion, in source
// order. A class can have several interpretations. Arbitrary properties have
// an empty root; inline class references are resolved later and are omitted.
// This lets clients attach compatibility semantics without parsing class names.
func (t *Template) UtilityRoots() []string {
	var roots []string
	for _, entry := range t.entries {
		for _, candidate := range entry.candidates {
			roots = append(roots, candidate.Root)
		}
	}
	return roots
}

// CSS generates one named rule. Use Compiler.Render to share theme declarations
// and CSS property registrations across several rules.
func (t *Template) CSS(selector string, resolve ResolveFunc) (string, error) {
	return t.compiler.Render([]Rule{{selector, t, resolve}})
}

// Nodes resolves a template to an independent tree of declarations and nested
// variants, before selector flattening. This is useful when integrating with an
// existing CSS writer. Theme declarations, keyframes and property registrations
// are not hoisted; use CSS or Compiler.Render for a complete stylesheet.
func (t *Template) Nodes(resolve ResolveFunc) ([]*Node, error) {
	b := buildContext{compiler: t.compiler, used: map[string]bool{}, inApply: true, standalone: true}
	nodes, err := b.renderTemplate(t, resolve, nil)
	if err != nil {
		return nil, err
	}
	if prefix := t.compiler.config.InternalVariablePrefix; prefix != "" {
		return namespaceInternalVariables(nodes, prefix), nil
	}
	return cloneNodes(nodes, "", false), nil
}

// Render generates only the supplied named rules and their CSS dependencies.
// It does not emit the compiler's input stylesheet or accumulate prior calls.
func (c *Compiler) Render(rules []Rule) (string, error) {
	b := buildContext{compiler: c, used: map[string]bool{}, inApply: true, standalone: true}
	var nodes []*Node
	for _, rule := range rules {
		if strings.TrimSpace(rule.Selector) == "" {
			return "", fmt.Errorf("named rule needs a selector")
		}
		if rule.Template == nil || rule.Template.compiler != c {
			return "", fmt.Errorf("named rule %q needs a template from this compiler", rule.Selector)
		}
		content, err := b.renderTemplate(rule.Template, rule.Resolve, nil)
		if err != nil {
			return "", fmt.Errorf("rule %s: %w", rule.Selector, err)
		}
		if len(content) > 0 {
			nodes = append(nodes, &Node{Kind: "rule", Name: rule.Selector, Nodes: content})
		}
	}
	css := b.render(nodes)
	return css, b.err
}

type templateResolution struct {
	template *Template
	resolve  ResolveFunc
	values   []Value
	resolved []bool
}

func (r *templateResolution) get(index int) (Value, error) {
	if r.resolved[index] {
		return r.values[index], nil
	}
	p := r.template.parameters[index]
	if r.resolve == nil {
		return Value{}, fmt.Errorf("parameter {%s} needs a resolver", p.Expression)
	}
	value, err := r.resolve(p)
	if err != nil {
		return Value{}, fmt.Errorf("parameter {%s}: %w", p.Expression, err)
	}
	if value.Mode > ValueModeLiteral {
		return Value{}, fmt.Errorf("parameter {%s} has invalid value mode %d", p.Expression, value.Mode)
	}
	if value.CSS != "" || value.Classes != nil {
		if p.Type == TypeClasses {
			if value.Classes == nil {
				return Value{}, fmt.Errorf("parameter {%s} needs a class list", p.Expression)
			}
		} else if value.Classes != nil {
			return Value{}, fmt.Errorf("parameter {%s} needs a CSS value", p.Expression)
		}
	}
	r.values[index], r.resolved[index] = value, true
	return value, nil
}

func (r *templateResolution) text(input string) (value string, mode ValueMode, omit bool, err error) {
	marker := r.template.marker
	index := strings.Index(input, marker)
	if index < 0 {
		return input, ValueModeLiteral, false, nil
	}
	var out strings.Builder
	for index >= 0 {
		end := index + len(marker)
		for end < len(input) && digit(input[end]) {
			end++
		}
		// Only Prepare inserts markers; neither source text nor resolved values
		// can introduce one into this scan.
		parameter, _ := strconv.Atoi(input[index+len(marker) : end])
		resolved, err := r.get(parameter)
		if err != nil || resolved.CSS == "" {
			return "", ValueModeLiteral, true, err
		}
		if index == 0 && end+1 == len(input) && out.Len() == 0 {
			mode := resolved.Mode
			if mode == ValueModeAuto && input[end] == 'L' {
				mode = ValueModeLiteral
			}
			return resolved.CSS, mode, false, nil
		}
		if resolved.Mode == ValueModeToken {
			return "", ValueModeLiteral, false, fmt.Errorf("named token for {%s} must fill the whole value", r.template.parameters[parameter].Expression)
		}
		out.WriteString(input[:index])
		out.WriteString(resolved.CSS)
		input = input[end+1:]
		index = strings.Index(input, marker)
	}
	out.WriteString(input)
	return out.String(), ValueModeLiteral, false, nil
}

func (r *templateResolution) candidateValue(input *CandidateValue) (*CandidateValue, bool, error) {
	if input == nil {
		return nil, false, nil
	}
	value, mode, omit, err := r.text(input.Value)
	if err != nil || omit {
		return nil, omit, err
	}
	if value == input.Value && mode == ValueModeLiteral {
		return input, false, nil
	}
	result := *input
	result.Value = value
	if mode == ValueModeToken {
		result.Kind, result.Fraction = "named", ""
	} else if mode == ValueModeAuto {
		result.Kind, result.Fraction = "auto", ""
	}
	return &result, false, nil
}

func (r *templateResolution) modifier(input *CandidateModifier) (*CandidateModifier, bool, error) {
	if input == nil {
		return nil, false, nil
	}
	value, mode, omit, err := r.text(input.Value)
	if err != nil || omit {
		return nil, omit, err
	}
	if value == input.Value && mode == ValueModeLiteral {
		return input, false, nil
	}
	result := *input
	result.Value = value
	if mode == ValueModeToken {
		result.Kind = "named"
	} else if mode == ValueModeAuto {
		result.Kind = "auto"
	}
	return &result, false, nil
}

func (r *templateResolution) variant(input *CandidateVariant) (*CandidateVariant, bool, error) {
	if input == nil {
		return nil, false, nil
	}
	result := *input
	var omit bool
	var err error
	result.Value, omit, err = r.candidateValue(input.Value)
	if omit || err != nil {
		return nil, omit, err
	}
	result.Modifier, omit, err = r.modifier(input.Modifier)
	if omit || err != nil {
		return nil, omit, err
	}
	result.Selector, _, omit, err = r.text(input.Selector)
	if omit || err != nil {
		return nil, omit, err
	}
	result.Variant, omit, err = r.variant(input.Variant)
	return &result, omit, err
}

func (b *buildContext) renderTemplate(t *Template, resolve ResolveFunc, stack []*Template) ([]*Node, error) {
	if t.compiler != b.compiler {
		return nil, fmt.Errorf("inline class list belongs to a different compiler")
	}
	if slices.Contains(stack, t) {
		return nil, fmt.Errorf("circular class list reference")
	}
	if len(stack) >= 100 {
		return nil, fmt.Errorf("class list nesting exceeds 100 levels")
	}
	stack = append(stack, t)
	r := templateResolution{t, resolve, make([]Value, len(t.parameters)), make([]bool, len(t.parameters))}
	var result []*Node
	for _, entry := range t.entries {
		if entry.reference >= 0 {
			value, err := r.get(entry.reference)
			if err != nil {
				return nil, err
			}
			if value.Classes == nil {
				continue
			}
			childResolver := value.Resolve
			if childResolver == nil {
				childResolver = resolve
			}
			nodes, err := b.renderTemplate(value.Classes, childResolver, stack)
			if err != nil {
				return nil, err
			}
			if entry.important {
				nodes = cloneNodes(nodes, "", true)
			}
			for _, variant := range entry.variants {
				resolved, omit, err := r.variant(variant)
				if err != nil {
					return nil, err
				}
				if omit {
					nodes = nil
					break
				}
				var valid bool
				nodes, valid = b.applyParsedVariant(nodes, b.compiledVariant(resolved), 0)
				if b.err != nil {
					return nil, b.err
				}
				if !valid {
					return nil, fmt.Errorf("invalid variant in %q", entry.raw)
				}
			}
			result = append(result, nodes...)
			continue
		}
		accepted := false
		for _, input := range entry.candidates {
			candidate := input
			var omit bool
			var err error
			candidate.Value, omit, err = r.candidateValue(input.Value)
			if err != nil {
				return nil, err
			}
			if omit {
				accepted = true
				continue
			}
			candidate.Modifier, omit, err = r.modifier(input.Modifier)
			if err != nil {
				return nil, err
			}
			if omit {
				accepted = true
				continue
			}
			candidate.Variants = make([]*CandidateVariant, 0, len(input.Variants))
			for _, variant := range input.Variants {
				var value *CandidateVariant
				value, omit, err = r.variant(variant)
				if omit || err != nil {
					break
				}
				candidate.Variants = append(candidate.Variants, value)
			}
			if err != nil {
				return nil, err
			}
			if omit {
				accepted = true
				continue
			}
			groups := b.templateCandidateGroups(candidate)
			if b.err != nil {
				return nil, b.err
			}
			accepted = accepted || len(groups) > 0
			for _, nodes := range groups {
				nodes = b.evaluateNodes(nodes)
				if candidate.Important || b.compiler.config.Important {
					nodes = cloneNodes(nodes, "", true)
				}
				for _, variant := range candidate.Variants {
					var valid bool
					nodes, valid = b.applyParsedVariant(nodes, b.compiledVariant(variant), 0)
					if b.err != nil {
						return nil, b.err
					}
					if !valid {
						return nil, fmt.Errorf("invalid variant in %q", entry.raw)
					}
				}
				result = append(result, nodes...)
			}
		}
		if !accepted {
			return nil, fmt.Errorf("invalid value for utility %q", entry.raw)
		}
	}
	return result, b.err
}

// Try named values before literals, independently for the value and modifier.
// Keep these interpretations local; prepared candidates are shared by renders.
func (b *buildContext) templateCandidateGroups(candidate Candidate) [][]*Node {
	value, modifier := candidate.Value, candidate.Modifier
	if candidate.Kind == "arbitrary" && value.Kind == "named" {
		return nil // Arbitrary CSS properties have no token namespace.
	}
	valueAuto := value != nil && value.Kind == "auto"
	modifierAuto := modifier != nil && modifier.Kind == "auto"
	// A resolved fraction uses the same named-value path as w-1/2. Retain
	// the original string in case Auto needs the literal interpretation.
	fraction := ""
	if value != nil && value.Kind != "arbitrary" && modifier == nil {
		if numerator, denominator, ok := strings.Cut(value.Value, "/"); ok && numerator != "" && denominator != "" && validNamed(numerator) && validNamed(denominator) {
			fraction = value.Value
			copy := *value
			copy.Value = numerator
			value, candidate.Value = &copy, &copy
			modifier = &CandidateModifier{Kind: "named", Value: denominator}
			candidate.Modifier = modifier
		}
	}
	if value != nil && value.Kind == "named" && modifier != nil {
		copy := *value
		value, candidate.Value = &copy, &copy
	}
	// Auto values were allocated by resolution and belong to this render.
	if valueAuto {
		value.Kind = "named"
	}
	for {
		if modifierAuto {
			modifier.Kind = "named"
		}
		for {
			if value != nil && value.Kind == "named" && modifier != nil {
				value.Fraction = ""
				if modifier.Kind == "named" {
					value.Fraction = value.Value + "/" + modifier.Value
				}
			}
			groups := b.candidateGroups(candidate)
			if len(groups) > 0 || b.err != nil {
				return groups
			}
			if !modifierAuto || modifier.Kind == "arbitrary" {
				break
			}
			modifier.Kind = "arbitrary"
		}
		if !valueAuto || value.Kind == "arbitrary" {
			return nil
		}
		value.Kind = "arbitrary"
		value.Fraction = ""
		if fraction != "" {
			value.Value = fraction
			modifier, candidate.Modifier = nil, nil
		}
	}
}

func expressionEnd(input string, start int) (int, error) {
	depth := 1
	for i := start + 1; i < len(input); i++ {
		switch input[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unclosed expression in %q", input)
}

func dollarParameters(input string) string {
	parts := slices.Collect(Segments(input, ':'))
	base := parts[len(parts)-1]
	start := strings.IndexByte(base, '$')
	if start < 0 || start > 0 && base[start-1] != '-' {
		return input
	}
	parts[len(parts)-1] = base[:start] + "{" + base[start+1:] + "}"
	return strings.Join(parts, ":")
}

func classWords(input string) iter.Seq[string] {
	return func(yield func(string) bool) {
		var local [32]byte
		stack := local[:0]
		start := 0
		for i := 0; i < len(input); i++ {
			c := input[i]
			if len(stack) == 0 && cssSpace(c) {
				if start < i && !yield(input[start:i]) {
					return
				}
				start = i + 1
				continue
			}
			switch c {
			case '\\':
				i++
			case '\'', '"':
				for i++; i < len(input); i++ {
					if input[i] == '\\' {
						i++
					} else if input[i] == c {
						break
					}
				}
			case '[':
				stack = append(stack, ']')
			case '(':
				stack = append(stack, ')')
			case '{':
				stack = append(stack, '}')
			case ']', ')', '}':
				if len(stack) > 0 && stack[len(stack)-1] == c {
					stack = stack[:len(stack)-1]
				}
			}
		}
		if start < len(input) {
			yield(input[start:])
		}
	}
}
