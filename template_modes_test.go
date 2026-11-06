package rocketwind

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestTemplateValueModes(t *testing.T) {
	for _, version := range []Compatibility{Tailwind3, Tailwind4} {
		c := New(Config{Compatibility: version, InlineTheme: true, DisablePolyfills: PolyfillAll})
		for _, tc := range []struct {
			name, source, value, want string
			kind                      ValueType
			mode                      ValueMode
		}{
			{"auto token", "text-{x}", "xl", "text-xl", TypeLength, ValueModeAuto},
			{"auto literal", "text-{x}", "19px", "text-[19px]", TypeLength, ValueModeAuto},
			{"auto bare", "opacity-{x}", "1", "opacity-1", TypeNumber, ValueModeAuto},
			{"literal number", "opacity-{x}", "1", "opacity-[1]", TypeNumber, ValueModeLiteral},
			{"decimal literal", "opacity-{x}", "1.0", "opacity-[1.0]", TypeNumber, ValueModeAuto},
			{"percentage literal", "opacity-{x}", "100%", "opacity-[100%]", TypeNumber, ValueModeAuto},
			{"explicit brackets", "opacity-[{x}]", "1", "opacity-[1]", TypeNumber, ValueModeAuto},
			{"typed brackets", "text-[length:{x}]", "xl", "text-[length:xl]", TypeLength, ValueModeAuto},
			{"bracketed variable", "text-[{x}]", "var(--local)", "text-[length:var(--local)]", TypeLength, ValueModeAuto},
			{"strict token", "text-{x}", "xl", "text-xl", TypeLength, ValueModeToken},
			{"strict token in brackets", "text-[length:{x}]", "xl", "text-xl", TypeLength, ValueModeToken},
			{"literal token spelling", "text-{x}", "xl", "text-[length:xl]", TypeLength, ValueModeLiteral},
			{"auto color", "text-{x}", "red-500", "text-red-500", TypeColor, ValueModeAuto},
			{"literal color", "text-{x}", "red", "text-[red]", TypeColor, ValueModeAuto},
			{"negative token", "-m-{x}", "1", "-m-1", TypeLength, ValueModeAuto},
			{"negative literal", "-m-{x}", "7px", "-m-[7px]", TypeLength, ValueModeAuto},
			{"fraction token", "w-{x}", "1/2", "w-1/2", TypeLength, ValueModeAuto},
			{"strict fraction token", "w-{x}", "1/2", "w-1/2", TypeLength, ValueModeToken},
			{"literal slash", "content-{x}", "a/b", "content-[a/b]", TypeAny, ValueModeAuto},
			{"fraction literal", "w-[{x}]", "1/2", "w-[1/2]", TypeLength, ValueModeAuto},
			{"named modifier", "text-xl/{x}", "tight", "text-xl/tight", TypeNumber, ValueModeAuto},
			{"literal modifier", "text-xl/{x}", "1.7", "text-xl/[1.7]", TypeNumber, ValueModeAuto},
			{"bracket modifier", "text-xl/[{x}]", "1", "text-xl/[1]", TypeNumber, ValueModeAuto},
			{"strict modifier", "text-xl/{x}", "tight", "text-xl/tight", TypeNumber, ValueModeToken},
			{"expression", "w-[calc(100%_-_{x})]", "1", "w-[calc(100%_-_1)]", TypeLength, ValueModeAuto},
			{"whitespace value", "[--empty:{x}]", " ", "[--empty:RWSPACE]", TypeAny, ValueModeAuto},
			{"empty string content", "content-[{x}]", `""`, `content-[""]`, TypeAny, ValueModeAuto},
		} {
			t.Run(fmt.Sprint(version)+"/"+tc.name, func(t *testing.T) {
				if version == Tailwind3 && tc.name == "auto bare" {
					tc.want = "opacity-[1]" // v3 has no opacity-1 token.
				}
				template, err := c.Prepare(tc.source, TemplateOptions{TypeOf: func(string) (ValueType, error) { return tc.kind, nil }})
				if err != nil {
					t.Fatal(err)
				}
				actual, err := template.CSS(".x", func(Parameter) (Value, error) { return Value{CSS: tc.value, Mode: tc.mode}, nil })
				if err != nil {
					t.Fatal(err)
				}
				wantTemplate, err := c.Prepare(tc.want, TemplateOptions{})
				if err != nil {
					t.Fatal(err)
				}
				want, err := wantTemplate.CSS(".x", nil)
				want = strings.ReplaceAll(want, "RWSPACE", " ")
				if err != nil || actual != want {
					t.Fatalf("got:\n%s\nwant:\n%s\nerror: %v", actual, want, err)
				}
			})
		}
	}
}

