package rocketwind

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type legacyConfigSource struct {
	config ConfigObject
	base   string
}

// Legacy configures Tailwind's legacy object-style configuration from Go.
// Use ConfigObject for ordered fields, or map[string]any for nested objects.
// JavaScript callbacks become ThemeFunc values or ordinary Go plugins.
func Legacy(config ConfigObject) Plugin {
	return func(api *PluginAPI) error {
		return api.compiler.configureLegacy([]legacyConfigSource{{config, api.compiler.config.Base}})
	}
}

// ConfigValue reads a value from resolved legacy configuration. An empty path
// returns the whole configuration. The optional fallback handles missing keys.
func (api *PluginAPI) ConfigValue(path string, fallback ...any) any {
	var value any = api.compiler.legacyConfig
	if path != "" {
		value = configGet(value, themeKeyPath(path))
	}
	if value == nil && len(fallback) > 0 {
		value = fallback[0]
	}
	return cloneConfigValue(value)
}

func (c *Compiler) configureLegacy(sources []legacyConfigSource) error {
	if len(sources) == 0 {
		return nil
	}
	c.initializeLegacyTheme()
	var configs []legacyConfigSource
	var visit func(legacyConfigSource) error
	visit = func(source legacyConfigSource) error {
		source.config = configObject(cloneConfigValue(source.config))
		if presets, ok := source.config.Get("presets").([]any); ok {
			if len(presets) == 0 {
				return fmt.Errorf("Error in the config file/plugin/preset. An empty preset (`preset: []`) is not currently supported.")
			}
			for _, preset := range presets {
				if err := visit(legacyConfigSource{configObject(preset), source.base}); err != nil {
					return err
				}
			}
		}
		if plugins, ok := source.config.Get("plugins").([]any); ok {
			for _, plugin := range plugins {
				if config := configObject(configObject(plugin).Get("config")); config != nil {
					if err := visit(legacyConfigSource{config, source.base}); err != nil {
						return err
					}
				}
			}
		}
		configs = append(configs, source)
		return nil
	}
	for _, source := range sources {
		if err := visit(source); err != nil {
			return err
		}
	}
	var userTheme, extensions ConfigObject
	replaced := map[string]bool{}
	resolved := ConfigObject{{"blocklist", []any{}}, {"future", ConfigObject{}}, {"experimental", ConfigObject{}}, {"prefix", ""}, {"important", false}, {"darkMode", nil}, {"theme", ConfigObject{}}, {"plugins", []any{}}, {"content", ConfigObject{{"files", []any{}}}}}
	var files []any
	for _, source := range configs {
		for _, entry := range source.config {
			if entry.Key != "theme" && entry.Key != "presets" && entry.Key != "content" && entry.Key != "plugins" {
				resolved.set(entry.Key, cloneConfigValue(entry.Value))
			}
		}
		content := source.config.Get("content")
		if object := configObject(content); object != nil {
			content = object.Get("files")
		}
		if paths, ok := content.([]any); ok {
			for _, path := range paths {
				if pattern, ok := path.(string); ok {
					files = append(files, ConfigObject{{"base", source.base}, {"pattern", pattern}})
					c.sources = append(c.sources, Source{Base: source.base, Pattern: pattern})
				} else {
					object := configObject(path)
					if object.Get("raw") != nil {
						var detail bytes.Buffer
						encoder := json.NewEncoder(&detail)
						encoder.SetEscapeHTML(false)
						encoder.SetIndent("", "  ")
						_ = encoder.Encode(object)
						return fmt.Errorf("Error in the config file/plugin/preset. The `content` key contains a `raw` entry:\n\n%s\nThis feature is not currently supported.", detail.String())
					}
					base, pattern := themeString(object.Get("base")), themeString(object.Get("pattern"))
					c.sources = append(c.sources, Source{Base: base, Pattern: pattern})
					files = append(files, cloneConfigValue(path))
				}
			}
		}
		for _, entry := range configObject(source.config.Get("theme")) {
			if entry.Key == "extend" {
				for _, extra := range configObject(entry.Value) {
					values, _ := extensions.Get(extra.Key).([]any)
					extensions.set(extra.Key, append(values, extra.Value))
				}
				continue
			}
			replaced[entry.Key] = true
			userTheme.set(entry.Key, cloneConfigValue(entry.Value))
			value := cloneConfigValue(entry.Value)
			if original, ok := c.legacyTheme.Get(entry.Key).(legacyThemeMerge); ok {
				value = append(legacyThemeMerge{value}, original[1:]...)
			}
			c.legacyTheme.set(entry.Key, value)
		}
	}
	for _, entry := range extensions {
		values := entry.Value.([]any)
		userTheme.set(entry.Key, append(legacyThemeMerge{userTheme.Get(entry.Key)}, values...))
		c.legacyTheme.set(entry.Key, append(legacyThemeMerge{c.legacyTheme.Get(entry.Key)}, values...))
	}
	clear(c.legacyResolved)
	for i, entry := range userTheme {
		userTheme[i].Value = c.resolveLegacyValue(entry.Value)
	}
	if screens := configObject(userTheme.Get("screens")); screens != nil {
		for i, entry := range screens {
			object := configObject(entry.Value)
			if object.Get("raw") == nil && object.Get("max") == nil && object.Get("min") != nil {
				screens[i].Value = object.Get("min")
			}
		}
		userTheme.set("screens", screens)
	}
	if err := c.resolveAllLegacyTheme(); err != nil {
		return err
	}
	resolved.set("theme", cloneConfigValue(c.legacyTheme))
	resolved.set("content", ConfigObject{{"files", files}})
	c.legacyConfig = resolved
	c.legacyPending = true
	c.legacyPendingTheme = userTheme
	c.legacyReplaced = replaced
	return nil
}

