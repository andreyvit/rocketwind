package rocketwind

import (
	"strings"
	"testing"
)

func TestExistingClasses(t *testing.T) {
	sheets := []string{
		`/* .comment {} */ .flex, .block:hover { content: ".value"; } @media (min-width: 600px) { .sm\:grid { display: grid; } }`,
		`.scope .group:hover .group-hover\:bg-red-500 { color: red; } .w-1\/2 { width: 50%; } .\32 xl\:flex { display:flex }`,
		`.card { &.active { color: blue; } &:is(.selected, .focused):not(.disabled) { color: red; } }`,
		`[data-class=".attribute"] .item:has(.child) :lang('.language') { content: '.value'; } :where(.one, .two):nth-child(2n of .third) { display:block; }`,
		`.\62 lock, .caf\e9, .a\.b, .bang\! { color:red; } @supports selector(.supports) { .supported {} }`,
	}
	set, err := DefinedClasses(sheets...)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flex", "block", "sm:grid", "scope", "group", "group-hover:bg-red-500", "w-1/2", "2xl:flex", "card", "active", "selected", "focused", "item", "one", "two", "third", "café", "a.b", "bang!", "supported"} {
		if !set.Contains(name) {
			t.Errorf("missing class %q", name)
		}
	}
	for _, name := range []string{"comment", "value", "disabled", "attribute", "child", "language", "supports"} {
		if set.Contains(name) {
			t.Errorf("not a definition: %q", name)
		}
	}
	if len(set) != 20 {
		t.Fatalf("unexpected definitions: %v", set)
	}
}

func TestExistingClassesBuildAndApply(t *testing.T) {
	existing, err := DefinedClasses(`.flex { display:flex; } .hover\:block:hover {display:block;} .bg-red-500 {background-color:red;}`)
	if err != nil {
		t.Fatal(err)
	}
	c := New(Config{ExistingClasses: existing})
	delete(existing, "flex")
	existing["grid"] = struct{}{}
	css, err := c.Build([]string{"flex", "hover:block", "bg-red-500", "grid"})
	if err != nil {
		t.Fatal(err)
	}
	if css != ".grid {\n  display: grid;\n}\n" {
		t.Fatal(css)
	}
	c, err = Compile(`@tailwind utilities; .card { @apply grid; }`, Config{ExistingClasses: existing})
	if err != nil {
		t.Fatal(err)
	}
	css, err = c.Build([]string{"grid", "block"})
	if err != nil || !strings.Contains(css, ".card {\n  display: grid;") || !strings.Contains(css, ".block {") || strings.Contains(css, ".grid {") {
		t.Fatalf("%s: %v", css, err)
	}
	template, err := c.Prepare("grid", TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	css, err = template.CSS(".named", nil)
	if err != nil || !strings.Contains(css, "display: grid;") {
		t.Fatalf("%s: %v", css, err)
	}
}

func TestExistingClassesEscapesAndEmpty(t *testing.T) {
	set, err := DefinedClasses("", `.\000066lex, .\1F680, .x\31 0, .foo\'bar, .foo\\bar, .cafè, ::slotted(.slot), :-webkit-any(.webkit), :-moz-any(.moz) {}`, ".\\62\r\nlock {}")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"flex", "🚀", "x10", "foo'bar", `foo\bar`, "cafè", "slot", "webkit", "moz", "block"} {
		if !set.Contains(name) {
			t.Errorf("missing %q: %v", name, set)
		}
	}
	if _, err = DefinedClasses(`.broken {`); err == nil || !strings.Contains(err.Error(), "existing stylesheet 1") {
		t.Fatal(err)
	}
	if ClassSet(nil).Contains("flex") {
		t.Fatal("nil set contains flex")
	}
}

func TestSelectorClassExtractionIgnoresNonSelectors(t *testing.T) {
	set := ClassSet{}
	for _, input := range []string{`.one/* .comment */.two`, `.three / .four`, `.five/* unfinished`, `.`, `.escaped\`, `:is(.six, :has(.ignored))`, `:is(`, ".\\31\r\n2", `:not(:is(.negative))`, `.seven[data-x="].fake["]`, `:x('(.hidden)')`, `.eight:where(.nine`, `.\000061a`, `.\1 b`, `:is([data-x='a\'b'] .ten)`, `.\"quoted`, `.eleven\31
2`, `\:not .twelve`} {
		collectSelectorClasses(input, set)
	}
	for _, name := range []string{"comment", "unfinished", "ignored", "negative", "fake", "hidden"} {
		if set.Contains(name) {
			t.Errorf("false definition: %s", name)
		}
	}
	for _, name := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "twelve"} {
		if !set.Contains(name) {
			t.Errorf("missing %s", name)
		}
	}
	// Helpers can consume a prefix without corrupting the remaining selector.
	if end := cssBracketEnd("[x='unfinished", 0); end != len("[x='unfinished") {
		t.Fatal(end)
	}
}