func TestTemplateValueAndModifierModes(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	for _, size := range []string{"xl", "19px"} {
		for _, leading := range []string{"tight", "1.7"} {
			t.Run(size+"/"+leading, func(t *testing.T) {
				template, err := c.Prepare("text-{size}/{leading}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"size": TypeLength, "leading": TypeNumber})})
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				css, err := template.CSS(".x", func(p Parameter) (Value, error) {
					calls++
					if p.Expression == "size" {
						return Value{CSS: size}, nil
					}
					return Value{CSS: leading}, nil
				})
				wantSize, wantLeading := "1.25rem", "1.25"
				if size == "19px" {
					wantSize = size
				}
				if leading == "1.7" {
					wantLeading = leading
				}
				if err != nil || calls != 2 || !strings.Contains(css, "font-size: "+wantSize+";") || !strings.Contains(css, "line-height: "+wantLeading+";") {
					t.Fatalf("%s\nerror: %v, resolver calls: %d", css, err, calls)
				}
			})
		}
	}
}

func TestTemplateValueModeErrors(t *testing.T) {
	c := New(Config{})
	for _, source := range []string{"text-{x}", "text-xl/{x}", "min-{x}:flex", "group-nth-{x}:flex", "w-[calc(1px_+_{x})]", "[--x:{x}]"} {
		t.Run(source, func(t *testing.T) {
			template, err := c.Prepare(source, TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeLength, nil }})
			if err != nil {
				t.Fatal(err)
			}
			if css, err := template.CSS(".x", func(Parameter) (Value, error) { return Value{CSS: "missing", Mode: ValueModeToken}, nil }); err == nil || css != "" {
				t.Fatalf("strict mode accepted an unknown token: %s, %v", css, err)
			}
		})
	}
	template, _ := c.Prepare("p-{x}", TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeLength, nil }})
	if _, err := template.CSS(".x", func(Parameter) (Value, error) { return Value{CSS: "1", Mode: 99}, nil }); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("invalid mode: %v", err)
	}
}

func TestTemplateEmptyValues(t *testing.T) {
	c := New(Config{DisablePolyfills: PolyfillAll})
	for _, source := range []string{"p-{x}", "p-[{x}]", "w-[calc(1px_+_{x})]", "content-['hello{x}']", "{x}", "hover:{x}"} {
		for _, mode := range []ValueMode{ValueModeAuto, ValueModeToken, ValueModeLiteral} {
			template, err := c.Prepare(source, TemplateOptions{TypeOf: func(string) (ValueType, error) {
				if source == "{x}" || source == "hover:{x}" {
					return TypeClasses, nil
				}
				return TypeAny, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			css, err := template.CSS(".x", func(Parameter) (Value, error) { return Value{Mode: mode}, nil })
			if err != nil || css != "" {
				t.Fatalf("%s mode %d: %s %v", source, mode, css, err)
			}
		}
	}
}

func TestTemplateModePluginError(t *testing.T) {
	want := errors.New("plugin failure")
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "failing", Values: []NamedValue{{"x", "red"}}, CompileWithError: func(string, string) ([]*Node, error) { return nil, want }})
	})
	if err != nil {
		t.Fatal(err)
	}
	template, _ := c.Prepare("failing-{x}", TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeColor, nil }})
	if _, err := template.CSS(".x", func(Parameter) (Value, error) { return Value{CSS: "x"}, nil }); !errors.Is(err, want) {
		t.Fatalf("plugin error lost during fallback: %v", err)
	}
}

