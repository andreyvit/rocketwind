package rocketwind

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"
)

type utilityDefinition struct {
	Color, Negative, Fractions, HasDefault bool
	Keys                                   []string
	Bare, Default                          string
	Template                               []*Node
	ZeroTemplate                           []*Node
	Static                                 map[string][]*Node
}

type Config struct {
	DisablePolyfills Polyfills
	Base             string
	LoadStylesheet   StylesheetLoader
	LoadModule       ModuleLoader
	Important        bool
	// Theme contains CSS theme variables, for example --color-brand: #663399.
	Theme map[string]string
	// NoDefaultTheme starts with an empty theme, like @tailwind utilities alone.
	NoDefaultTheme bool
	// InlineTheme emits resolved values rather than theme variable references.
	InlineTheme bool
	Prefix      string
	Variants    []Variant
	Plugins     map[string]Plugin
}

type Compiler struct {
	hasUtilities          bool
	config                Config
	input                 []*Node
	hasInput, themePlaced bool
	themeFlags            map[string]string
	themeOrder            []string
	keyframes             []*Node
	custom                map[string][]*Node
	customFunctional      map[string][][]*Node
	customOrder           []string
	customVariants        map[string]variantDefinition
	variantOrder          []string
	pluginBase            []*Node
	sources               []Source
	inlineCandidates      []string
	excludedCandidates    map[string]bool
	matchingVariants      map[string]*Variant
}

func New(config Config) *Compiler {
	config.Theme = maps.Clone(config.Theme)
	c := &Compiler{config: config, customVariants: map[string]variantDefinition{}, matchingVariants: map[string]*Variant{}}
	for _, variant := range config.Variants {
		c.addVariant(variant)
	}
	return c
}

func (c *Compiler) Build(candidates []string) (string, error) {
	b := buildContext{compiler: c, used: make(map[string]bool)}
	if len(c.inlineCandidates) > 0 {
		candidates = append(slices.Clone(candidates), c.inlineCandidates...)
	}
	rules := b.compileRules(candidates)
	var nodes []*Node
	for _, r := range rules {
		nodes = append(nodes, r.node)
	}
	if c.hasInput {
		var expand func([]*Node) []*Node
		expand = func(input []*Node) []*Node {
			var result []*Node
			for _, n := range input {
				if n.Kind == "utilities" {
					result = append(result, nodes...)
				} else {
					copy := *n
					if n.Nodes != nil {
						copy.Nodes = expand(n.Nodes)
					}
					result = append(result, &copy)
				}
			}
			return result
		}
		nodes = expand(c.input)
	}
	return b.render(nodes), nil
}

