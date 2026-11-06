// Standard plugin behavior follows the pinned Tailwind Labs sources.
// MIT licenses and source revisions are in testdata/plugins/.
package rocketwind

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

func standardModule(name string) (Module, bool) {
	switch name {
	case "@tailwindcss/aspect-ratio":
		return Module{Plugin: AspectRatio(AspectRatioOptions{})}, true
	case "@tailwindcss/line-clamp":
		return Module{Plugin: LineClamp(LineClampOptions{})}, true
	case "@tailwindcss/container-queries":
		return Module{Plugin: ContainerQueries(ContainerQueriesOptions{})}, true
	case "@tailwindcss/typography":
		return Module{Legacy: ConfigObject{{"theme", ConfigObject{{"typography", typographyTheme}}}}, Configure: func(options map[string]any) (Plugin, error) {
			className, _ := options["className"].(string)
			target, _ := options["target"].(string)
			return Typography(TypographyOptions{ClassName: className, Legacy: target == "legacy"}), nil
		}}, true
	case "@tailwindcss/forms":
		return Module{Configure: func(options map[string]any) (Plugin, error) {
			strategy, _ := options["strategy"].(string)
			if _, ok := options["strategy"]; ok && strategy == "" {
				strategy = "invalid"
			}
			return Forms(FormsOptions{Strategy: strategy}), nil
		}}, true
	}
	return Module{}, false
}

// AspectRatioOptions selects named aspect-w-* and aspect-h-* values. Nil uses
// the original plugin's 1 through 16 defaults and configured theme values.
type AspectRatioOptions struct{ Values []NamedValue }

// AspectRatio is the Go port of @tailwindcss/aspect-ratio v0.4.2. It uses the
// padding-based layout supported by browsers predating native aspect-ratio.
func AspectRatio(options AspectRatioOptions) Plugin {
	return func(api *PluginAPI) error {
		values := options.Values
		if values == nil {
			values = numberedPluginValues(16)
		}
		values = pluginThemeValues(api, "--aspect", values)
		// Both names and callbacks below are fixed, valid registrations.
		_ = api.MatchUtility(Utility{Name: "aspect-w", Values: values, Compile: func(value, modifier string) []*Node {
			return []*Node{
				{Kind: "declaration", Name: "position", Value: "relative"},
				{Kind: "declaration", Name: "padding-bottom", Value: "calc(var(--tw-aspect-h) / var(--tw-aspect-w) * 100%)"},
				{Kind: "declaration", Name: "--tw-aspect-w", Value: value},
				{Kind: "rule", Name: "> *", Nodes: aspectChildren(false)},
			}
		}})
		_ = api.MatchUtility(Utility{Name: "aspect-h", Values: values, Compile: func(value, modifier string) []*Node { return declarations("--tw-aspect-h", value) }})
		return api.AddComponents([]*Node{{Kind: "rule", Name: ".aspect-none", Nodes: []*Node{
			{Kind: "declaration", Name: "position", Value: "static"},
			{Kind: "declaration", Name: "padding-bottom", Value: "0"},
			{Kind: "rule", Name: "& > *", Nodes: aspectChildren(true)},
		}}})
	}
}

func aspectChildren(none bool) []*Node {
	position, size, offset := "absolute", "100%", "0"
	if none {
		position, size, offset = "static", "auto", "auto"
	}
	return []*Node{
		{Kind: "declaration", Name: "position", Value: position},
		{Kind: "declaration", Name: "height", Value: size},
		{Kind: "declaration", Name: "width", Value: size},
		{Kind: "declaration", Name: "top", Value: offset},
		{Kind: "declaration", Name: "right", Value: offset},
		{Kind: "declaration", Name: "bottom", Value: offset},
		{Kind: "declaration", Name: "left", Value: offset},
	}
}

// LineClampOptions selects named values. Nil uses 1 through 6 and the theme.
type LineClampOptions struct{ Values []NamedValue }