func TestTemplateModesInVariants(t *testing.T) {
	c, err := NewWithPlugins(Config{InlineTheme: true, DisablePolyfills: PolyfillAll}, func(api *PluginAPI) error {
		api.MatchVariant(Variant{Name: "state", Values: []NamedValue{{"open", "expanded"}}, Match: func(value, modifier string) string { return "&[data-state=" + value + "]" }})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	child, _ := c.Prepare("flex", TemplateOptions{})
	for _, tc := range []struct {
		source, value, want string
		mode                ValueMode
	}{
		{"min-{x}", "md", "@media (width >= 48rem)", ValueModeAuto},
		{"min-{x}", "713px", "@media (width >= 713px)", ValueModeAuto},
		{"min-[{x}]", "md", "@media (width >= md)", ValueModeAuto},
		{"min-{x}", "md", "@media (width >= md)", ValueModeLiteral},
		{"min-[{x}]", "md", "@media (width >= 48rem)", ValueModeToken},
		{"group-state-{x}", "open", "data-state=expanded", ValueModeAuto},
		{"peer-state-{x}", "closed", "data-state=closed", ValueModeAuto},
		{"state-{x}", "open", "data-state=open", ValueModeLiteral},
		{"state-[{x}]", "open", "data-state=open", ValueModeAuto},
		{"state-[{x}]", "open", "data-state=expanded", ValueModeToken},
	} {
		for _, base := range []string{"flex", "{list}"} {
			t.Run(tc.source+"/"+tc.value+"/"+base+fmt.Sprint(tc.mode), func(t *testing.T) {
				template, err := c.Prepare(tc.source+":"+base, TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"x": TypeAny, "list": TypeClasses})})
				if err != nil {
					t.Fatal(err)
				}
				css, err := template.CSS(".x", func(p Parameter) (Value, error) {
					if p.Type == TypeClasses {
						return Value{Classes: child}, nil
					}
					return Value{CSS: tc.value, Mode: tc.mode}, nil
				})
				if err != nil || !strings.Contains(css, tc.want) {
					t.Fatalf("missing %q: %s %v", tc.want, css, err)
				}
			})
		}
	}
}

func TestTemplateVariantModeErrors(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		api.MatchVariant(Variant{Name: "broken", Values: []NamedValue{{"x", "named"}}, Match: func(value, modifier string) string {
			if value == "named" {
				return "&:hover { color: red; } }"
			}
			return "&:hover"
		}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	child, _ := c.Prepare("flex", TemplateOptions{})
	for _, base := range []string{"flex", "{list}"} {
		template, _ := c.Prepare("broken-{x}:"+base, TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"x": TypeAny, "list": TypeClasses})})
		_, err := template.CSS(".x", func(p Parameter) (Value, error) {
			if p.Type == TypeClasses {
				return Value{Classes: child}, nil
			}
			return Value{CSS: "x"}, nil
		})
		if err == nil || strings.Contains(err.Error(), "invalid variant") {
			t.Fatalf("plugin error lost during fallback: %v", err)
		}
	}
}

func TestTemplateModesShareOneResolution(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	template, err := c.Prepare("p-{x} m-[{x}] w-[calc(100%_-_{x})]", TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeLength, nil }})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	css, err := template.CSS(".x", func(Parameter) (Value, error) { calls++; return Value{CSS: "1"}, nil })
	if err != nil || calls != 1 || css != ".x {\n  padding: 0.25rem;\n  margin: 1;\n  width: calc(100% - 1);\n}\n" {
		t.Fatalf("calls: %d, CSS: %s, error: %v", calls, css, err)
	}
}

