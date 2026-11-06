package rocketwind

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// JavaScript runs only while capturing upstream tests. This adapter replays
// declarative plugin registrations through the Go API and checks theme queries.
// Functional fixture callbacks are ported explicitly below.
type upstreamModule struct {
	Args   []string
	Result struct {
		Module json.RawMessage
		Base   string
	}
	Plugins [][]upstreamOperation
	Error   string
}
type upstreamOperation struct {
	Name      string
	Args      []json.RawMessage
	Result    json.RawMessage
	Calls     []upstreamCallback
	SortCases []upstreamCallback
}
type upstreamCallback struct {
	Name   string
	Args   []json.RawMessage
	Result json.RawMessage
	Error  string
}

func upstreamModuleLoader(modules []upstreamModule) ModuleLoader {
	return func(id, base, kind string) (Module, error) {
		for _, record := range modules {
			if len(record.Args) != 3 || record.Args[0] != id || record.Args[1] != base || record.Args[2] != kind {
				continue
			}
			if record.Error != "" {
				return Module{}, fmt.Errorf("%s", record.Error)
			}
			var raw map[string]json.RawMessage
			json.Unmarshal(record.Result.Module, &raw)
			config := raw
			if value := raw["config"]; value != nil {
				json.Unmarshal(value, &config)
			}
			module := Module{Base: record.Result.Base}
			var configRaw json.RawMessage
			if kind == "config" {
				configRaw = record.Result.Module
			}
			if value := raw["config"]; value != nil {
				configRaw = value
			}
			json.Unmarshal(configRaw, &module.Legacy)
			var portErr error
			module.Legacy, portErr = portLegacyFixtureConfig(module.Legacy)
			if portErr != nil {
				return Module{}, portErr
			}
			for _, operations := range record.Plugins {
				module.Plugins = append(module.Plugins, func(api *PluginAPI) error { return replayPlugin(api, operations) })
			}
			if strings.Contains(string(raw["$function"]), "function optionsFunction(options)") {
				plugins := module.Plugins
				module.Plugins = nil
				module.Configure = func(options map[string]any) (Plugin, error) {
					return func(api *PluginAPI) error {
						for _, p := range plugins {
							if err := p(api); err != nil {
								return err
							}
						}
						return nil
					}, nil
				}
			}
			return module, nil
		}
		return Module{}, fmt.Errorf("uncaptured module %q from %q (%s)", id, base, kind)
	}
}

