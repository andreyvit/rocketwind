package rocketwind

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JavaScript runs only while capturing upstream tests. This adapter replays
// declarative plugin registrations through the Go API and checks theme queries.
// Functional fixture callbacks are ported explicitly below.
type upstreamModule struct {
	Args    []string
	Result  struct{ Module json.RawMessage }
	Plugins [][]upstreamOperation
	Error   string
}
type upstreamOperation struct {
	Name   string
	Args   []json.RawMessage
	Result json.RawMessage
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
			module := Module{Theme: map[string]string{}}
			var theme map[string]any
			json.Unmarshal(config["theme"], &theme)
			var flatten func([]string, any)
			flatten = func(path []string, value any) {
				switch value := value.(type) {
				case map[string]any:
					for key, child := range value {
						if key == "extend" {
							flatten(path, child)
						} else {
							flatten(append(append([]string{}, path...), key), child)
						}
					}
				case string:
					module.Theme[legacyThemeKey(strings.Join(path, "."))] = value
				case float64:
					module.Theme[legacyThemeKey(strings.Join(path, "."))] = fmt.Sprint(value)
				}
			}
			flatten(nil, theme)
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

func replayPlugin(api *PluginAPI, operations []upstreamOperation) error {
	for _, op := range operations {
		switch op.Name {
		case "theme":
			var path, want string
			json.Unmarshal(op.Args[0], &path)
			json.Unmarshal(op.Result, &want)
			if got := api.Theme(path); got != want {
				return fmt.Errorf("plugin theme(%q): got %q, want %q", path, got, want)
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
				template = []*Node{{Kind: "rule", Name: selector, Nodes: []*Node{{Kind: "slot"}}}}
			case '[':
				var selectors []string
				json.Unmarshal(op.Args[1], &selectors)
				for _, selector := range selectors {
					template = append(template, &Node{Kind: "rule", Name: selector, Nodes: []*Node{{Kind: "slot"}}})
				}
			default:
				template = upstreamStyleNodes(op.Args[1])
			}
			if err := api.AddVariant(name, template); err != nil {
				return err
			}
		case "matchUtilities", "matchComponents":
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
		default:
			return fmt.Errorf("unported plugin operation: %s", op.Name)
		}
	}
	return nil
}

func upstreamStyleNodes(raw json.RawMessage) []*Node {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.Token()
	var result []*Node
	for decoder.More() {
		key, _ := decoder.Token()
		name := key.(string)
		var value json.RawMessage
		decoder.Decode(&value)
		if value[0] == '{' {
			n := &Node{Kind: "rule", Name: name, Nodes: upstreamStyleNodes(value)}
			if strings.HasPrefix(name, "@") {
				n.Kind = "at-rule"
				n.Name, n.Value, _ = strings.Cut(name, " ")
			}
			result = append(result, n)
		} else {
			var text string
			if value[0] == '"' {
				json.Unmarshal(value, &text)
			} else {
				text = string(value)
			}
			if text == "@slot" && !strings.HasPrefix(name, "--") {
				result = append(result, &Node{Kind: "rule", Name: name, Nodes: []*Node{{Kind: "slot"}}})
			} else {
				result = append(result, &Node{Kind: "declaration", Name: kebabCase(name), Value: text})
			}
		}
	}
	return result
}