func TestTemplateNamedValueTypes(t *testing.T) {
	for _, version := range []Compatibility{Tailwind3, Tailwind4} {
		c, err := NewWithPlugins(Config{Compatibility: version, InlineTheme: true, DisablePolyfills: PolyfillAll}, Legacy(ConfigObject{{"theme", ConfigObject{{"extend", ConfigObject{
			{"colors", ConfigObject{{"dual", "red"}}},
			{"fontSize", ConfigObject{{"dual", "13px"}}},
			{"fontFamily", ConfigObject{{"dual", "ExampleFamily"}}},
			{"fontWeight", ConfigObject{{"dual", "550"}}},
			{"backgroundImage", ConfigObject{{"dual", "url(example.png)"}}},
			{"borderWidth", ConfigObject{{"dual", "3px"}}},
			{"boxShadow", ConfigObject{{"dual", "0 1px 2px #000"}}},
		}}}}}))
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			root, want, unwanted string
			kind                 ValueType
		}{
			{"text", "font-size: 13px;", "color:", TypeLength},
			{"text", "color:", "font-size:", TypeColor},
			{"font", "font-family: ExampleFamily;", "font-weight:", TypeFamilyName},
			{"font", "font-weight: 550;", "font-family:", TypeNumber},
			{"bg", "background-image: url(example.png);", "background-color:", TypeImage},
			{"bg", "background-color:", "background-image:", TypeColor},
			{"border", "border-width: 3px;", "border-color:", TypeLength},
			{"border", "border-color:", "border-width:", TypeColor},
			{"shadow", "--tw-shadow-color:", "--tw-shadow: 0", TypeColor},
			{"shadow", "--tw-shadow: 0", "--tw-shadow-color:", TypeShadow},
		} {
			for _, mode := range []ValueMode{ValueModeAuto, ValueModeToken} {
				t.Run(fmt.Sprint(version)+"/"+tc.root+"/"+string(tc.kind)+fmt.Sprint(mode), func(t *testing.T) {
					template, err := c.Prepare(tc.root+"-{x}", TemplateOptions{TypeOf: func(string) (ValueType, error) { return tc.kind, nil }})
					if err != nil {
						t.Fatal(err)
					}
					css, err := template.CSS(".x", func(Parameter) (Value, error) { return Value{CSS: "dual", Mode: mode}, nil })
					if err != nil || !strings.Contains(css, tc.want) || strings.Contains(css, tc.unwanted) {
						t.Fatalf("wanted %q without %q:\n%s\n%v", tc.want, tc.unwanted, css, err)
					}
				})
			}
		}
	}
}

func TestTemplateTypedPluginModes(t *testing.T) {
	c, err := NewWithPlugins(Config{DisablePolyfills: PolyfillAll}, func(api *PluginAPI) error {
		for _, kind := range []ValueType{TypeColor, TypeLength} {
			property, value := "color", "red"
			if kind == TypeLength {
				property, value = "width", "3px"
			}
			if err := api.MatchUtility(Utility{Name: "choice", Types: []ValueType{kind}, Values: []NamedValue{{"same", value}}, Modifiers: []NamedValue{{"small", "2px"}}, Compile: func(value, modifier string) []*Node {
				nodes := declarations(property, value)
				if modifier != "" {
					nodes = append(nodes, declarations("--modifier", modifier)...)
				}
				return nodes
			}}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	template, _ := c.Prepare("choice-{x}/{m}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"x": TypeLength, "m": TypeLength})})
	for _, value := range []string{"same", "19px"} {
		for _, modifier := range []string{"small", "4px"} {
			css, err := template.CSS(".x", func(p Parameter) (Value, error) {
				if p.Expression == "x" {
					return Value{CSS: value}, nil
				}
				return Value{CSS: modifier}, nil
			})
			wantValue, wantModifier := value, modifier
			if value == "same" {
				wantValue = "3px"
			}
			if modifier == "small" {
				wantModifier = "2px"
			}
			want := ".x {\n  width: " + wantValue + ";\n  --modifier: " + wantModifier + ";\n}\n"
			if err != nil || css != want {
				t.Fatalf("%s/%s: got %s, want %s, error: %v", value, modifier, css, want, err)
			}
		}
	}
}

func TestTemplateTypesDoNotUseUnrelatedThemeTokens(t *testing.T) {
	c := New(Config{DisablePolyfills: PolyfillAll, Theme: map[string]string{
		"--text-other": "13px", "--color-length": "red", "--font-weight-family": "500", "--font-weight": "ExampleFamily", "--background-image-paint": "url(example.png)",
	}})
	template, err := c.Prepare("text-{other} text-{length} font-{family} font-{weight} bg-{paint}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{
		"other": TypeColor, "length": TypeLength, "family": TypeFamilyName, "weight": TypeNumber, "paint": TypeColor,
	})})
	if err != nil {
		t.Fatal(err)
	}
	css, err := template.CSS(".x", func(p Parameter) (Value, error) { return Value{CSS: p.Expression}, nil })
	if err != nil || strings.Contains(css, ":root") || strings.Contains(css, "var(") {
		t.Fatalf("unrelated theme tokens were emitted: %s %v", css, err)
	}
}