func portLegacyFixtureConfig(config ConfigObject) (ConfigObject, error) {
	result := configObject(cloneConfigValue(config))
	var convert func(any) (any, error)
	convert = func(value any) (any, error) {
		object := configObject(value)
		if source, ok := object.Get("$function").(string); ok {
			if strings.Contains(source, "rootWithDefault =") {
				return ThemeFunc(func(theme ThemeResolver) any {
					if configObject(theme("text")).Get("DEFAULT") != "1rem" || configObject(theme("text", ConfigObject{})).Get("DEFAULT") != "1rem" || theme("animate.spin") != "spin 1s linear infinite" {
						return ConfigObject{{"badNamespaceLookup", true}}
					}
					return ConfigObject{}
				}), nil
			}
			if strings.Contains(source, "colors.green") && !strings.Contains(source, "theme(") {
				return ThemeFunc(func(ThemeResolver) any {
					var green ConfigObject
					for key, value := range builtinTheme {
						if shade, ok := strings.CutPrefix(key, "--color-green-"); ok {
							green.set(shade, value)
						}
					}
					return ConfigObject{{"primary", green}}
				}), nil
			}
			if strings.Contains(source, `primary: theme("colors.green")`) {
				return ThemeFunc(func(theme ThemeResolver) any {
					return ConfigObject{{"primary", theme("colors.green")}, {"secondary", theme("color.green")}}
				}), nil
			}
			if strings.Contains(source, "[class~=lead-base]") {
				return ThemeFunc(func(theme ThemeResolver) any {
					var result ConfigObject
					for _, size := range []string{"base", "md", "xl"} {
						tuple, _ := theme("fontSize." + size).([]any)
						style := ConfigObject{}
						if len(tuple) == 2 {
							style = ConfigObject{{"fontSize", tuple[0]}}
							for _, entry := range configObject(tuple[1]) {
								style.set(entry.Key, entry.Value)
							}
						}
						result.set("[class~=lead-"+size+"]", style)
					}
					return result
				}), nil
			}
			matches := regexp.MustCompile(`theme\("([^"]+)"(?:, \{\})?\)`).FindAllStringSubmatch(source, -1)
			if len(matches) == 1 {
				path := matches[0][1]
				return ThemeFunc(func(theme ThemeResolver) any {
					value := theme(path)
					if strings.Contains(source, "spin: theme(") {
						return ConfigObject{{"spin", value}}
					}
					for _, name := range []string{"red", "blue"} {
						if strings.Contains(source, name+`: "`+name+`"`) {
							object := configObject(cloneConfigValue(value))
							object.set(name, name)
							return object
						}
					}
					return value
				}), nil
			}
			if strings.Contains(source, "fontSize.base[1].lineHeight") {
				return ThemeFunc(func(theme ThemeResolver) any {
					var result ConfigObject
					for _, size := range []string{"base", "md", "xl"} {
						result.set(size, theme("fontSize."+size+"[1].lineHeight"))
					}
					return result
				}), nil
			}
			return nil, fmt.Errorf("unported theme callback: %s", source)
		}
		if object != nil {
			for i, entry := range object {
				v, err := convert(entry.Value)
				if err != nil {
					return nil, err
				}
				object[i].Value = v
			}
			return object, nil
		}
		if array, ok := value.([]any); ok {
			for i, v := range array {
				v, err := convert(v)
				if err != nil {
					return nil, err
				}
				array[i] = v
			}
			return array, nil
		}
		return value, nil
	}
	if theme := result.Get("theme"); theme != nil {
		value, err := convert(theme)
		if err != nil {
			return nil, err
		}
		result.set("theme", value)
	}
	for _, key := range []string{"presets", "plugins"} {
		if values, ok := result.Get(key).([]any); ok {
			for i, value := range values {
				object := configObject(value)
				if key == "plugins" {
					object = configObject(object.Get("config"))
					if object == nil {
						continue
					}
				}
				ported, err := portLegacyFixtureConfig(object)
				if err != nil {
					return nil, err
				}
				if key == "plugins" {
					wrapper := configObject(value)
					wrapper.set("config", ported)
					values[i] = wrapper
				} else {
					values[i] = ported
				}
			}
		}
	}
	return result, nil
}

func replayPlugin(api *PluginAPI, operations []upstreamOperation) error {
	for _, op := range operations {
		switch op.Name {
		case "theme":
			var path string
			json.Unmarshal(op.Args[0], &path)
			want, _ := decodeConfigValue(op.Result)
			var fallback []any
			if len(op.Args) > 1 {
				value, _ := decodeConfigValue(op.Args[1])
				fallback = append(fallback, value)
			}
			got := api.ThemeValue(path, fallback...)
			if !reflect.DeepEqual(normalizePluginValue(got), normalizePluginValue(want)) {
				return fmt.Errorf("plugin theme(%q): got %v, want %v", path, got, want)
			}
		case "addBase":
			api.AddBase(upstreamStyleNodes(op.Args[0]))
		case "addUtilities", "addComponents":
			if err := api.AddUtilities(upstreamStyleNodes(op.Args[0])); err != nil {
				return err
			}
		case "addVariant":
			var name string
			json.Unmarshal(op.Args[0], &name)
			var template []*Node
			switch op.Args[1][0] {
			case '"':
				var selector string
				json.Unmarshal(op.Args[1], &selector)
				if err := api.AddVariantFormats(name, selector); err != nil {
					return err
				}
				continue
			case '[':
				var selectors []string
				json.Unmarshal(op.Args[1], &selectors)
				if err := api.AddVariantFormats(name, selectors...); err != nil {
					return err
				}
				continue
			default:
				template = upstreamStyleNodes(op.Args[1])
			}
			if err := api.AddVariant(name, template); err != nil {
				return err
			}
		case "matchUtilities", "matchComponents":
			if op.Calls != nil {
				if err := replayUtilityCallbacks(api, op); err != nil {
					return err
				}
				continue
			}
			var callbacks map[string]json.RawMessage
			var options struct{ Values map[string]string }
			json.Unmarshal(op.Args[0], &callbacks)
			json.Unmarshal(op.Args[1], &options)
			for name := range callbacks {
				for key, value := range options.Values {
					var nodes []*Node
					switch name {
					case "bar":
						nodes = []*Node{{Kind: "at-rule", Name: "@variant", Value: "dark", Nodes: declarations("--bar", value)}}
					case "match-utility", "match-components":
						nodes = []*Node{{Kind: "at-root", Nodes: []*Node{{Kind: "at-rule", Name: "@keyframes", Value: name, Nodes: []*Node{{Kind: "rule", Name: "100%", Nodes: declarations("opacity", "0")}}}}}}
					default:
						return fmt.Errorf("unported functional plugin fixture: %s", name)
					}
					api.AddUtility(name+"-"+key, nodes)
				}
			}
		case "matchVariant":
			if err := replayVariantCallbacks(api, op); err != nil {
				return err
			}
		case "config":
			path := ""
			if len(op.Args) > 0 {
				json.Unmarshal(op.Args[0], &path)
			}
			want, _ := decodeConfigValue(op.Result)
			var fallback []any
			if len(op.Args) > 1 {
				v, _ := decodeConfigValue(op.Args[1])
				fallback = append(fallback, v)
			}
			if got := api.ConfigValue(path, fallback...); !reflect.DeepEqual(normalizePluginValue(got), normalizePluginValue(want)) {
				return fmt.Errorf("config(%q): got %v, want %v", path, got, want)
			}
		case "prefix":
			var class, want string
			json.Unmarshal(op.Args[0], &class)
			json.Unmarshal(op.Result, &want)
			if got := api.Prefix(class); got != want {
				return fmt.Errorf("prefix(%q): got %q, want %q", class, got, want)
			}
		default:
			return fmt.Errorf("unported plugin operation: %s", op.Name)
		}
	}
	return nil
}

