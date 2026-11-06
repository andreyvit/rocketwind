package rocketwind

import (
	"strings"
	"testing"
)

func TestV3TemplateImportantAndAliases(t *testing.T) {
	c := New(Config{Compatibility: Tailwind3, Aliases: map[string]Alias{"space": {Utility: "p"}}})
	child, err := c.Prepare("space-{size}", TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeLength, nil }})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := c.Prepare("!hover:{child} !space-2 focus:space-4!", TemplateOptions{TypeOf: func(string) (ValueType, error) { return TypeClasses, nil }})
	if err != nil {
		t.Fatal(err)
	}
	css, err := parent.CSS(".card", func(Parameter) (Value, error) {
		return Value{Classes: child, Resolve: func(Parameter) (Value, error) { return Value{CSS: "13px"}, nil }}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".card:hover", "padding: 13px !important;", "padding: 0.5rem !important;", ".card:focus", "padding: 1rem !important;"} {
		if !strings.Contains(css, want) {
			t.Fatalf("missing %q in %s", want, css)
		}
	}
	css, err = c.Build([]string{"!space-2", "-space-4"})
	if err != nil || !strings.Contains(css, "padding: 0.5rem !important;") {
		t.Fatalf("legacy alias: %s / %v", css, err)
	}
}
