package rocketwind

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestOrderedConfiguration(t *testing.T) {
	var config ConfigObject
	input := `{"z":1,"theme":{"colors":{"brand":"red"}},"z":2,"values":["a",{"b":true},null]}`
	if err := json.Unmarshal([]byte(input), &config); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(config)
	if want := `{"z":2,"theme":{"colors":{"brand":"red"}},"values":["a",{"b":true},null]}`; err != nil || string(encoded) != want {
		t.Fatalf("%s %v", encoded, err)
	}
	copy := cloneConfigValue(config).(ConfigObject)
	configObject(configObject(copy.Get("theme")).Get("colors"))[0].Value = "blue"
	if got := configGet(config, []string{"theme", "colors", "brand"}); got != "red" {
		t.Fatal(got)
	}
	if got := configGet(config, []string{"values", "1", "b"}); got != true {
		t.Fatal(got)
	}
	if configGet(config, []string{"values", "-1"}) != nil || configGet(config, []string{"missing"}) != nil {
		t.Fatal("invalid path resolved")
	}
	if got := configGet(ConfigObject{{"a-b", "joined"}}, []string{"a", "b"}); got != "joined" {
		t.Fatal(got)
	}
	if _, err := json.Marshal(ConfigObject{{"fn", ThemeFunc(func(ThemeResolver) any { return nil })}}); err == nil {
		t.Fatal("marshaled a function")
	}
	for _, input := range []string{"", "null", "[]", "{", `{"a"`, `{"a":`, `{"a":1`, `{"a":[]`, `{"a":{x}}`, `{"a":1e999}`, `{"a":[1e999]}`, `{} {}`, `{} !`} {
		t.Run(input, func(t *testing.T) {
			before := cloneConfigValue(config)
			if err := config.UnmarshalJSON([]byte(input)); err == nil {
				t.Fatal("accepted invalid JSON")
			}
			if !reflect.DeepEqual(config, before) {
				t.Fatal("failed decode changed the object")
			}
		})
	}
	for _, input := range []string{"", "[", "[{} !]", "{!}", "false!"} {
		if _, err := decodeConfigValue([]byte(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	got := cloneConfigValue(ConfigObject{{"map", map[string]string{"z": "last", "a": "first"}}, {"slices", []string{"one", "two"}}, {"array", [2]int{1, 2}}})
	want := ConfigObject{{"map", ConfigObject{{"a", "first"}, {"z", "last"}}}, {"slices", []any{"one", "two"}}, {"array", []any{1, 2}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%#v", got)
	}
}

func TestConfigurationExtensionArrays(t *testing.T) {
	a, b := ConfigObject{{"color", "red"}}, ConfigObject{{"color", "blue"}}
	for _, tt := range []struct{ base, extra, want any }{
		{[]any{a}, b, []any{a, b}},
		{[]any{a}, []any{b}, []any{a, b}},
		{a, []any{b}, []any{a, b}},
		{[]any{"serif"}, []any{"sans-serif"}, []any{"sans-serif"}},
		{a, b, b},
	} {
		if got := mergeConfigValue(tt.base, tt.extra); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("merge(%v,%v) = %v, want %v", tt.base, tt.extra, got, tt.want)
		}
	}
}

func TestProgrammaticLegacyConfiguration(t *testing.T) {
	configuration := ConfigObject{
		{"prefix", "tw-"}, {"darkMode", []string{"class", ".night"}}, {"blocklist", []string{"tw:flex"}}, {"content", []string{"./pages/*.html"}},
		{"theme", map[string]any{
			"colors":      map[string]string{"brand": "#123456"},
			"screens":     map[string]string{"tiny": "360px"},
			"fontSize":    map[string]any{"title": []any{"2rem", map[string]string{"lineHeight": "1.2"}}},
			"extend":      map[string]any{"spacing": map[string]string{"huge": "123px"}},
			"borderColor": ThemeFunc(func(theme ThemeResolver) any { return theme("colors") }),
		}},
	}
	c, err := NewWithPlugins(Config{Base: "/templates", DisablePolyfills: PolyfillAll}, Legacy(configuration), func(api *PluginAPI) error {
		if got := api.Theme("colors.brand"); got != "#123456" {
			t.Errorf("brand = %q", got)
		}
		if got := api.ThemeValue("fontSize.title[1].lineHeight"); got != "1.2" {
			t.Errorf("line height = %v", got)
		}
		if got := api.ConfigValue("missing", "fallback"); got != "fallback" {
			t.Errorf("fallback = %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	configuration.set("prefix", "changed")
	classes := []string{"tw:flex", "tw:sm:block", "tw:tiny:block", "tw:text-title", "tw:p-huge", "tw:border-brand", "tw:dark:bg-brand"}
	css, err := c.Build(classes)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"width >= 360px", "font-size: 2rem", "line-height: var(--tw-leading, 1.2)", "padding: 123px", "border-color: #123456", ":is(.night *)", "background-color: #123456"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q:\n%s", want, css)
		}
	}
	for _, absent := range []string{".tw\\:flex", ".tw\\:sm\\:block", "changed"} {
		if strings.Contains(css, absent) {
			t.Errorf("unexpected %q:\n%s", absent, css)
		}
	}
	if got := c.Sources(); !reflect.DeepEqual(got, []Source{{Base: "/templates", Pattern: "./pages/*.html"}}) {
		t.Fatal(got)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				if got, err := c.Build(classes); err != nil || got != css {
					t.Errorf("concurrent build differs: %v", err)
				}
			}
		})
	}
	wg.Wait()
}

func TestLegacyConfigurationErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config ConfigObject
		want   string
	}{
		{"presets", ConfigObject{{"presets", []any{}}}, "empty preset"},
		{"nested preset", ConfigObject{{"presets", []ConfigObject{{{"presets", []any{}}}}}}, "empty preset"},
		{"plugin preset", ConfigObject{{"plugins", []any{ConfigObject{{"config", ConfigObject{{"presets", []any{}}}}}}}}, "empty preset"},
		{"prefix", ConfigObject{{"prefix", "Bad"}}, "invalid"},
		{"cycle", ConfigObject{{"theme", ConfigObject{{"one", ThemeFunc(func(theme ThemeResolver) any { return theme("two") })}, {"two", ThemeFunc(func(theme ThemeResolver) any { return theme("one") })}}}}, "Circular"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewWithPlugins(Config{}, Legacy(tt.config)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestLegacyBareValuesAndCopies(t *testing.T) {
	_, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		for _, tt := range []struct{ path, value, fraction, want string }{
			{"aspectRatio", "3", "3/2", "3/2"}, {"aspectRatio", "3", "", ""}, {"aspectRatio", "3", "3/2.5", ""},
			{"gridTemplateColumns", "3", "", "repeat(3, minmax(0, 1fr))"}, {"gridTemplateColumns", "-1", "", ""},
			{"zIndex", "12", "", "12"}, {"transitionDuration", "250", "", "250ms"}, {"borderWidth", "2", "", "2px"},
		} {
			fn, ok := api.ThemeValue(tt.path + ".__BARE_VALUE__").(BareValue)
			if !ok {
				t.Errorf("%s has no bare matcher", tt.path)
				continue
			}
			value, matched := fn(tt.value, tt.fraction)
			if value != tt.want || matched != (tt.want != "") {
				t.Errorf("%s = %q, %v", tt.path, value, matched)
			}
		}
		colors := configObject(api.ThemeValue("colors"))
		colors.set("red-500", "changed")
		if api.ThemeValue("colors.red-500") == "changed" {
			t.Fatal("theme object was not copied")
		}
		if api.ThemeValue("absent", "fallback") != "fallback" {
			t.Fatal("missing fallback")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSourceDiscoveryMetadata(t *testing.T) {
	c, err := Compile(`@import "tailwindcss" source("../templates"); @source "./other/*.gox"; @source not "./other/skip.gox";`, Config{Base: "/app/css"})
	if err != nil {
		t.Fatal(err)
	}
	want := SourceRoot{Base: "/app/css", Pattern: "../templates"}
	if got := c.Root(); got == nil || *got != want {
		t.Fatalf("root = %#v", got)
	}
	c.Root().Pattern = "changed"
	if *c.Root() != want {
		t.Fatal("root was not copied")
	}
	paths := c.Sources()
	if len(paths) != 2 || paths[0].Base != "/app/css" || !paths[1].Negated {
		t.Fatal(paths)
	}
	paths[0].Pattern = "changed"
	if c.Sources()[0].Pattern == "changed" {
		t.Fatal("sources were not copied")
	}
	if c.Features()&(FeatureImport|FeatureUtilities|FeatureTheme) != FeatureImport|FeatureUtilities|FeatureTheme {
		t.Fatal(c.Features())
	}
	for _, input := range []string{`@tailwind utilities source(none);`, `@tailwind utilities source(unquoted);`} {
		c, err := Compile(input, Config{})
		if strings.Contains(input, "none") {
			if err != nil || c.Root() == nil || !c.Root().Disabled {
				t.Fatal(c, err)
			}
		} else if err == nil {
			t.Fatal("accepted unquoted source")
		}
	}
}