func normalizePluginValue(value any) any {
	if object := configObject(value); object != nil {
		if object.Get("$undefined") != nil {
			return nil
		}
		if object.Get("$function") != nil {
			return "__function"
		}
		result := map[string]any{}
		for _, entry := range object {
			result[entry.Key] = normalizePluginValue(entry.Value)
		}
		return result
	}
	if array, ok := value.([]any); ok {
		result := make([]any, len(array))
		for i, v := range array {
			result[i] = normalizePluginValue(v)
		}
		return result
	}
	if value != nil && reflect.TypeOf(value).Kind() == reflect.Func {
		return "__function"
	}
	if value, ok := value.(int); ok {
		return float64(value)
	}
	return value
}

// Callback tables replace only test-local JavaScript functions. Calls and their
// exact arguments are checked; candidate selection and CSS generation still run
// through the real Go compiler. Runtime library code never reads these fixtures.
func replayCallback(api *PluginAPI, calls []upstreamCallback, name string, args ...any) json.RawMessage {
	want := normalizePluginValue(args)
	for _, call := range calls {
		if call.Name != name {
			continue
		}
		got := make([]any, len(call.Args))
		for i, raw := range call.Args {
			got[i], _ = decodeConfigValue(raw)
		}
		if reflect.DeepEqual(normalizePluginValue(got), want) {
			if call.Error != "" {
				api.compiler.legacyErr = fmt.Errorf("callback %s: %s", name, call.Error)
			}
			return call.Result
		}
	}
	api.compiler.legacyErr = fmt.Errorf("unrecorded plugin callback %s(%v)", name, args)
	return nil
}

func pluginCallbackArgs(value any, modifier string) []any {
	var mod any
	if modifier != "" {
		mod = modifier
	}
	return []any{value, ConfigObject{{"modifier", mod}}}
}

