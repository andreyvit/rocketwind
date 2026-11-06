package rocketwind

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

func (c *Compiler) finishV3Legacy() error {
	resolved := c.legacyConfig
	if prefix, ok := resolved.Get("prefix").(string); ok && prefix != "" {
		c.v3Prefix = prefix
	}
	if important, ok := resolved.Get("important").(bool); ok {
		c.config.Important = important
	}
	if selector, ok := resolved.Get("important").(string); ok {
		c.importantSelector = selector
	}
	if blocked, ok := resolved.Get("blocklist").([]any); ok {
		c.blocklist = map[string]bool{}
		for _, value := range blocked {
			if class, ok := value.(string); ok {
				c.blocklist[class] = true
			}
		}
	}
	c.configureDarkMode(resolved.Get("darkMode"))
	var classes []ClassSuggestion
	if safelist, ok := resolved.Get("safelist").([]any); ok {
		for _, entry := range safelist {
			if class, ok := entry.(string); ok {
				c.inlineCandidates = append(c.inlineCandidates, class)
				continue
			}
			object := configObject(entry)
			pattern, ok := object.Get("pattern").(*regexp.Regexp)
			if !ok || pattern == nil {
				value, ok := object.Get("pattern").(string)
				if !ok {
					return fmt.Errorf("safelist pattern must be a string or *regexp.Regexp")
				}
				var err error
				pattern, err = regexp.Compile(value)
				if err != nil {
					return fmt.Errorf("safelist pattern: %w", err)
				}
			}
			if classes == nil {
				classes = c.v3ClassList()
			}
			variants, _ := object.Get("variants").([]any)
			for _, class := range classes {
				names := []string{class.Name}
				for _, modifier := range class.Modifiers {
					names = append(names, class.Name+"/"+modifier)
				}
				if strings.Contains(pattern.String(), "!") {
					names = append(names, "!"+class.Name)
				}
				for _, name := range names {
					if !pattern.MatchString(name) {
						continue
					}
					c.inlineCandidates = append(c.inlineCandidates, name)
					for _, variant := range variants {
						c.inlineCandidates = append(c.inlineCandidates, themeString(variant)+":"+name)
					}
				}
			}
		}
	}
	return nil
}

type v3CompletionDefinition struct {
	Name  string
	Index int
}

func (c *Compiler) v3ClassList() []ClassSuggestion {
	var result []ClassSuggestion
	add := func(root string, values ConfigObject, modifiers []string, negative, filterDefault bool) {
		var negatives []ClassSuggestion
		for _, value := range v3ObjectEntries(values) {
			if value.Value == nil || filterDefault && value.Key == "DEFAULT" {
				continue
			}
			name := root
			if value.Key != "DEFAULT" && value.Key != "" {
				name += "-" + value.Key
			}
			result = append(result, ClassSuggestion{c.v3Prefix + name, slices.Clone(modifiers)})
			if negative && v3Negate(themeString(value.Value)) != "" {
				negatives = append(negatives, ClassSuggestion{"-" + c.v3Prefix + name, slices.Clone(modifiers)})
			}
		}
		result = append(result, negatives...)
	}
	for _, entry := range v3Completions {
		if entry.Index < 0 {
			result = append(result, ClassSuggestion{Name: c.v3Prefix + entry.Name})
			continue
		}
		d := v3Functional[entry.Name][entry.Index]
		values := configObject(c.themeLookup(d.Namespace))
		var modifiers []string
		if slices.Contains(d.Types, "color") {
			values = FlattenColors(values)
			for _, opacity := range configObject(c.themeLookup("opacity")) {
				modifiers = append(modifiers, opacity.Key)
			}
		}
		if d.Namespace == "fontSize" {
			for _, height := range configObject(c.themeLookup("lineHeight")) {
				modifiers = append(modifiers, height.Key)
			}
		}
		add(entry.Name, values, modifiers, d.Negative, d.FilterDefault)
	}
	for _, name := range c.customOrder {
		if c.custom[name] != nil {
			result = append(result, ClassSuggestion{Name: c.v3Prefix + name})
		}
	}
	for _, root := range c.functionalOrder {
		for _, matcher := range c.matchingUtilities[root] {
			if matcher.Name != root {
				continue
			}
			var values ConfigObject
			var modifiers []string
			for _, value := range matcher.Values {
				values = append(values, ConfigEntry{value.Name, value.Value})
			}
			for _, value := range matcher.Modifiers {
				modifiers = append(modifiers, value.Name)
			}
			add(root, values, modifiers, matcher.SupportsNegative, false)
		}
	}
	return result
}

func v3ObjectEntries(object ConfigObject) ConfigObject {
	index := func(key string) uint64 {
		v, err := strconv.ParseUint(key, 10, 32)
		if err != nil || v == 1<<32-1 || strconv.FormatUint(v, 10) != key {
			return 1 << 32
		}
		return v
	}
	object = slices.Clone(object)
	slices.SortStableFunc(object, func(a, b ConfigEntry) int { return cmp.Compare(index(a.Key), index(b.Key)) })
	return object
}

func (c *Compiler) v3ConfiguredPrefix() string {
	if value, ok := c.legacyConfig.Get("prefix").(string); ok && value != "" {
		return value
	}
	return c.v3Prefix
}