func (c *Compiler) finishLegacy() error {
	if !c.legacyPending {
		return nil
	}
	c.legacyPending = false
	userTheme, replaced, resolved := c.legacyPendingTheme, c.legacyReplaced, c.legacyConfig
	if c.config.Theme == nil {
		c.config.Theme = map[string]string{}
	}
	if c.themeFlags == nil {
		c.themeFlags = map[string]string{}
	}
	removed := map[string]bool{}
	for name := range replaced {
		namespace := legacyThemeKey(name)
		if namespace == "" {
			continue
		}
		for _, key := range c.themeKeys() {
			if (key == namespace || strings.HasPrefix(key, namespace+"-")) && c.themeOptionBits(key)&4 != 0 {
				c.config.Theme[key] = ""
				removed[key] = true
			}
		}
	}
	if len(removed) > 0 {
		c.themeOrder = slices.DeleteFunc(c.themeOrder, func(key string) bool { return removed[key] })
	}
	for _, entry := range userTheme {
		if entry.Key == "keyframes" {
			for _, frame := range configObject(entry.Value) {
				c.keyframes = append(c.keyframes, &Node{Kind: "at-rule", Name: "@keyframes", Value: frame.Key, Nodes: configStyleNodes(frame.Value)})
			}
			continue
		}
		c.applyLegacyTheme([]string{entry.Key}, entry.Value)
	}
	for _, font := range []string{"sans", "mono"} {
		value := configObject(userTheme.Get("fontFamily")).Get(font)
		if value == nil || c.themeOptionBits("--font-"+font)&4 == 0 {
			continue
		}
		prefix := "--default-font"
		if font == "mono" {
			prefix = "--default-mono-font"
		}
		family := themeString(value)
		var extra ConfigObject
		if tuple, ok := value.([]any); ok && len(tuple) == 2 {
			extra = configObject(tuple[1])
		}
		for _, entry := range []NamedValue{{prefix + "-family", family}, {prefix + "-feature-settings", themeString(extra.Get("fontFeatureSettings"))}, {prefix + "-variation-settings", themeString(extra.Get("fontVariationSettings"))}} {
			if entry.Value == "" {
				entry.Value = "normal"
			}
			c.config.Theme[entry.Name] = entry.Value
			c.themeFlags[entry.Name] = "inline default"
			c.themeOrder = append(c.themeOrder, entry.Name)
		}
	}
	if prefix, ok := resolved.Get("prefix").(string); ok && prefix != "" && c.config.Prefix == "" {
		prefix = strings.TrimSuffix(prefix, "-")
		for _, ch := range prefix {
			if ch < 'a' || ch > 'z' {
				return fmt.Errorf("The prefix %q is invalid. Prefixes must be lowercase ASCII letters (a-z) only.", prefix)
			}
		}
		c.config.Prefix = prefix
	}
	if important, ok := resolved.Get("important").(bool); ok && important {
		c.config.Important = true
	}
	if selector, ok := resolved.Get("important").(string); ok {
		c.importantSelector = selector
	}
	if blocked, ok := resolved.Get("blocklist").([]any); ok {
		c.blocklist = map[string]bool{}
		for _, class := range blocked {
			if class, ok := class.(string); ok {
				c.blocklist[class] = true
			}
		}
	}
	c.configureDarkMode(resolved.Get("darkMode"))
	c.legacyVariantValues = map[string]ConfigObject{}
	for _, name := range []string{"aria", "data", "supports"} {
		c.legacyVariantValues[name] = configObject(userTheme.Get(name))
	}
	c.registerLegacyScreens(configObject(userTheme.Get("screens")))
	c.registerLegacyContainer(configObject(userTheme.Get("container")))
	c.prepareBreakpoints()
	return nil
}