// LineClamp ports @tailwindcss/line-clamp v0.4.4 for existing configurations.
// Modern Tailwind already provides line-clamp utilities without this plugin.
func LineClamp(options LineClampOptions) Plugin {
	return func(api *PluginAPI) error {
		values := options.Values
		if values == nil {
			values = numberedPluginValues(6)
		}
		values = pluginThemeValues(api, "--line-clamp", values)
		_ = api.MatchUtility(Utility{Name: "line-clamp", Values: values, Compile: func(value, modifier string) []*Node {
			return []*Node{
				{Kind: "declaration", Name: "overflow", Value: "hidden"},
				{Kind: "declaration", Name: "display", Value: "-webkit-box"},
				{Kind: "declaration", Name: "-webkit-box-orient", Value: "vertical"},
				{Kind: "declaration", Name: "-webkit-line-clamp", Value: value},
			}
		}})
		return api.AddUtilities([]*Node{{Kind: "rule", Name: ".line-clamp-none", Nodes: declarations("-webkit-line-clamp", "unset")}})
	}
}

// ContainerQueriesOptions selects named container widths. Nil uses the
// original plugin's xs through 7xl defaults and configured container values.
type ContainerQueriesOptions struct{ Values []NamedValue }

// ContainerQueries ports @tailwindcss/container-queries v0.1.1. Container
// utilities and variants are also built into modern Tailwind without a plugin.
func ContainerQueries(options ContainerQueriesOptions) Plugin {
	return func(api *PluginAPI) error {
		values := options.Values
		if values == nil {
			values = []NamedValue{{"xs", "20rem"}, {"sm", "24rem"}, {"md", "28rem"}, {"lg", "32rem"}, {"xl", "36rem"}, {"2xl", "42rem"}, {"3xl", "48rem"}, {"4xl", "56rem"}, {"5xl", "64rem"}, {"6xl", "72rem"}, {"7xl", "80rem"}}
		}
		values = pluginThemeValues(api, "--container", values)
		api.MatchVariant(Variant{Name: "@", Values: values, Match: func(value, modifier string) string {
			if _, ok := containerPluginNumber(value); !ok {
				return ""
			}
			return "@container " + modifier + " (min-width: " + value + ")"
		}, Sort: func(a, b VariantValue) int {
			x, _ := containerPluginNumber(a.Value)
			y, _ := containerPluginNumber(b.Value)
			if d := cmp.Compare(x, y); d != 0 {
				return d
			}
			if a.Modifier == "" && b.Modifier != "" {
				return 1
			}
			if a.Modifier != "" && b.Modifier == "" {
				return -1
			}
			return naturalCompare(a.Modifier, b.Modifier)
		}})
		return api.MatchUtility(Utility{Name: "@container", Values: []NamedValue{{"DEFAULT", "inline-size"}, {"normal", "normal"}}, AllowAnyModifier: true, Compile: func(value, modifier string) []*Node {
			nodes := declarations("container-type", value)
			if modifier != "" {
				nodes = append(nodes, declarations("container-name", modifier)...)
			}
			return nodes
		}})
	}
}

func numberedPluginValues(max int) []NamedValue {
	values := make([]NamedValue, max)
	for i := range values {
		value := strconv.Itoa(i + 1)
		values[i] = NamedValue{value, value}
	}
	return values
}

// Theme namespaces override plugin defaults, as in the CSS configuration API.
func pluginThemeValues(api *PluginAPI, namespace string, values []NamedValue) []NamedValue {
	values = slices.Clone(values)
	c := api.compiler
	if c.config.Theme == nil {
		c.config.Theme = map[string]string{}
	}
	if c.themeFlags == nil {
		c.themeFlags = map[string]string{}
	}
	prefix := namespace + "-"
	keys := make([]string, 0)
	for key := range c.config.Theme {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		name := strings.TrimPrefix(key, prefix)
		if strings.Contains(name, "--") || c.config.Theme[key] == "" {
			continue
		}
		found := false
		for i := range values {
			if values[i].Name == name {
				values[i].Value = c.config.Theme[key]
				found = true
				break
			}
		}
		if !found {
			values = append(values, NamedValue{name, c.config.Theme[key]})
		}
	}
	for _, value := range values {
		key := prefix + value.Name
		if _, ok := c.config.Theme[key]; !ok {
			c.config.Theme[key] = value.Value
			c.themeFlags[key] = "inline reference"
			c.themeOrder = append(c.themeOrder, key)
		}
	}
	return values
}

func containerPluginNumber(value string) (float64, bool) {
	i := 0
	for i < len(value) && digit(value[i]) {
		i++
	}
	if i < len(value) && value[i] == '.' {
		i++
		for i < len(value) && digit(value[i]) {
			i++
		}
	}
	if i == 0 || i == len(value) {
		return 0, false
	}
	v, err := strconv.ParseFloat(value[:i], 64)
	return v, err == nil
}