func (b *buildContext) compileRules(candidates []string) []compiledRule {
	seen := make(map[string]bool, len(candidates))
	var rules []compiledRule
	for _, raw := range candidates {
		if b.compiler.excludedCandidates[raw] {
			continue
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		_, variants, _, _ := parseClass(raw, b.compiler.config.Prefix)
		for candidate := range b.compiler.Parse(raw) {
			for _, nodes := range b.candidateGroups(candidate) {
				nodes = b.evaluateNodes(nodes)
				if len(nodes) == 0 {
					continue
				}
				order, count := sortProperties(nodes)
				n := &Node{Kind: "rule", Name: "." + Escape(raw), Nodes: nodes}
				if candidate.Important || b.compiler.config.Important && !b.inApply {
					n = cloneNode(n, "", true)
				}
				valid := true
				for _, variant := range candidate.Variants {
					n.Nodes, valid = b.applyParsedVariant(n.Nodes, b.compiledVariant(variant), 0)
					if !valid {
						break
					}
				}
				if !valid {
					continue
				}
				rules = append(rules, compiledRule{raw: raw, node: n, order: order, count: count, variants: variants})
			}
		}
	}
	slices.SortStableFunc(rules, func(a, z compiledRule) int {
		if v := b.compareVariantSets(a.variants, z.variants); v != 0 {
			return v
		}
		for i := 0; i < max(len(a.order), len(z.order)); i++ {
			if i == len(a.order) {
				return 1
			}
			if i == len(z.order) {
				return -1
			}
			if v := cmp.Compare(a.order[i], z.order[i]); v != 0 {
				return v
			}
		}
		if v := cmp.Compare(z.count, a.count); v != 0 {
			return v
		}
		return naturalCompare(a.raw, z.raw)
	})
	return rules
}

type buildContext struct {
	err      error
	inApply  bool
	compiler *Compiler
	used     map[string]bool
}

type compiledRule struct {
	variants []string
	raw      string
	node     *Node
	order    []int
	count    int
}

func parseClass(raw, prefix string) (base string, variants []string, important, ok bool) {
	parts := slices.Collect(Segments(raw, ':'))
	if prefix != "" {
		if len(parts) < 2 || parts[0] != prefix {
			return
		}
		parts = parts[1:]
	}
	base = parts[len(parts)-1]
	variants = parts[:len(parts)-1]
	if strings.HasSuffix(base, "!") {
		important = true
		base = base[:len(base)-1]
	} else if strings.HasPrefix(base, "!") {
		important = true
		base = base[1:]
	}
	ok = base != ""
	return
}

func (b *buildContext) themeValue(key string) string {
	if value, ok := b.compiler.config.Theme[key]; ok {
		return value
	}
	if b.compiler.config.NoDefaultTheme {
		return ""
	}
	return builtinTheme[key]
}

func (b *buildContext) theme(key string) string {
	value := b.themeValue(key)
	if value == "" {
		return ""
	}
	flags := b.compiler.themeFlags[key]
	if b.compiler.config.InlineTheme || strings.Contains(flags, "inline") {
		return value
	}
	b.used[key] = true
	if p := b.compiler.config.Prefix; p != "" {
		key = "--" + p + "-" + key[2:]
	}
	if strings.Contains(flags, "reference") {
		return "var(" + Escape(key) + ", " + value + ")"
	}
	return "var(" + Escape(key) + ")"
}

func (b *buildContext) resolve(value string, keys []string) string {
	for _, key := range keys {
		namespace := key
		if value != "" {
			key += "-" + value
		}
		if ignoredThemeKey(key, namespace) {
			continue
		}
		if v := b.theme(key); v != "" {
			return v
		}
		if strings.Contains(value, ".") {
			if v := b.theme(strings.ReplaceAll(key, ".", "_")); v != "" {
				return v
			}
		}
	}
	return ""
}

func (b *buildContext) candidateGroups(candidate Candidate) [][]*Node {
	modifier := ""
	modifierArbitrary := false
	if candidate.Modifier != nil {
		modifier, modifierArbitrary = candidate.Modifier.Value, candidate.Modifier.Kind == "arbitrary"
	}
	switch candidate.Kind {
	case "static":
		var result [][]*Node
		if nodes := b.compiler.custom[candidate.Root]; len(nodes) > 0 {
			result = append(result, nodes)
		}
		if candidate.Root == "container" {
			result = append(result, b.container())
		} else if nodes := builtinStatic[candidate.Root]; len(nodes) > 0 {
			result = append(result, nodes)
		}
		return result
	case "arbitrary":
		value := candidate.Value.Value
		if modifier != "" {
			value = b.colorAlpha(value, modifier, modifierArbitrary)
			if value == "" {
				return nil
			}
		}
		return [][]*Node{declarations(candidate.Property, value)}
	case "functional":
		value, dataType, arbitrary := "", "", false
		if candidate.Value != nil {
			value, dataType, arbitrary = candidate.Value.Value, candidate.Value.DataType, candidate.Value.Kind == "arbitrary"
		}
		return b.functionalGroups(candidate.Root, value, modifier, dataType, arbitrary, modifierArbitrary)
	}
	return nil
}

func (b *buildContext) functionalGroups(root, value, mod, dataType string, arbitrary, modArbitrary bool) [][]*Node {
	var result [][]*Node
	if nodes := b.functional(root, value, mod, dataType, arbitrary, modArbitrary); len(nodes) > 0 {
		result = append(result, nodes)
	}
	for _, definition := range b.compiler.customFunctional[root] {
		if nodes := b.customUtility(definition, value, mod, dataType, arbitrary, modArbitrary); len(nodes) > 0 {
			result = append(result, nodes)
		}
	}
	return result
}

func (b *buildContext) functional(root, value, mod, dataType string, arbitrary, modArbitrary bool) []*Node {
	negative := strings.HasPrefix(root, "-")
	name := strings.TrimPrefix(root, "-")
	for _, d := range builtinFunctional[name] {
		if negative && !d.Negative {
			continue
		}
		resolved := ""
		if d.Color {
			if value == "" {
				continue
			}
			if arbitrary {
				resolved = value
			} else {
				resolved = b.resolveColor(value, d.Keys)
			}
			resolved = b.colorAlpha(resolved, mod, modArbitrary)
		} else if arbitrary {
			if mod != "" {
				continue
			}
			resolved = value
		} else if value == "" {
			if mod != "" {
				continue
			}
			if d.HasDefault {
				resolved = d.Default
			} else {
				resolved = b.resolve("", d.Keys)
			}
		} else {
			fraction := ""
			if mod != "" && !modArbitrary {
				fraction = value + "/" + mod
			}
			key := value
			if fraction != "" {
				key = fraction
			}
			resolved = b.resolve(key, d.Keys)
			if resolved != "" && mod != "" && fraction == "" {
				continue
			}
			if resolved == "" && d.Fractions && fraction != "" && positiveInteger(value) && positiveInteger(mod) {
				resolved = "calc(" + value + " / " + mod + " * 100%)"
			}
			if resolved == "" {
				resolved = b.bare(d.Bare, value, fraction, negative)
				if !strings.Contains(resolved, "/") && mod != "" {
					continue
				}
				if resolved != "" && (d.Bare == "spacing" || d.Bare == "angle") && negative {
					return instantiate(d, resolved)
				}
			}
			if resolved == "" && !negative && mod == "" {
				if nodes := d.Static[value]; nodes != nil {
					return nodes
				}
			}
		}
		if resolved == "" {
			continue
		}
		if negative {
			resolved = formatMath("calc(" + resolved + " * -1)")
		}
		return instantiate(d, resolved)
	}
	return b.manual(root, value, mod, dataType, arbitrary, modArbitrary)
}

func instantiate(d utilityDefinition, value string) []*Node {
	if d.ZeroTemplate != nil && zeroLength(value) {
		return cloneNodes(d.ZeroTemplate, value, false)
	}
	return cloneNodes(d.Template, value, false)
}

func zeroLength(s string) bool {
	if strings.HasPrefix(s, "calc(") && strings.HasSuffix(s, " * -1)") {
		s = s[5 : len(s)-6]
	}
	i := 0
	for i < len(s) && (digit(s[i]) || strings.ContainsRune("+-.eE", rune(s[i]))) {
		i++
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	return err == nil && n == 0 && (i == len(s) || isLength(s))
}

func (b *buildContext) bare(kind, value, fraction string, negative bool) string {
	switch kind {
	case "spacing":
		if !quarterNumber(value) {
			return ""
		}
		spacing := b.theme("--spacing")
		if spacing == "" {
			return ""
		}
		if value == "0" {
			return "0px"
		}
		if value == "1" && !negative {
			return spacing
		}
		if negative {
			value = "-" + value
		}
		return "calc(" + spacing + " * " + value + ")"
	case "ratio":
		if a, z, ok := strings.Cut(fraction, "/"); ok && quarterNumber(a) && quarterNumber(z) {
			return fraction
		}
	case "grid":
		if positiveInteger(value) && value != "0" {
			return "repeat(" + value + ", minmax(0, 1fr))"
		}
	case "stretch":
		if strings.HasSuffix(value, "%") {
			n, err := strconv.Atoi(value[:len(value)-1])
			if err == nil && n >= 50 && n <= 200 {
				return value
			}
		}
	case "angle":
		if positiveInteger(value) {
			if negative && value != "0" {
				value = "-" + value
			}
			if value == "0" || value == "1" || value == "-1" {
				return value + "deg"
			}
			return "calc(1deg * " + value + ")"
		}
	case "opacity":
		if quarterNumber(value) {
			return value + "%"
		}
	default:
		if strings.HasPrefix(kind, "integer") && positiveInteger(value) {
			return value + kind[len("integer"):]
		}
	}
	return ""
}

func positiveInteger(s string) bool {
	if s == "" || len(s) > 1 && s[0] == '0' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !digit(s[i]) {
			return false
		}
	}
	return true
}

func quarterNumber(s string) bool {
	n, err := strconv.ParseFloat(s, 64)
	return err == nil && n >= 0 && n*4 == float64(int64(n*4)) && strconv.FormatFloat(n, 'f', -1, 64) == s
}

func (b *buildContext) resolveColor(s string, keys []string) string {
	switch s {
	case "inherit", "transparent":
		return s
	case "current":
		return "currentcolor"
	}
	return b.resolve(s, keys)
}

func (b *buildContext) colorAlpha(value, mod string, arbitrary bool) string {
	if value == "" || mod == "" {
		return value
	}
	alpha := mod
	if !arbitrary {
		alpha = b.resolve(mod, []string{"--opacity"})
		if alpha == "" {
			if !quarterNumber(mod) {
				return ""
			}
			alpha = mod + "%"
		}
	}
	if n, err := strconv.ParseFloat(alpha, 64); err == nil {
		alpha = strconv.FormatFloat(n*100, 'f', -1, 64) + "%"
	}
	if alpha == "100%" {
		return value
	}
	return "color-mix(in oklab, " + value + " " + alpha + ", transparent)"
}

func declarations(property, value string) []*Node {
	return []*Node{{Kind: "declaration", Name: property, Value: value}}
}

func cloneNode(n *Node, value string, important bool) *Node {
	r := *n
	r.Value = strings.ReplaceAll(r.Value, "__ROCKETWIND_VALUE__", value)
	if r.Kind == "declaration" && !strings.HasPrefix(r.Name, "--tw-sort") {
		r.Important = r.Important || important
	}
	if n.Kind == "at-root" {
		important = false
	}
	if n.Nodes != nil {
		r.Nodes = cloneNodes(n.Nodes, value, important)
	}
	return &r
}

func cloneNodes(nodes []*Node, value string, important bool) []*Node {
	result := make([]*Node, len(nodes))
	for i, n := range nodes {
		result[i] = cloneNode(n, value, important)
	}
	return result
}

func sortProperties(nodes []*Node) ([]int, int) {
	var order []int
	count := 0
	seenSort := false
	queue := slices.Clone(nodes)
	for i := 0; i < len(queue); i++ {
		n := queue[i]
		if n.Kind == "declaration" {
			count++
			if seenSort {
				continue
			}
			if n.Name == "--tw-sort" {
				if p, ok := propertyOrder[n.Value]; ok {
					order = append(order, p)
					seenSort = true
				}
				continue
			}
			if p, ok := propertyOrder[n.Name]; ok {
				order = append(order, p)
			}
		} else if n.Kind == "rule" || n.Kind == "at-rule" {
			queue = append(queue, n.Nodes...)
		}
	}
	slices.Sort(order)
	return slices.Compact(order), count
}

// Tailwind compares digit runs numerically, including runs after decimal points.
func naturalCompare(a, z string) int {
	for i := 0; i < min(len(a), len(z)); i++ {
		if digit(a[i]) && digit(z[i]) {
			aEnd, zEnd := i+1, i+1
			for aEnd < len(a) && digit(a[aEnd]) {
				aEnd++
			}
			for zEnd < len(z) && digit(z[zEnd]) {
				zEnd++
			}
			x, _ := strconv.ParseFloat(a[i:aEnd], 64)
			y, _ := strconv.ParseFloat(z[i:zEnd], 64)
			if v := cmp.Compare(x, y); v != 0 {
				return v
			}
			if v := strings.Compare(a[i:aEnd], z[i:zEnd]); v != 0 {
				return v
			}
		} else if v := cmp.Compare(a[i], z[i]); v != 0 {
			return v
		}
	}
	return cmp.Compare(len(a), len(z))
}

func ignoredThemeKey(key, namespace string) bool {
	var ignores []string
	switch namespace {
	case "--font":
		ignores = []string{"--font-weight", "--font-size"}
	case "--inset":
		ignores = []string{"--inset-shadow", "--inset-ring"}
	case "--text":
		ignores = []string{"--text-color", "--text-decoration-color", "--text-decoration-thickness", "--text-indent", "--text-shadow", "--text-underline-offset"}
	case "--grid-column":
		ignores = []string{"--grid-column-start", "--grid-column-end"}
	case "--grid-row":
		ignores = []string{"--grid-row-start", "--grid-row-end"}
	}
	for _, prefix := range ignores {
		if key == prefix || strings.HasPrefix(key, prefix+"-") {
			return true
		}
	}
	return false
}
