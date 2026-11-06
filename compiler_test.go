package rocketwind

import (
	"strings"
	"testing"
)

func TestProgrammaticUtilities(t *testing.T) {
	c := New(Config{InlineTheme: true, Theme: map[string]string{"--color-brand": "#663399"}})
	css, err := c.Build([]string{"grid", "p-4", "w-1/2", "text-xl", "bg-brand", "grid-cols-3", "-mt-2", "unknown-class"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".grid {\n  display: grid;\n}", "padding: calc(0.25rem * 4);", "width: calc(1 / 2 * 100%);", "font-size: 1.25rem;", "background-color: #663399;", "grid-template-columns: repeat(3, minmax(0, 1fr));", "margin-top: calc(0.25rem * -2);"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q in:\n%s", want, css)
		}
	}
	if strings.Contains(css, "unknown") {
		t.Fatal(css)
	}
	again, err := c.Build([]string{"grid", "p-4", "w-1/2", "text-xl", "bg-brand", "grid-cols-3", "-mt-2"})
	if err != nil || css != again {
		t.Fatalf("nondeterministic output: %v", err)
	}
}