func (c *Compiler) applyLegacyTheme(path []string, value any) {
	if object := configObject(value); object != nil {
		for _, entry := range object {
			c.applyLegacyTheme(append(slices.Clone(path), entry.Key), entry.Value)
		}
		return
	}
	if array, ok := value.([]any); ok {
		if len(array) == 2 && configObject(array[1]) != nil {
			c.applyLegacyTheme(path, array[0])
			for _, entry := range configObject(array[1]) {
				c.applyLegacyTheme(append(slices.Clone(path), "-"+entry.Key), entry.Value)
			}
		} else if path[0] == "fontSize" && len(array) > 0 {
			c.applyLegacyTheme(path, array[0])
			if len(array) > 1 {
				c.applyLegacyTheme(append(slices.Clone(path), "-line-height"), array[1])
			}
		} else {
			allStrings := true
			for _, entry := range array {
				if _, ok := entry.(string); !ok {
					allStrings = false
					break
				}
			}
			if allStrings {
				c.applyLegacyTheme(path, themeString(array))
			} else {
				for i, entry := range array {
					c.applyLegacyTheme(append(slices.Clone(path), strconv.Itoa(i)), entry)
				}
			}
		}
		return
	}
	text := themeString(value)
	if text == "" {
		return
	}
	key := legacyThemeProperty(path)
	if key == "" {
		return
	}
	if _, ok := c.config.Theme[key]; ok && c.themeOptionBits(key)&4 == 0 {
		return
	}
	text = strings.ReplaceAll(text, "<alpha-value>", "1")
	if path[0] == "opacity" {
		if number, err := strconv.ParseFloat(text, 64); err == nil && number >= 0 && number <= 1 {
			text = strconv.FormatFloat(number*100, 'f', -1, 64) + "%"
		}
	}
	c.config.Theme[key] = text
	c.themeFlags[key] = "inline reference default"
	c.themeOrder = append(c.themeOrder, key)
}

func configStyleNodes(value any) []*Node {
	if array, ok := value.([]any); ok {
		var nodes []*Node
		for _, object := range array {
			nodes = append(nodes, configStyleNodes(object)...)
		}
		return nodes
	}
	var nodes []*Node
	for _, entry := range configObject(value) {
		if entry.Value == nil || entry.Value == false {
			continue
		}
		if configObject(entry.Value) != nil {
			n := &Node{Kind: "rule", Name: entry.Key, Nodes: configStyleNodes(entry.Value)}
			if strings.HasPrefix(entry.Key, "@") {
				n.Kind = "at-rule"
				n.Name, n.Value, _ = strings.Cut(entry.Key, " ")
			}
			nodes = append(nodes, n)
		} else if array, ok := entry.Value.([]any); ok {
			for _, v := range array {
				if text, ok := v.(string); ok {
					nodes = append(nodes, declarations(entry.Key, text)...)
				} else {
					nodes = append(nodes, &Node{Kind: "rule", Name: entry.Key, Nodes: configStyleNodes(v)})
				}
			}
		} else {
			name := entry.Key
			if !strings.HasPrefix(name, "--") {
				if entry.Value == "@slot" {
					nodes = append(nodes, &Node{Kind: "rule", Name: name, Nodes: []*Node{{Kind: "slot"}}})
					continue
				}
				name = cssPropertyName(name)
			}
			value := themeString(entry.Value)
			if value == "" {
				value = fmt.Sprint(entry.Value)
			}
			nodes = append(nodes, declarations(name, value)...)
		}
	}
	return nodes
}

func cssPropertyName(name string) string {
	var out strings.Builder
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch >= 'A' && ch <= 'Z' {
			out.WriteByte('-')
			ch += 'a' - 'A'
		}
		out.WriteByte(ch)
	}
	result := out.String()
	return result
}

func (c *Compiler) configureDarkMode(value any) {
	mode, selector := themeString(value), any(".dark")
	if array, ok := value.([]any); ok && len(array) > 0 {
		mode = themeString(array[0])
		if len(array) > 1 {
			selector = array[1]
		}
	}
	var templates []*Node
	switch mode {
	case "media":
		templates = []*Node{{Kind: "at-rule", Name: "@media", Value: "(prefers-color-scheme: dark)", Nodes: []*Node{{Kind: "slot"}}}}
	case "class":
		templates = []*Node{{Kind: "rule", Name: "&:is(" + themeString(selector) + " *)", Nodes: []*Node{{Kind: "slot"}}}}
	case "selector":
		templates = []*Node{{Kind: "rule", Name: "&:where(" + themeString(selector) + ", " + themeString(selector) + " *)", Nodes: []*Node{{Kind: "slot"}}}}
	case "variant":
		values, ok := selector.([]any)
		if !ok {
			values = []any{selector}
		}
		for _, value := range values {
			selector := themeString(value)
			if selector == ".dark" || !strings.Contains(selector, "&") {
				return
			}
			templates = append(templates, &Node{Kind: "rule", Name: selector, Nodes: []*Node{{Kind: "slot"}}})
		}
	default:
		return
	}
	c.registerVariantTemplate("dark", templates)
}
