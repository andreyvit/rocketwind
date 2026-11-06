package rocketwind

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func templateTypes(types map[string]ValueType) func(string) (ValueType, error) {
	return func(expression string) (ValueType, error) {
		if value, ok := types[expression]; ok {
			return value, nil
		}
		return "", fmt.Errorf("unknown setting %s", expression)
	}
}

func TestTypedTemplate(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	template, err := c.Prepare("text-{size} text-{color} font-{weight} font-{family} bg-{image} p-{padding}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{
		"size": TypeLength, "color": TypeColor, "weight": TypeNumber, "family": TypeFamilyName, "image": TypeImage, "padding": TypeLength,
	})})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	css, err := template.CSS(".card", func(p Parameter) (Value, error) { calls++; return Value{CSS: "var(--" + p.Expression + ")"}, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".card {", "font-size: var(--size);", "color: var(--color);", "font-weight: var(--weight);", "font-family: var(--family);", "background-image: var(--image);", "padding: var(--padding);"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q:\n%s", want, css)
		}
	}
	if calls != 6 {
		t.Fatalf("resolved %d parameters", calls)
	}
}

func TestTemplateReferences(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	child, err := c.Prepare("text-{color} p-2", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"color": TypeColor})})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := c.Prepare("p-1 hover:{button}! p-4 text-{color}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"button": TypeClasses, "color": TypeColor})})
	if err != nil {
		t.Fatal(err)
	}
	var childCalls, parentCalls int
	css, err := parent.CSS(".signup", func(p Parameter) (Value, error) {
		parentCalls++
		if p.Expression == "button" {
			return Value{Classes: child, Resolve: func(Parameter) (Value, error) { childCalls++; return Value{CSS: "red"}, nil }}, nil
		}
		return Value{CSS: "blue"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"&:hover {", "color: red !important;", "padding: calc(0.25rem * 2) !important;", "color: blue;"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q:\n%s", want, css)
		}
	}
	if childCalls != 1 || parentCalls != 2 {
		t.Fatalf("scope resolutions: child=%d parent=%d", childCalls, parentCalls)
	}
	if strings.Index(css, "padding: 0.25rem") > strings.Index(css, "padding: calc(0.25rem * 4)") {
		t.Fatal("declaration order changed")
	}
}

func TestTemplateExpressions(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	template, err := c.Prepare("w-[calc(100%_-_{gap})] text-{size}/{leading} min-{breakpoint}:bg-{color} [--label:'hello_{name}'] content-['literal_{braces}']", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{
		"gap": TypeLength, "size": TypeLength, "leading": TypeNumber, "breakpoint": TypeLength, "color": TypeColor, "name": TypeAny, "braces": TypeAny,
	})})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"gap": "var(--gap)", "size": "20px", "leading": "1.5", "breakpoint": "800px", "color": "rgb(10 20 30)", "name": "my_name", "braces": "a_b"}
	css, err := template.CSS(".card", func(p Parameter) (Value, error) { return Value{CSS: values[p.Expression]}, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"width: calc(100% - var(--gap));", "font-size: 20px;", "line-height: 1.5;", "@media (width >= 800px)", "background-color: rgb(10 20 30);", "--label: 'hello my_name';", "--tw-content: 'literal a_b';"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q:\n%s", want, css)
		}
	}
}

func TestTemplateOmitAndNamedTokens(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	template, err := c.Prepare("text-{size} bg-{color} w-[calc(1px_+_{missing})] hover:{empty} p-1", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"size": TypeLength, "color": TypeColor, "missing": TypeLength, "empty": TypeClasses})})
	if err != nil {
		t.Fatal(err)
	}
	css, err := template.CSS(".x", func(p Parameter) (Value, error) {
		switch p.Expression {
		case "size":
			return Value{CSS: "xl", Named: true}, nil
		case "color":
			return Value{CSS: "red-500", Named: true}, nil
		default:
			return Value{Omit: true}, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"font-size: 1.25rem;", "background-color: oklch(63.7% 0.237 25.331);", "padding: 0.25rem;"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q:\n%s", want, css)
		}
	}
	if strings.Contains(css, "width:") || strings.Contains(css, "hover") {
		t.Fatal(css)
	}
}

