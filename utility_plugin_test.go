package rocketwind

import (
	"errors"
	"strings"
	"testing"
)

func TestGoUtilityMatcher(t *testing.T) {
	c, err := NewWithPlugins(Config{DisablePolyfills: PolyfillAll}, func(api *PluginAPI) error {
		api.AddBase([]*Node{{Kind: "rule", Name: "button", Nodes: declarations("font", "inherit")}})
		return api.MatchUtility(Utility{Name: "tabs", Types: []ValueType{TypeNumber}, Values: []NamedValue{{"DEFAULT", "4"}, {"two", "2"}, {"1/2", "0.5"}}, Modifiers: []NamedValue{{"wide", "2"}}, SupportsNegative: true,
			Bare: func(value, fraction string) (string, bool) {
				if positiveInteger(value) {
					if _, denominator, ok := strings.Cut(fraction, "/"); ok && positiveInteger(denominator) {
						return fraction, true
					}
					return value, true
				}
				return "", false
			},
			Compile: func(value, modifier string) []*Node {
				nodes := declarations("tab-size", value)
				if modifier != "" {
					nodes = append(nodes, declarations("--modifier", modifier)...)
				}
				return nodes
			},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"tabs", "tabs-two", "tabs-1/2", "tabs-3/4", "tabs-5/wide", "-tabs-two", "tabs-[7]", "tabs-[number:var(--tabs)]", "tabs-[length:2px]", "tabs-nope", "tabs-two/nope"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"@layer base", "font: inherit", "tab-size: 4", "tab-size: 2", "tab-size: 0.5", "tab-size: 3/4", "--modifier: 2", "tab-size: calc(2 * -1)", "tab-size: 7", "tab-size: var(--tabs)"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %s:\n%s", want, css)
		}
	}
	if strings.Contains(css, "2px") || strings.Contains(css, "nope") {
		t.Fatal(css)
	}
}

func TestGoUtilityColorModifiers(t *testing.T) {
	c, err := NewWithPlugins(Config{DisablePolyfills: PolyfillAll}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "paint", Types: []ValueType{TypeColor}, Values: []NamedValue{{"brand", "red"}, {"current", "blue"}}, Modifiers: []NamedValue{{"half", "0.5"}}, Compile: func(value, modifier string) []*Node { return declarations("color", value) }})
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"paint-brand/50", "paint-brand/half", "paint-brand/[0.25]", "paint-current", "paint-inherit", "paint-transparent", "paint-[red]/bad", "paint-[12px]", "paint", "paint-bad"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"color-mix(in oklab, red 50%, transparent)", "color-mix(in oklab, red 25%, transparent)", "color: blue", "color: inherit", "color: transparent"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %s:\n%s", want, css)
		}
	}
	if strings.Contains(css, "bad") || strings.Contains(css, "12px") {
		t.Fatal(css)
	}
}

func TestGoUtilityArbitraryAndRejection(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		if err := api.MatchUtility(Utility{Name: "size", AllowAnyModifier: true, Compile: func(value, modifier string) []*Node { return declarations("--size", value+"/"+modifier) }}); err != nil {
			return err
		}
		if err := api.MatchUtility(Utility{Name: "zero", Types: []ValueType{TypeLength}, Compile: func(value, modifier string) []*Node { return nil }}); err != nil {
			return err
		}
		return api.MatchUtility(Utility{Name: "foo", Types: []ValueType{TypeNumber}, Values: []NamedValue{{"one", "1"}}, Compile: func(value, modifier string) []*Node { return declarations("--foo", value) }})
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"size-[2px]/whatever", "zero-[1px]", "foo-[red]", "foo-[2]/bad", "foo-one/bad"})
	if err != nil || !strings.Contains(css, "--size: 2px/whatever") || strings.Contains(css, "--foo") || strings.Contains(css, ".zero") {
		t.Fatalf("%s %v", css, err)
	}
}

