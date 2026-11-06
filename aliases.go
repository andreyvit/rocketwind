package rocketwind

import (
	"slices"
	"strings"
)

// Alias gives a utility another spelling, optionally fixing its value type.
// For example, {Utility: "font", Type: TypeFamilyName} makes family-* unambiguous.
// Aliases are opt-in, apply once, and can override a builtin root.
type Alias struct {
	Utility string
	Type    ValueType
}

func (c *Compiler) aliased(class string) (string, ValueType) {
	parts := slices.Collect(Segments(class, ':'))
	base := parts[len(parts)-1]
	important, negative := "", ""
	if strings.HasSuffix(base, "!") {
		base, important = base[:len(base)-1], "!"
	} else if strings.HasPrefix(base, "!") {
		base, important = base[1:], "!"
	}
	if strings.HasPrefix(base, "-") {
		base, negative = base[1:], "-"
	}
	for end := len(base); end > 0; end = strings.LastIndexByte(base[:end], '-') {
		if alias, ok := c.config.Aliases[base[:end]]; ok {
			resolved := negative + alias.Utility + base[end:]
			if c.config.Compatibility == Tailwind3 {
				resolved = important + resolved
			} else {
				resolved += important
			}
			parts[len(parts)-1] = resolved
			return strings.Join(parts, ":"), alias.Type
		}
	}
	return class, ""
}