func TestTemplateRepeatedParametersAndConcurrency(t *testing.T) {
	c := New(Config{InlineTheme: true})
	typeCalls := 0
	template, err := c.Prepare("p-{gap} m-{gap} gap-{gap}", TemplateOptions{TypeOf: func(string) (ValueType, error) { typeCalls++; return TypeLength, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if typeCalls != 1 {
		t.Fatalf("type lookup calls: %d", typeCalls)
	}
	parameters := template.Parameters()
	parameters[0].Expression = "changed"
	if template.Parameters()[0].Expression != "gap" {
		t.Fatal("parameters are mutable")
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			calls := 0
			css, err := template.CSS(".x", func(Parameter) (Value, error) { calls++; return Value{CSS: fmt.Sprintf("%dpx", i)}, nil })
			if err != nil || calls != 1 || strings.Count(css, fmt.Sprintf(": %dpx;", i)) != 3 {
				t.Errorf("calls=%d err=%v css=%s", calls, err, css)
			}
		}(i)
	}
	wg.Wait()
}

func TestTemplateAliases(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll, Aliases: map[string]Alias{
		"color": {Utility: "text", Type: TypeColor}, "family": {Utility: "font", Type: TypeFamilyName}, "textalign": {Utility: "text"}, "font": {Utility: "font", Type: TypeNumber},
	}})
	template, err := c.Prepare("color-{color} family-{family} textalign-center !hover:font-{weight}", TemplateOptions{DollarReferences: true, TypeOf: templateTypes(map[string]ValueType{"color": TypeColor, "family": TypeFamilyName, "weight": TypeNumber})})
	if err != nil {
		t.Fatal(err)
	}
	css, err := template.CSS(".x", func(p Parameter) (Value, error) { return Value{CSS: "var(--" + p.Expression + ")"}, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"color: var(--color)", "font-family: var(--family)", "text-align: center", "font-weight: var(--weight) !important"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q:\n%s", want, css)
		}
	}
	helper, err := c.Build([]string{"color-red-500", "family-mono", "textalign-center", "font-bold"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(helper, ".color-red-500") || !strings.Contains(helper, "font-family:") || !strings.Contains(helper, "font-weight: 700") {
		t.Fatal(helper)
	}
}

func TestTemplateErrors(t *testing.T) {
	c := New(Config{})
	for _, tc := range []struct {
		input   string
		options TemplateOptions
		want    string
	}{
		{"text-{size}", TemplateOptions{}, "type lookup"},
		{"text-{}", TemplateOptions{}, "empty expression"},
		{"text-{size", TemplateOptions{}, "unclosed expression"},
		{"text-{size}", TemplateOptions{TypeOf: templateTypes(nil)}, "unknown setting size"},
		{"text-{size}", TemplateOptions{TypeOf: func(string) (ValueType, error) { return "", nil }}, "missing type"},
		{"nonsense", TemplateOptions{}, "unknown utility"},
		{"{color}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"color": TypeColor})}, "class list"},
		{"text-{classes}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"classes": TypeClasses})}, "CSS value"},
	} {
		_, err := c.Prepare(tc.input, tc.options)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: want %q, got %v", tc.input, tc.want, err)
		}
	}
	template, err := c.Prepare("text-{color}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"color": TypeColor})})
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("setting unavailable")
	_, err = template.CSS(".x", func(Parameter) (Value, error) { return Value{}, boom })
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err = template.CSS(".x", nil); err == nil {
		t.Fatal("missing resolver accepted")
	}
	if _, err = template.CSS("", nil); err == nil {
		t.Fatal("empty selector accepted")
	}
	cyclic, err := c.Prepare("{loop}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"loop": TypeClasses})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cyclic.CSS(".x", func(Parameter) (Value, error) { return Value{Classes: cyclic}, nil })
	if err == nil || !strings.Contains(err.Error(), "circular") {
		t.Fatal(err)
	}
}

func TestTemplateDynamicReferenceVariantAndDollarSyntax(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	types := TemplateOptions{DollarReferences: true, TypeOf: templateTypes(map[string]ValueType{"part": TypeClasses, "screen": TypeLength, "color": TypeColor})}
	child, err := c.Prepare("bg-$color", types)
	if err != nil {
		t.Fatal(err)
	}
	template, err := c.Prepare("min-{screen}:$part", types)
	if err != nil {
		t.Fatal(err)
	}
	css, err := template.CSS(".x", func(p Parameter) (Value, error) {
		switch p.Expression {
		case "part":
			return Value{Classes: child}, nil
		case "screen":
			return Value{CSS: "900px"}, nil
		default:
			return Value{CSS: "var(--brand)"}, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(css, "@media (width >= 900px)") || !strings.Contains(css, "background-color: var(--brand)") {
		t.Fatal(css)
	}
}

func TestTemplateRenderMultipleAndCompiledTheme(t *testing.T) {
	c, err := Compile(`@theme { --color-brand: red; } @tailwind utilities; .unrelated { color: green; }`, Config{})
	if err != nil {
		t.Fatal(err)
	}
	template, err := c.Prepare("bg-brand", TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Render([]Rule{{".one", template, nil}, {".two", template, nil}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(css, "--color-brand: red;") != 1 || !strings.Contains(css, ".one") || !strings.Contains(css, ".two") || strings.Contains(css, "unrelated") {
		t.Fatal(css)
	}
	if _, err = c.Render([]Rule{{Selector: ".none"}}); err == nil {
		t.Fatal("nil template accepted")
	}
	other := New(Config{})
	if _, err = other.Render([]Rule{{".wrong", template, nil}}); err == nil {
		t.Fatal("foreign template accepted")
	}
}

func TestTemplateResolutionFailuresAndOmissions(t *testing.T) {
	c := New(Config{DisablePolyfills: PolyfillAll})
	child, _ := c.Prepare("flex", TemplateOptions{})
	foreign, _ := New(Config{}).Prepare("flex", TemplateOptions{})
	types := TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"x": TypeLength, "list": TypeClasses})}
	boom := errors.New("failed")
	for _, tc := range []struct {
		input   string
		resolve ResolveFunc
		want    string
	}{
		{"{list}", func(Parameter) (Value, error) { return Value{}, nil }, "class list"},
		{"{list}", func(Parameter) (Value, error) { return Value{}, boom }, "failed"},
		{"{list}", func(Parameter) (Value, error) { return Value{Classes: foreign}, nil }, "different compiler"},
		{"p-{x}", func(Parameter) (Value, error) { return Value{}, nil }, "CSS value"},
		{"p-{x}", func(Parameter) (Value, error) { return Value{Classes: child}, nil }, "CSS value"},
		{"p-[calc(1px_+_{x})]", func(Parameter) (Value, error) { return Value{CSS: "4", Named: true}, nil }, "whole value"},
		{"text-xl/{x}", func(Parameter) (Value, error) { return Value{}, boom }, "failed"},
		{"min-{x}:flex", func(Parameter) (Value, error) { return Value{}, boom }, "failed"},
		{"group-hover/{x}:flex", func(Parameter) (Value, error) { return Value{}, boom }, "failed"},
		{"[&_{x}]:flex", func(Parameter) (Value, error) { return Value{}, boom }, "failed"},
		{"text-{x}", func(Parameter) (Value, error) { return Value{CSS: "missing", Named: true}, nil }, "invalid value"},
		{"min-{x}:{list}", func(p Parameter) (Value, error) {
			if p.Type == TypeClasses {
				return Value{Classes: child}, nil
			}
			return Value{}, boom
		}, "failed"},
		{"min-{x}:{list}", func(p Parameter) (Value, error) {
			if p.Type == TypeClasses {
				return Value{Classes: child}, nil
			}
			return Value{CSS: "var(--breakpoint)"}, nil
		}, "invalid variant"},
		{"min-{x}:flex", func(Parameter) (Value, error) { return Value{CSS: "var(--breakpoint)"}, nil }, "invalid variant"},
	} {
		template, err := c.Prepare(tc.input, types)
		if err != nil {
			t.Fatalf("%s: %v", tc.input, err)
		}
		_, err = template.CSS(".x", tc.resolve)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want %q, got %v", tc.input, tc.want, err)
		}
	}
	for _, input := range []string{"text-xl/{x}", "min-{x}:flex", "group-hover/{x}:flex", "[&_{x}]:flex", "group-aria-{x}:flex", "min-{x}:{list}"} {
		template, err := c.Prepare(input, types)
		if err != nil {
			t.Fatal(err)
		}
		css, err := template.CSS(".x", func(p Parameter) (Value, error) {
			if p.Type == TypeClasses {
				return Value{Classes: child}, nil
			}
			return Value{Omit: true}, nil
		})
		if err != nil || css != "" {
			t.Errorf("%s: css=%s err=%v", input, css, err)
		}
	}
}

func TestTemplateNestedClassLimit(t *testing.T) {
	c := New(Config{})
	types := TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"next": TypeClasses})}
	var templates []*Template
	for i := 0; i < 101; i++ {
		template, err := c.Prepare("{next}", types)
		if err != nil {
			t.Fatal(err)
		}
		templates = append(templates, template)
	}
	index := 0
	_, err := templates[0].CSS(".x", func(Parameter) (Value, error) { index++; return Value{Classes: templates[index]}, nil })
	if err == nil || !strings.Contains(err.Error(), "100 levels") {
		t.Fatal(err)
	}
}

func TestTemplateTokenizationAndEscapes(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	template, err := c.Prepare("\n flex\tcontent-['a b']\r p-[var(--gap)]\f m-{a{b}} w-{a\\}b}\n [--literal:'RWparam'] content-['\\{'] ", TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeLength, nil }})
	if err != nil {
		t.Fatal(err)
	}
	css, err := template.CSS(".x", func(Parameter) (Value, error) { return Value{CSS: "1px"}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(css, "--literal: 'RWparam'") || !strings.Contains(css, "margin: 1px") || !strings.Contains(css, "width: 1px") {
		t.Fatal(css)
	}
	for range classWords("one two three") {
		break
	}
	for range classWords("'one\\'two' \\ three") {
	}
	if _, err = c.Prepare("{unclosed", TemplateOptions{}); err == nil {
		t.Fatal("unclosed reference accepted")
	}
	if _, err = c.Prepare("{unknown}", TemplateOptions{}); err == nil {
		t.Fatal("untyped reference accepted")
	}
	if _, err = c.Prepare("hover:{known}extra", TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeAny, nil }}); err == nil {
		t.Fatal("invalid reference accepted")
	}
}

func TestTemplateNamedModifier(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll})
	template, err := c.Prepare("text-xl/6 text-xl/{leading}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"leading": TypeNumber})})
	if err != nil {
		t.Fatal(err)
	}
	css, err := template.CSS(".x", func(Parameter) (Value, error) { return Value{CSS: "none", Named: true}, nil })
	if err != nil || !strings.Contains(css, "line-height: 1;") {
		t.Fatalf("%s: %v", css, err)
	}
}

func TestAliasBoundariesAndTypes(t *testing.T) {
	c := New(Config{InlineTheme: true, DisablePolyfills: PolyfillAll, Aliases: map[string]Alias{
		"space": {Utility: "m"}, "color": {Utility: "text", Type: TypeColor}, "size": {Utility: "text", Type: TypeLength},
		"border-color": {Utility: "border", Type: TypeColor}, "border-size": {Utility: "border", Type: TypeLength},
	}, Theme: map[string]string{"--text-dual": "12px", "--color-dual": "red", "--border-width-dual": "3px"}})
	css, err := c.Build([]string{"-space-4!", "!space-2", "flex", "color-dual", "size-dual", "border-color-dual", "border-size-dual"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"margin: calc(0.25rem * -4) !important", "margin: calc(0.25rem * 2) !important", "font-size: 12px", "color: red", "border-color: red", "border-width: 3px", "display: flex"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %s:\n%s", want, css)
		}
	}
	for range c.Parse("color-dual") {
		break
	}
}