func TestPluginComponentSelectors(t *testing.T) {
	plugin := func(api *PluginAPI) error {
		return api.AddComponents([]*Node{
			{Kind: "rule", Name: ".button, .link:hover", Nodes: declarations("color", "red")},
			{Kind: "rule", Name: ".button .button:hover", Nodes: declarations("font-weight", "700")},
			{Kind: "rule", Name: ".button:not(.disabled):nth-child(2 of .row)", Nodes: declarations("display", "block")},
			{Kind: "at-rule", Name: "@keyframes", Value: "enter", Nodes: []*Node{{Kind: "rule", Name: "to", Nodes: declarations("opacity", "1")}}},
		})
	}
	c, err := NewWithPlugins(Config{}, plugin)
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"hover:button", "link", "disabled", "row"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".hover\\:button", ".link:hover", ".hover\\:button:hover", "@keyframes enter", ":not(.disabled):nth-child(2 of .row)"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %s:\n%s", want, css)
		}
	}
	if strings.Contains(css, ".disabled {") || strings.Contains(css, ".row {") {
		t.Fatal(css)
	}
	prefixed, err := NewWithPlugins(Config{Prefix: "tw"}, plugin)
	if err != nil {
		t.Fatal(err)
	}
	css, err = prefixed.Build([]string{"tw:button"})
	if err != nil || !strings.Contains(css, ".tw\\:disabled") || !strings.Contains(css, ".tw\\:row") {
		t.Fatalf("%s %v", css, err)
	}
}

func TestGoPluginErrors(t *testing.T) {
	boom := errors.New("registration failed")
	if _, err := NewWithPlugins(Config{}, func(*PluginAPI) error { return boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	for _, name := range []string{"", "-foo", "foo?"} {
		_, err := NewWithPlugins(Config{}, func(api *PluginAPI) error { return api.MatchUtility(Utility{Name: name}) })
		if err == nil || !strings.Contains(err.Error(), "invalid utility name") {
			t.Fatal(err)
		}
	}
	_, err := NewWithPlugins(Config{}, func(api *PluginAPI) error { return api.MatchUtility(Utility{Name: "foo"}) })
	if err == nil || !strings.Contains(err.Error(), "compile function") {
		t.Fatal(err)
	}
	_, err = NewWithPlugins(Config{}, func(api *PluginAPI) error {
		return api.AddUtilities([]*Node{{Kind: "rule", Name: ":not(.foo)", Nodes: declarations("color", "red")}})
	})
	if err == nil || !strings.Contains(err.Error(), "invalid utility selector") {
		t.Fatal(err)
	}
	_, err = NewWithPlugins(Config{}, func(api *PluginAPI) error {
		api.AddUtility("foo", []*Node{{Kind: "at-rule", Name: "@apply", Value: "unknown"}})
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "unknown utility") {
		t.Fatal(err)
	}
	_, err = NewWithPlugins(Config{}, func(api *PluginAPI) error {
		api.AddUtility("foo", []*Node{{Kind: "at-rule", Name: "@variant", Value: "unknown", Nodes: declarations("color", "red")}})
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatal(err)
	}
}

func TestProgrammaticPluginBaseDirectives(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		api.AddBase([]*Node{{Kind: "rule", Name: "button", Nodes: []*Node{{Kind: "at-rule", Name: "@apply", Value: "flex"}, {Kind: "declaration", Name: "margin", Value: "--spacing(2)"}}}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build(nil)
	if err != nil || !strings.Contains(css, "display: flex;") || !strings.Contains(css, "margin: calc(var(--spacing) * 2)") {
		t.Fatalf("%s %v", css, err)
	}
	_, err = NewWithPlugins(Config{NoDefaultTheme: true}, func(api *PluginAPI) error {
		api.AddBase([]*Node{{Kind: "rule", Name: "button", Nodes: declarations("margin", "--spacing(2)")}})
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "--spacing") {
		t.Fatal(err)
	}
}

func TestTemplateNamedFractionMatcher(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "fraction", Values: []NamedValue{{"1/2", "50%"}}, Compile: func(value, modifier string) []*Node { return declarations("width", value) }})
	})
	if err != nil {
		t.Fatal(err)
	}
	template, err := c.Prepare("fraction-{value}/2", TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeNumber, nil }})
	if err != nil {
		t.Fatal(err)
	}
	css, err := template.CSS(".half", func(Parameter) (Value, error) { return Value{CSS: "1", Named: true}, nil })
	if err != nil || !strings.Contains(css, "width: 50%") {
		t.Fatalf("%s %v", css, err)
	}
}
