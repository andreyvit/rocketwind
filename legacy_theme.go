package rocketwind

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ThemeResolver reads a theme value, with an optional fallback.
type ThemeResolver func(path string, fallback ...any) any

// ThemeFunc computes a configuration value in Go. Theme references are resolved
// lazily, so a callback can refer to a namespace declared later in the config.
type ThemeFunc func(ThemeResolver) any

// BareValue resolves values outside a plugin's named table.
type BareValue func(value, fraction string) (string, bool)

type legacyThemeMerge []any

func legacyBare(kind string) BareValue {
	return func(value, fraction string) (string, bool) {
		if kind == "ratio" {
			a, b, ok := strings.Cut(fraction, "/")
			if !ok || !positiveInteger(a) || !positiveInteger(b) {
				return "", false
			}
			return fraction, true
		}
		if !positiveInteger(value) {
			return "", false
		}
		if kind == "integer" {
			return value, true
		}
		if kind == "repeat" {
			return "repeat(" + value + ", minmax(0, 1fr))", true
		}
		return value + kind, true
	}
}

func (c *Compiler) initializeLegacyTheme() {
	if c.legacyTheme != nil {
		return
	}
	c.legacyTheme = configObject(cloneConfigValue(legacyThemeDefaults))
	c.legacyResolved = map[string]bool{}
	c.legacyResolving = map[string]bool{}
	for _, entry := range []NamedValue{{"fontSize", "text"}, {"boxShadow", "shadow"}, {"animation", "animate"}, {"aspectRatio", "aspect"}, {"borderRadius", "radius"}, {"screens", "breakpoint"}, {"letterSpacing", "tracking"}, {"lineHeight", "leading"}, {"maxWidth", "container"}} {
		c.legacyTheme.set(entry.Name, legacyThemeMerge{c.legacyTheme.Get(entry.Name), ConfigObject{{"\x00theme:" + entry.Value, true}}})
	}
	for _, entry := range []NamedValue{{"transitionDuration", "--default-transition-duration"}, {"transitionTimingFunction", "--default-transition-timing-function"}} {
		b := buildContext{compiler: c}
		value := b.themeValue(entry.Value)
		var defaultValue any
		if value != "" {
			defaultValue = value
		}
		c.legacyTheme.set(entry.Name, legacyThemeMerge{c.legacyTheme.Get(entry.Name), ConfigObject{{"DEFAULT", defaultValue}}})
	}
}

func (c *Compiler) resolveLegacyNamespace(name string) any {
	value := c.legacyTheme.Get(name)
	if value == nil {
		return nil
	}
	if c.legacyResolved[name] {
		return value
	}
	if c.legacyResolving[name] {
		c.legacyErr = fmt.Errorf("Circular theme configuration reference: %s", name)
		return nil
	}
	c.legacyResolving[name] = true
	value = c.resolveLegacyValue(value)
	delete(c.legacyResolving, name)
	c.legacyResolved[name] = true
	c.legacyTheme.set(name, value)
	return value
}

func (c *Compiler) resolveLegacyValue(value any) any {
	switch value := value.(type) {
	case ThemeFunc:
		return c.resolveLegacyValue(value(c.themeLookup))
	case legacyThemeMerge:
		var merged any = ConfigObject{}
		for _, part := range value {
			merged = mergeConfigValue(merged, c.resolveLegacyValue(part))
		}
		return merged
	case []any:
		result := make([]any, len(value))
		for i, entry := range value {
			result[i] = c.resolveLegacyValue(entry)
		}
		return result
	}
	if object := configObject(value); object != nil {
		result := make(ConfigObject, 0, len(object))
		for _, entry := range object {
			if path, ok := strings.CutPrefix(entry.Key, "\x00theme:"); ok {
				for _, item := range configObject(c.themeLookup(path)) {
					result.set(item.Key, cloneConfigValue(item.Value))
				}
			} else {
				result.set(entry.Key, c.resolveLegacyValue(entry.Value))
			}
		}
		return result
	}
	return value
}

func (c *Compiler) resolveAllLegacyTheme() error {
	if c.legacyTheme == nil {
		return nil
	}
	for _, entry := range slices.Clone(c.legacyTheme) {
		c.resolveLegacyNamespace(entry.Key)
	}
	return c.legacyErr
}

// ThemeValue returns a scalar, tuple or ordered object, preserving the shape
// of the Tailwind plugin theme API. Returned objects can be changed by callers.
func (api *PluginAPI) ThemeValue(path string, fallback ...any) any {
	api.compiler.initializeLegacyTheme()
	return cloneConfigValue(api.compiler.themeLookup(path, fallback...))
}

