package rocketwind

import "slices"

// Builder accumulates candidates and compiles only additions. Keep one per
// stylesheet or watch session. A Builder is not safe for concurrent use;
// independent builders can share their immutable Compiler.
type Builder struct {
	compiler          *Compiler
	seen              map[string]bool
	rules             []compiledRule
	css               string
	sourceMap         *SourceMap
	built             bool
	externalVariables []string
}

func (c *Compiler) NewBuilder() *Builder { return &Builder{compiler: c, seen: make(map[string]bool)} }

// Build adds candidates and returns the complete stylesheet. Repeating an
// unchanged candidate set returns the cached CSS without allocating.
func (builder *Builder) Build(candidates []string) (string, error) {
	return builder.build(candidates, false)
}

func (builder *Builder) build(candidates []string, track bool) (string, error) {
	c := builder.compiler
	if c.hasInput && c.features == 0 {
		return c.inputCSS, nil
	}
	var local [64]string
	added := local[:0]
	add := func(candidates []string) {
		for _, raw := range candidates {
			if builder.seen[raw] {
				continue
			}
			builder.seen[raw] = true
			added = append(added, raw)
		}
	}
	add(candidates)
	if !builder.built {
		add(c.inlineCandidates)
	}
	if len(added) == 0 && builder.built {
		return builder.css, nil
	}
	b := buildContext{compiler: c, used: map[string]bool{}, track: track}
	var rules []compiledRule
	if !c.hasInput || c.hasUtilities {
		rules = b.compileRules(added)
	}
	if b.err != nil {
		for _, class := range added {
			delete(builder.seen, class)
		}
		return builder.css, b.err
	}
	if len(rules) == 0 && len(b.externalVariables) == 0 && builder.built {
		return builder.css, nil
	}
	builder.externalVariables = append(builder.externalVariables, b.externalVariables...)
	b.externalVariables = builder.externalVariables
	builder.rules = append(builder.rules, rules...)
	slices.SortStableFunc(builder.rules, b.compareRules)
	builder.css = b.renderRules(builder.rules)
	builder.sourceMap = nil
	if track {
		builder.sourceMap = createSourceMap(builder.css, b.mappings).encode()
	}
	builder.built = true
	return builder.css, nil
}

// Reset starts a fresh stylesheet while retaining candidate storage capacity.
func (builder *Builder) Reset() {
	clear(builder.seen)
	clear(builder.rules)
	builder.rules = builder.rules[:0]
	clear(builder.externalVariables)
	builder.externalVariables = builder.externalVariables[:0]
	builder.css = ""
	builder.sourceMap = nil
	builder.built = false
}

// ClassOrder returns each class's position in Tailwind's generated rule order.
// Unknown classes have order -1. Duplicates receive the same position. Existing
// stylesheet exclusions do not affect ordering.
func (c *Compiler) ClassOrder(classes []string) []int {
	b := buildContext{compiler: c, used: map[string]bool{}, inApply: true}
	rules := b.compileRules(classes)
	positions := make(map[string]int, len(rules))
	for _, rule := range rules {
		if _, exists := positions[rule.raw]; !exists {
			positions[rule.raw] = len(positions)
		}
	}
	result := make([]int, len(classes))
	for i, class := range classes {
		position, known := positions[class]
		if !known {
			position = -1
		}
		result[i] = position
	}
	return result
}
