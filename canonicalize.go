// Candidate canonicalization follows Tailwind CSS v4.3.4. See testdata/upstream/LICENSE.
package rocketwind

import (
	"iter"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// CanonicalizeOptions controls which equivalent utility spellings are preferred.
type CanonicalizeOptions struct {
	// Rem enables conversion between rem and px using this root font size.
	Rem *float64
	// Collapse combines utilities such as mt-2, mr-2, mb-2 and ml-2 into m-2.
	Collapse bool
	// LogicalToPhysical treats logical and physical properties as equivalent.
	LogicalToPhysical bool
}

// Canonicalizer reuses compiled signatures across class lists. Like Builder, it
// belongs to one caller; independent instances may share a Compiler concurrently.
type Canonicalizer struct {
	compiler, signatureCompiler *Compiler
	options                     CanonicalizeOptions
	features                    signatureFeatures
	candidates                  map[string]string
	signatures                  map[string]utilitySignature
	utilities                   map[string][]string
	variantSignatures           map[string]string
	variantLookup               map[string][]string
	roots                       []string
	properties                  map[string]map[string][]string
	staticProperties            map[string]map[string][]string
	propertyKeys                map[string][]string
	staticValueKeys             map[string][]string
	propertyGeneration          uint64
	collapseCache               map[string]cachedCollapse
	values                      map[string]canonicalValue
	lastInput, lastOutput       []string
	lastGeneration              uint64
}

type canonicalValue struct {
	value, variables string
}

type utilitySignature struct {
	css          string
	variables    []string
	properties   map[string][]string
	propertyKeys []string
}

// NewCanonicalizer creates a reusable class-list canonicalizer.
func (c *Compiler) NewCanonicalizer(options CanonicalizeOptions) *Canonicalizer {
	if options.Rem != nil {
		rem := *options.Rem
		options.Rem = &rem
	}
	copy := *c
	copy.config.InlineTheme = false
	copy.themeFlags = maps.Clone(c.themeFlags)
	for key, flags := range copy.themeFlags {
		copy.themeFlags[key] = strings.ReplaceAll(flags, "inline", "")
	}
	k := &Canonicalizer{compiler: c, signatureCompiler: &copy, options: options,
		candidates: map[string]string{}, signatures: map[string]utilitySignature{},
		variantSignatures: map[string]string{}, properties: map[string]map[string][]string{}, staticProperties: map[string]map[string][]string{}, propertyKeys: map[string][]string{}, staticValueKeys: map[string][]string{}}
	k.collapseCache = map[string]cachedCollapse{}
	k.values = map[string]canonicalValue{}
	if options.Collapse {
		k.features |= signatureExpand
	}
	if options.LogicalToPhysical {
		k.features |= signatureLogicalToPhysical
	}
	k.roots = slices.Clone(builtinFunctionalOrder)
	for _, name := range c.functionalOrder {
		k.roots = appendUnique(k.roots, name)
	}
	for _, name := range c.customOrder {
		if root, ok := strings.CutSuffix(name, "-*"); ok {
			k.roots = appendUnique(k.roots, root)
		}
	}
	return k
}

// Canonicalize returns equivalent, normalized class names in input order.
// Unknown classes are preserved. Use NewCanonicalizer to amortize indexing.
func (c *Compiler) Canonicalize(classes []string, options CanonicalizeOptions) []string {
	return c.NewCanonicalizer(options).Canonicalize(classes)
}

func (k *Canonicalizer) Canonicalize(classes []string) []string {
	if k.lastInput != nil && k.lastGeneration == k.propertyGeneration && slices.Equal(classes, k.lastInput) {
		return slices.Clone(k.lastOutput)
	}
	generation := k.propertyGeneration
	result := make([]string, 0, len(classes))
	seen := make(map[string]bool, len(classes))
	for _, raw := range classes {
		name := k.canonicalCandidate(raw)
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	if k.options.Collapse && len(result) > 1 {
		result = k.collapse(result)
	}
	// A cold call can discover more replacement utilities while processing later
	// groups. Cache whole lists only once their property index is stable.
	if generation == k.propertyGeneration {
		k.lastInput = append([]string{}, classes...)
		k.lastOutput = slices.Clone(result)
		k.lastGeneration = generation
	}
	return result
}

func (k *Canonicalizer) prefixed(name string) string {
	if p := k.compiler.config.Prefix; p != "" && !strings.HasPrefix(name, p+":") {
		return p + ":" + name
	}
	return name
}

func (k *Canonicalizer) parse(name string) iter.Seq[Candidate] {
	return k.compiler.Parse(k.prefixed(name))
}
func (k *Canonicalizer) print(c Candidate) string { return k.compiler.Print(c) }
func (k *Canonicalizer) unprefixed(c Candidate) string {
	name := k.print(c)
	if p := k.compiler.config.Prefix; p != "" {
		name = strings.TrimPrefix(name, p+":")
	}
	return name
}
func (k *Canonicalizer) same(a, b Candidate) bool { return k.sameNames(k.print(a), k.print(b)) }
func (k *Canonicalizer) sameNames(a, b string) bool {
	sig := k.signature(a).css
	return sig != "" && sig == k.signature(b).css
}

func (k *Canonicalizer) canonicalCandidate(raw string) string {
	if cached, ok := k.candidates[raw]; ok {
		return cached
	}
	result := raw
	for candidate := range k.compiler.Parse(raw) {
		if candidate.Important || len(candidate.Variants) > 0 {
			var variants []string
			for i := len(candidate.Variants) - 1; i >= 0; i-- {
				for _, v := range k.canonicalVariant(candidate.Variants[i]) {
					variants = append(variants, printVariant(v))
				}
			}
			important := candidate.Important
			candidate.Important = false
			candidate.Variants = nil
			result = k.canonicalCandidate(k.print(candidate))
			if len(variants) > 0 {
				if p := k.compiler.config.Prefix; p != "" {
					result = strings.TrimPrefix(result, p+":")
				}
				result = strings.Join(variants, ":") + ":" + result
				result = k.prefixed(result)
			}
			if important {
				result += "!"
			}
			break
		}
		result = k.print(k.canonicalUtility(candidate))
		if result != raw {
			break
		}
	}
	k.candidates[raw] = result
	return result
}

func (k *Canonicalizer) compileApply(compiler *Compiler, name string) []*Node {
	ast, _ := k.compileApplyDetails(compiler, name)
	return ast
}

func (k *Canonicalizer) compileApplyDetails(compiler *Compiler, name string) ([]*Node, bool) {
	names := strings.Fields(name)
	for i := range names {
		names[i] = k.prefixed(names[i])
	}
	b := buildContext{compiler: compiler, inApply: true, used: map[string]bool{}}
	rules := b.compileRules(names)
	if b.err != nil || len(rules) == 0 {
		return nil, false
	}
	valid := map[string]bool{}
	var children []*Node
	for _, rule := range rules {
		valid[rule.raw] = true
		children = append(children, cloneNodes(rule.node.Nodes, "", false)...)
	}
	children = b.evaluateNodes(children)
	if b.err != nil {
		return nil, false
	}
	for _, name := range names {
		if !valid[name] {
			return nil, false
		}
	}
	reuse := k.options.Collapse && len(names) == 1 && !k.compiler.config.InlineTheme
	if reuse {
		meanings := 0
		for range compiler.Parse(names[0]) {
			meanings++
			if meanings > 1 {
				reuse = false
				break
			}
		}
		for key := range b.used {
			if strings.Contains(k.compiler.themeFlags[key], "inline") {
				reuse = false
				break
			}
		}
	}
	return []*Node{{Kind: "rule", Name: ".x", Nodes: children}}, reuse
}

func (k *Canonicalizer) signature(name string) utilitySignature {
	if sig, ok := k.signatures[name]; ok {
		return sig
	}
	var sig utilitySignature
	if ast, reuse := k.compileApplyDetails(k.signatureCompiler, name); len(ast) > 0 {
		if reuse {
			sig.properties = map[string][]string{}
		}
		sig.variables = []string{}
		variables := map[*Node]string{}
		ast = k.canonicalAST(ast, variables)
		var css strings.Builder
		writeCSS(&css, ast, 0)
		sig.css = css.String()
		var collect func([]*Node)
		collect = func(nodes []*Node) {
			for _, n := range nodes {
				if n.Kind == "declaration" {
					sig.variables = append(sig.variables, variables[n])
					if reuse {
						sig.propertyKeys = appendUnique(sig.propertyKeys, n.Name)
						sig.properties[n.Name] = appendUnique(sig.properties[n.Name], n.Value)
					}
				}
				collect(n.Nodes)
			}
		}
		collect(ast)
	}
	k.signatures[name] = sig
	return sig
}

var canonicalHex = regexp.MustCompile(`(?i)#[a-f0-9]{8}|#[a-f0-9]{6}|#[a-f0-9]{4}|#[a-f0-9]{3}`)

func (k *Canonicalizer) canonicalAST(nodes []*Node, variables map[*Node]string) []*Node {
	var result []*Node
	for _, n := range nodes {
		switch n.Kind {
		case "context", "at-root":
			result = append(result, k.canonicalAST(n.Nodes, variables)...)
			continue
		case "comment":
			continue
		case "at-rule":
			if n.Name == "@property" {
				continue
			}
		case "declaration":
			if n.Name == "--tw-sort" {
				continue
			}
			if strings.HasPrefix(n.Name, "--tw-") {
				prop := strings.TrimPrefix(n.Name, "--tw-")
				if prop == "leading" {
					prop = "line-height"
				}
				if prop == "tracking" {
					prop = "letter-spacing"
				}
				duplicate := false
				for _, s := range nodes {
					if s.Kind == "declaration" && s.Name == prop && s.Value == n.Value && s.Important == n.Important {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
			}
			if k.features&signatureExpand != 0 {
				if replacement := expandDeclaration(n, k.features); replacement != nil {
					result = append(result, k.canonicalAST(replacement, variables)...)
					continue
				}
			}
			value := k.canonicalValue(n.Value)
			n.Value = value.value
			if variables != nil {
				variables[n] = value.variables
			}
			if !strings.HasPrefix(n.Name, "--") && strings.Contains(n.Value, "#") {
				n.Value = canonicalHex.ReplaceAllStringFunc(n.Value, strings.ToLower)
			}
		}
		if n.Nodes != nil {
			n.Nodes = k.canonicalAST(n.Nodes, variables)
		}
		if n.Kind == "rule" || n.Kind == "at-rule" {
			seen := map[string]bool{}
			for i := len(n.Nodes) - 1; i >= 0; i-- {
				child := n.Nodes[i]
				if child.Kind != "declaration" {
					continue
				}
				if seen[child.Name] {
					n.Nodes = slices.Delete(n.Nodes, i, i+1)
				}
				seen[child.Name] = true
			}
			slices.SortStableFunc(n.Nodes, func(a, b *Node) int {
				if a.Kind != "declaration" || b.Kind != "declaration" {
					return 0
				}
				return localeCompare(a.Name, b.Name)
			})
		}
		result = append(result, n)
	}
	return result
}

func (k *Canonicalizer) canonicalValue(input string) canonicalValue {
	if cached, ok := k.values[input]; ok {
		return cached
	}
	value := input
	var inlined []string
	if strings.Contains(value, "var(") {
		value, inlined = k.resolveVariables(value)
	}
	nodes := parseValue(value)
	folded := constantFoldNodes(nodes, false, k.options.Rem, true)
	if canonicalizeCalcNodes(nodes) || folded {
		value = valueCSS(nodes)
	}
	result := canonicalValue{printArbitraryValue(value), strings.Join(inlined, " ")}
	k.values[input] = result
	return result
}

func (k *Canonicalizer) themeValue(key string) string {
	if !strings.HasPrefix(key, "--") {
		key = legacyThemeKey(key)
	}
	b := buildContext{compiler: k.compiler}
	return b.themeValue(key)
}

func (k *Canonicalizer) resolveVariables(value string) (string, []string) {
	seen := map[string]bool{}
	var inlined []string
	var walk func([]*valueNode) []*valueNode
	walk = func(nodes []*valueNode) []*valueNode {
		var result []*valueNode
		for _, n := range nodes {
			if n.kind == 'f' && n.value == "var" && (len(n.nodes) == 1 || len(n.nodes) >= 3) {
				key := n.nodes[0].value
				if p := k.compiler.config.Prefix; p != "" {
					key = strings.TrimPrefix(key, "--"+p+"-")
				}
				resolved := k.themeValue(key)
				if !seen[key] {
					seen[key] = true
					switch strings.ToLower(resolved) {
					case "initial", "inherit", "revert", "revert-layer", "revert-rule", "unset":
						resolved = ""
					}
					if resolved != "" {
						if len(n.nodes) == 1 {
							n.nodes = append(n.nodes, parseValue(","+resolved)...)
						}
						if valueCSS(n.nodes) == n.nodes[0].value+","+resolved {
							inlined = appendUnique(inlined, key)
							result = append(result, walk(parseValue(resolved))...)
							continue
						}
					}
				}
			}
			if n.nodes != nil {
				n.nodes = walk(n.nodes)
			}
			result = append(result, n)
		}
		return result
	}
	result := valueCSS(walk(parseValue(value)))
	slices.Sort(inlined)
	return result, inlined
}

func (k *Canonicalizer) indexUtilities() {
	if k.utilities != nil {
		return
	}
	k.utilities = map[string][]string{}
	for _, entry := range k.compiler.ClassList() {
		sig := k.signature(entry.Name).css
		if sig == "" {
			continue
		}
		if strings.HasPrefix(entry.Name, "-") && strings.HasSuffix(entry.Name, "-0") && sig == k.signature(entry.Name[1:]).css {
			continue
		}
		k.utilities[sig] = append(k.utilities[sig], entry.Name)
		if k.options.Collapse {
			k.utilityProperties(entry.Name)
		}
		for _, modifier := range entry.Modifiers {
			if quarterNumber(modifier) {
				continue
			}
			name := entry.Name + "/" + modifier
			if sig := k.signature(name).css; sig != "" {
				k.utilities[sig] = append(k.utilities[sig], name)
				if k.options.Collapse {
					k.utilityProperties(name)
				}
			}
		}
	}
}

func (k *Canonicalizer) utilityProperties(name string) map[string][]string {
	if props, ok := k.properties[name]; ok {
		return props
	}
	props := map[string][]string{}
	k.properties[name] = props
	k.propertyGeneration++
	if sig := k.signatures[name]; sig.properties != nil {
		k.properties[name] = sig.properties
		k.propertyKeys[name] = sig.propertyKeys
		k.registerProperties(name)
		return sig.properties
	}
	var ast []*Node
	// Property discovery uses the first parsed meaning, unlike signatures, which
	// include every meaning through @apply. A later plugin meaning may fail.
	for candidate := range k.parse(name) {
		b := buildContext{compiler: k.compiler, used: map[string]bool{}}
		for _, nodes := range b.candidateGroups(candidate) {
			nodes = b.evaluateNodes(nodes)
			ast = append(ast, &Node{Kind: "rule", Name: ".x", Nodes: cloneNodes(nodes, "", false)})
		}
		if b.err != nil {
			ast = nil
		}
		break
	}
	ast = k.canonicalAST(ast, nil)
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Kind == "declaration" {
				k.propertyKeys[name] = appendUnique(k.propertyKeys[name], n.Name)
				props[n.Name] = appendUnique(props[n.Name], n.Value)
			}
			walk(n.Nodes)
		}
	}
	walk(ast)
	k.registerProperties(name)
	return props
}

func (k *Canonicalizer) registerProperties(name string) {
	for _, property := range k.propertyKeys[name] {
		if k.staticProperties[property] == nil {
			k.staticProperties[property] = map[string][]string{}
		}
		for _, value := range k.properties[name][property] {
			k.staticProperties[property][value] = appendUnique(k.staticProperties[property][value], k.prefixed(name))
			k.staticValueKeys[property] = appendUnique(k.staticValueKeys[property], value)
		}
	}
}