func (c *Compiler) themeLookup(path string, fallback ...any) any {
	path, alpha := splitThemeAlpha(path)
	keys := themeKeyPath(path)
	var configured any
	if len(keys) > 0 {
		configured = c.resolveLegacyNamespace(keys[0])
		configured = configGet(configured, keys[1:])
	}
	if text, ok := configured.(string); ok {
		configured = strings.Replace(text, "<alpha-value>", "1", 1)
	}
	css, flags := c.cssThemeObject(keys)
	value := css
	if css == nil {
		value = configured
	} else if flag, ok := flags.(int); ok {
		if flag&4 != 0 && configured != nil {
			value = configured
		}
	} else if object := configObject(configured); object != nil {
		merged := configObject(cloneConfigValue(object))
		for _, entry := range configObject(css) {
			if entry.Key == "__CSS_VALUES__" {
				continue
			}
			if flag, _ := configObject(flags).Get(entry.Key).(int); flag&4 != 0 && merged.Get(entry.Key) != nil {
				continue
			}
			merged.set(Unescape(entry.Key), cloneConfigValue(entry.Value))
		}
		value = merged
	} else if a, ok := css.([]any); ok {
		if b, ok := configured.([]any); ok && len(a) == 2 && len(b) == 2 {
			f, _ := flags.([]any)
			if len(f) == 2 {
				if flag, _ := f[0].(int); flag&4 != 0 && b[0] != nil {
					a[0] = b[0]
				}
				for i, entry := range configObject(a[1]) {
					flag, _ := configObject(f[1]).Get(entry.Key).(int)
					if replacement := configObject(b[1]).Get(entry.Key); flag&4 != 0 && replacement != nil {
						a[1].(ConfigObject)[i].Value = replacement
					}
				}
			}
		}
		value = a
	} else if len(keys) > 1 {
		if base := configObject(css).Get("DEFAULT"); base != nil {
			value = base
		}
	}
	if text, ok := value.(string); ok && alpha != "" {
		b := buildContext{compiler: c}
		value = b.colorAlpha(text, alpha, true)
	}
	if value == nil && len(fallback) > 0 {
		return fallback[0]
	}
	return value
}

func splitThemeAlpha(path string) (string, string) {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return strings.TrimSpace(path[:i]), strings.TrimSpace(path[i+1:])
	}
	return path, ""
}

func (c *Compiler) cssThemeObject(path []string) (any, any) {
	if len(path) == 0 {
		return nil, 0
	}
	b := buildContext{compiler: c}
	if len(path) == 1 && strings.HasPrefix(path[0], "--") {
		if value := b.themeValue(path[0]); value != "" {
			return value, c.themeOptionBits(path[0])
		}
		return nil, 0
	}
	namespace := legacyThemeKey(strings.Join(path, "."))
	if namespace == "" {
		return nil, 0
	}
	var values, flags ConfigObject
	for _, key := range c.themeKeys() {
		if key != namespace && !strings.HasPrefix(key, namespace+"-") || ignoredThemeKey(key, namespace) {
			continue
		}
		value := b.themeValue(key)
		if value == "" {
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(key, namespace), "-")
		if name == "" {
			name = "DEFAULT"
		}
		values.set(name, value)
		flags.set(name, c.themeOptionBits(key))
	}
	if len(values) == 0 {
		return nil, 0
	}
	var result, options ConfigObject
	for _, entry := range values {
		name, extra, tuple := strings.Cut(entry.Key, "--")
		if strings.HasPrefix(entry.Key, "-") {
			name, extra, tuple = "DEFAULT", entry.Key[1:], true
		}
		if tuple {
			base := values.Get(name)
			if base == nil {
				continue
			}
			key := camelCase(extra)
			if old, ok := result.Get(name).([]any); ok {
				object := old[1].(ConfigObject)
				object.set(key, entry.Value)
				old[1] = object
			} else {
				result.set(name, []any{base, ConfigObject{{key, entry.Value}}})
			}
			if old, ok := options.Get(name).([]any); ok {
				object := old[1].(ConfigObject)
				object.set(key, flags.Get(entry.Key))
				old[1] = object
			} else {
				options.set(name, []any{c.themeOptionBits(namespace), ConfigObject{{key, flags.Get(entry.Key)}}})
			}
		} else if result.Get(entry.Key) == nil {
			result.set(entry.Key, entry.Value)
			options.set(entry.Key, flags.Get(entry.Key))
		}
	}
	if path[len(path)-1] == "DEFAULT" || len(path) > 1 && len(result) == 1 && result[0].Key == "DEFAULT" {
		return result.Get("DEFAULT"), options.Get("DEFAULT")
	}
	result.set("__CSS_VALUES__", options)
	return result, options
}

func (c *Compiler) themeKeys() []string {
	keys := slices.Clone(c.themeOrder)
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		seen[key] = true
	}
	var extra []string
	for key := range c.config.Theme {
		if !seen[key] {
			extra = append(extra, key)
			seen[key] = true
		}
	}
	if !c.config.NoDefaultTheme {
		for key := range builtinTheme {
			if !seen[key] {
				extra = append(extra, key)
			}
		}
	}
	slices.Sort(extra)
	return append(keys, extra...)
}

func (c *Compiler) themeOptionBits(key string) int {
	flags := c.themeFlags[key]
	bits := 0
	for i, flag := range []string{"inline", "reference", "default", "static", "used"} {
		if strings.Contains(flags, flag) {
			bits |= 1 << i
		}
	}
	if _, ok := c.config.Theme[key]; !ok && !c.config.NoDefaultTheme {
		bits |= 4
	}
	return bits
}

func camelCase(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '-' && i+1 < len(value) && value[i+1] >= 'a' && value[i+1] <= 'z' {
			i++
			out.WriteByte(value[i] - 'a' + 'A')
		} else {
			out.WriteByte(value[i])
		}
	}
	return out.String()
}

func themeString(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case int:
		return strconv.Itoa(value)
	case []any:
		if len(value) == 2 && configObject(value[1]) != nil {
			return themeString(value[0])
		}
		parts := make([]string, len(value))
		for i, v := range value {
			parts[i] = themeString(v)
		}
		return strings.Join(parts, ", ")
	}
	return ""
}