func replayUtilityCallbacks(api *PluginAPI, op upstreamOperation) error {
	callbacks, _ := decodeConfigValue(op.Args[0])
	var options ConfigObject
	if len(op.Args) > 1 {
		json.Unmarshal(op.Args[1], &options)
	}
	for _, entry := range configObject(callbacks) {
		name := entry.Key
		u := Utility{Name: name}
		if types, ok := options.Get("type").(string); ok {
			u.Types = []ValueType{ValueType(types)}
		} else if types, ok := options.Get("type").([]any); ok {
			for _, v := range types {
				u.Types = append(u.Types, ValueType(themeString(v)))
			}
		}
		u.SupportsNegative, _ = options.Get("supportsNegativeValues").(bool)
		for _, entry := range configObject(options.Get("values")) {
			if entry.Key == "DEFAULT" && entry.Value == nil {
				u.Values = append(u.Values, NamedValue{entry.Key, ""})
				continue
			}
			if entry.Key == "__BARE_VALUE__" {
				u.Bare = func(value, fraction string) (string, bool) {
					var ratio any
					if fraction != "" {
						ratio = fraction
					}
					raw := replayCallback(api, op.Calls, "__BARE_VALUE__", ConfigObject{{"kind", "named"}, {"value", value}, {"fraction", ratio}})
					var result string
					err := json.Unmarshal(raw, &result)
					return result, err == nil
				}
				continue
			}
			if value := themeString(entry.Value); value != "" {
				u.Values = append(u.Values, NamedValue{entry.Key, value})
			}
		}
		u.AllowAnyModifier = options.Get("modifiers") == "any"
		for _, entry := range configObject(options.Get("modifiers")) {
			u.Modifiers = append(u.Modifiers, NamedValue{entry.Key, themeString(entry.Value)})
		}
		u.CompileWithError = func(value, modifier string) ([]*Node, error) {
			api.compiler.legacyErr = nil
			raw := replayCallback(api, op.Calls, name, pluginCallbackArgs(value, modifier)...)
			if api.compiler.legacyErr != nil {
				return nil, api.compiler.legacyErr
			}
			if len(raw) == 0 || string(raw) == "null" || strings.Contains(string(raw), `"$undefined":true`) {
				return nil, nil
			}
			return upstreamStyleNodes(raw), nil
		}
		if err := api.MatchUtility(u); err != nil {
			return err
		}
	}
	return nil
}

func replayVariantCallbacks(api *PluginAPI, op upstreamOperation) error {
	var name string
	json.Unmarshal(op.Args[0], &name)
	var options ConfigObject
	if len(op.Args) > 2 {
		json.Unmarshal(op.Args[2], &options)
	}
	v := Variant{Name: name}
	var defaultValue any
	defaultIsNull := false
	for _, entry := range configObject(options.Get("values")) {
		if entry.Key == "DEFAULT" && themeString(entry.Value) == "" {
			defaultValue = entry.Value
			defaultIsNull = true
			v.Values = append(v.Values, NamedValue{entry.Key, ""})
		} else if value, ok := entry.Value.(string); ok {
			v.Values = append(v.Values, NamedValue{entry.Key, value})
		}
	}
	v.MatchAll = func(value, modifier string) []string {
		var resolved any = value
		if value == "" && defaultIsNull {
			resolved = defaultValue
		}
		raw := replayCallback(api, op.Calls, "variant", pluginCallbackArgs(resolved, modifier)...)
		var selector string
		if json.Unmarshal(raw, &selector) == nil {
			return []string{selector}
		}
		var selectors []string
		if json.Unmarshal(raw, &selectors) == nil {
			return selectors
		}
		return nil
	}
	if options.Get("sort") != nil {
		v.Sort = func(a, b VariantValue) int {
			convert := func(value VariantValue) ConfigObject {
				var modifier any
				if value.Modifier != "" {
					modifier = value.Modifier
				}
				return ConfigObject{{"value", value.Value}, {"modifier", modifier}}
			}
			raw := replayCallback(api, op.SortCases, "", convert(a), convert(b))
			var result float64
			json.Unmarshal(raw, &result)
			if result < 0 {
				return -1
			}
			if result > 0 {
				return 1
			}
			return 0
		}
	}
	api.MatchVariant(v)
	return nil
}

func upstreamStyleNodes(raw json.RawMessage) []*Node {
	value, _ := decodeConfigValue(raw)
	return configStyleNodes(stripFixtureUndefined(value))
}

func stripFixtureUndefined(value any) any {
	if object := configObject(value); object != nil {
		if object.Get("$undefined") != nil {
			return nil
		}
		result := make(ConfigObject, len(object))
		for i, entry := range object {
			result[i] = ConfigEntry{entry.Key, stripFixtureUndefined(entry.Value)}
		}
		return result
	}
	if array, ok := value.([]any); ok {
		result := make([]any, len(array))
		for i, v := range array {
			result[i] = stripFixtureUndefined(v)
		}
		return result
	}
	return value
}
