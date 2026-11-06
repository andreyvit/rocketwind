package rocketwind

import (
	"errors"
	"strings"
	"testing"
)

func TestConfiguredVariantWithoutStylesheet(t *testing.T) {
	c := New(Config{Variants: []Variant{{Name: "when", Values: []NamedValue{{"open", "open"}}, Match: func(value, modifier string) string { return "&[data-state=" + value + "]" }}}})
	css, err := c.Build([]string{"when-open:flex"})
	if err != nil || !strings.Contains(css, "[data-state=open]") || !strings.Contains(css, "display: flex") {
		t.Fatalf("%v\n%s", err, css)
	}
}

func TestModuleConfigurationContracts(t *testing.T) {
	called := false
	c, err := Compile(`@theme {--color-brand:blue;} @config "config"; @tailwind utilities;`, Config{LoadModule: func(id, base, kind string) (Module, error) {
		if id != "config" || kind != "config" {
			t.Fatalf("load %q (%s)", id, kind)
		}
		return Module{Theme: map[string]string{"--color-brand": "red"}, Plugin: func(api *PluginAPI) error {
			called = true
			api.AddUtility("registered", declarations("color", "green"))
			plugins := api.ConfigValue("plugins").([]any)
			// Plugin metadata includes the standard dark-mode registration.
			handler := configObject(plugins[len(plugins)-1]).Get("handler").(Plugin)
			return handler(api)
		}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"registered", "bg-brand", "dark:flex"})
	if err != nil || !called || !strings.Contains(css, "--color-brand: blue") || !strings.Contains(css, "color: green") || !strings.Contains(css, "prefers-color-scheme: dark") {
		t.Fatalf("%v\n%s", err, css)
	}
	boom := errors.New("invalid plugin option")
	_, err = Compile(`@plugin "configured" { option: bad; }`, Config{LoadModule: func(id, base, kind string) (Module, error) {
		return Module{Configure: func(options map[string]any) (Plugin, error) {
			if options["option"] != "bad" {
				t.Fatal(options)
			}
			return nil, boom
		}}, nil
	}})
	if !errors.Is(err, boom) {
		t.Fatalf("lost option error: %v", err)
	}
}

func TestPluginCannotHideThemeCycle(t *testing.T) {
	plugin := func(api *PluginAPI) error {
		// A plugin may ignore its configuration callback's return value. The
		// compiler must still reject the unresolved configuration at the end.
		_ = Legacy(ConfigObject{{"theme", ConfigObject{{"recursive", ThemeFunc(func(theme ThemeResolver) any { return theme("recursive") })}}}})(api)
		return nil
	}
	_, err := NewWithPlugins(Config{}, plugin)
	if err == nil || !strings.Contains(err.Error(), "Circular theme configuration") {
		t.Fatal(err)
	}
	_, err = Compile(`@plugin "cycle";`, Config{Plugins: map[string]Plugin{"cycle": plugin}})
	if err == nil || !strings.Contains(err.Error(), "Circular theme configuration") {
		t.Fatal(err)
	}
}

func TestCSSThemeTuplesThroughPluginAPI(t *testing.T) {
	_, err := NewWithPlugins(Config{NoDefaultTheme: true, Theme: map[string]string{
		"--text-caption": "1rem", "--text-caption--line-height": "1.4", "--text-caption--letter-spacing": ".1em",
		"--text-missing--line-height": "2", "--text-blank": "",
	}}, func(api *PluginAPI) error {
		if value := api.ThemeValue("", "fallback"); value != "fallback" {
			t.Fatalf("empty lookup: %#v", value)
		}
		values := configObject(api.ThemeValue("fontSize"))
		caption := values.Get("caption").([]any)
		if caption[0] != "1rem" || configObject(caption[1]).Get("lineHeight") != "1.4" || configObject(caption[1]).Get("letterSpacing") != ".1em" {
			t.Fatalf("caption: %#v", caption)
		}
		if values.Get("missing") != nil || values.Get("blank") != nil {
			t.Fatalf("incomplete tokens: %#v", values)
		}
		caption[0] = "changed"
		if api.ThemeValue("fontSize.caption").([]any)[0] != "1rem" {
			t.Fatal("lookup leaked mutable state")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPluginVariantCompatibility(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		if err := api.AddVariantFormats("ignored", ":merge(.parent) &"); err != nil {
			return err
		}
		if err := api.AddVariantFormats("empty"); err != nil {
			return err
		}
		return api.AddVariantFormats("active", "", "&:active")
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"ignored:flex", "empty:flex", "active:flex"})
	if err != nil || !strings.Contains(css, ":active") || strings.Contains(css, "ignored") || strings.Contains(css, "empty") {
		t.Fatalf("%v\n%s", err, css)
	}
}

func TestThemeFunctionCycle(t *testing.T) {
	_, err := Compile(`@theme { --color-loop:theme(--color-loop); } .a { color:theme(--color-loop); }`, Config{})
	if err == nil || !strings.Contains(err.Error(), "Circular theme function reference") {
		t.Fatalf("cycle: %v", err)
	}
}
