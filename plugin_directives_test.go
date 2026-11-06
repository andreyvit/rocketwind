package rocketwind

import (
	"strconv"
	"strings"
	"testing"
)

func TestDynamicPluginDirectives(t *testing.T) {
	c, err := NewWithPlugins(Config{DisablePolyfills: PolyfillAll}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "panel", Values: []NamedValue{{"row", "flex"}, {"column", "grid"}}, Compile: func(value, modifier string) []*Node {
			return []*Node{
				{Kind: "at-rule", Name: "@apply", Value: value + " !important"},
				{Kind: "at-rule", Name: "@variant", Value: "hover:focus, active", Nodes: []*Node{{Kind: "at-rule", Name: "@apply", Value: "underline"}}},
				{Kind: "rule", Name: "& > span", Nodes: declarations("color", "red")},
			}
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"panel-row", "panel-column"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"display: flex !important", "display: grid !important", "&:hover", "&:focus", "&:active", "& > span", "text-decoration-line: underline"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q:\n%s", want, css)
		}
	}
	if strings.Contains(css, "@apply") || strings.Contains(css, "@variant") {
		t.Fatal(css)
	}
}

func TestDynamicPluginDirectiveErrors(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"unknown utility", "@apply no-such-utility;", "unknown utility"},
		{"apply body", "@apply flex { color: red; }", "must not have a body"},
		{"keyframes", "@keyframes pulse { to { @apply flex; } }", "inside `@keyframes`"},
		{"unknown variant", "@variant no-such-variant { color: red; }", "unknown variant"},
		{"invalid compound", "@variant nth-foo { color: red; }", "Cannot apply variant"},
		{"self cycle", "@apply cycle-a;", "circular dependency"},
		{"mutual cycle", "@apply cycle-b;", "circular dependency"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := ParseCSS(tt.source, "")
			if err != nil {
				t.Fatal(err)
			}
			c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
				return api.MatchUtility(Utility{Name: "cycle", Values: []NamedValue{{"a", "a"}, {"b", "b"}}, Compile: func(value, modifier string) []*Node {
					if value == "b" {
						return []*Node{{Kind: "at-rule", Name: "@apply", Value: "cycle-a"}}
					}
					return nodes
				}})
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = c.Build([]string{"cycle-a"}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("%v", err)
			}
			builder := c.NewBuilder()
			before, err := builder.Build([]string{"flex"})
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				got, err := builder.Build([]string{"block", "cycle-a"})
				if err == nil || !strings.Contains(err.Error(), tt.want) || got != before {
					t.Fatalf("failed build was cached or changed output: %q %v", got, err)
				}
			}
			got, err := builder.Build([]string{"block"})
			if err != nil || !strings.Contains(got, "display: block") || !strings.Contains(got, "display: flex") {
				t.Fatalf("valid retry: %q %v", got, err)
			}
		})
	}
}

func TestDynamicPluginExpansionDepth(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "deep", Bare: legacyBare("integer"), Compile: func(value, modifier string) []*Node {
			n, _ := strconv.Atoi(value)
			return []*Node{{Kind: "at-rule", Name: "@apply", Value: "deep-" + strconv.Itoa(n+1)}}
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Build([]string{"deep-1"}); err == nil || !strings.Contains(err.Error(), "too deeply nested") {
		t.Fatal(err)
	}
}

func TestPluginVariantFormatsErrors(t *testing.T) {
	for _, tt := range []struct{ name, format string }{
		{"bad name", "&:hover"}, {"valid", "&:hover { color: red; } }"},
	} {
		if _, err := NewWithPlugins(Config{}, func(api *PluginAPI) error { return api.AddVariantFormats(tt.name, tt.format) }); err == nil {
			t.Fatalf("accepted %q %q", tt.name, tt.format)
		}
	}
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		api.MatchVariant(Variant{Name: "broken", MatchAll: func(value, modifier string) []string { return []string{"&:hover { color: red; } }"} }})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Build([]string{"broken-[x]:flex"}); err == nil {
		t.Fatal("accepted invalid dynamic variant format")
	}
}
