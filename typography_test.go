package rocketwind

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTypographyAgainstUpstream(t *testing.T) {
	data, err := os.ReadFile("testdata/plugins/typography/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, CSS, Result string
		Candidates        []string
		Options           TypographyOptions
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 20 {
		t.Fatalf("got %d fixtures, want 20", len(cases))
	}
	var failures []map[string]any
	for i, tt := range cases {
		config := Config{Plugins: map[string]Plugin{"@tailwindcss/typography": Typography(tt.Options)}}
		config.LoadModule = func(id, base, kind string) (Module, error) {
			return Module{}, nil
		}
		// Options are passed in Go so these cases also exercise programmatic
		// configuration. The standard module options are checked separately.
		input := tt.CSS
		if start := strings.Index(input, `@plugin "@tailwindcss/typography"{`); start >= 0 {
			end := strings.IndexByte(input[start:], '}') + start
			input = input[:start] + `@plugin "@tailwindcss/typography";` + input[end+1:]
		}
		// The upstream v3 fixtures use class dark mode; use its v4 equivalent.
		if strings.Contains(tt.CSS, "@config") && strings.Contains(strings.Join(tt.Candidates, " "), "dark:") {
			input += " @custom-variant dark (&:is(.dark *));"
		}
		c, err := Compile(input, config)
		got := ""
		if err == nil {
			got, err = c.Build(tt.Candidates)
		}
		tt.Result = strings.TrimPrefix(tt.Result, "/*! tailwindcss v4.3.4 | MIT License | https://tailwindcss.com */\n")
		if err != nil || strings.TrimSpace(got) != strings.TrimSpace(tt.Result) {
			failures = append(failures, map[string]any{"index": i, "name": tt.Name, "got": got, "want": tt.Result, "error": err})
		}
	}
	if len(failures) > 0 {
		data, _ := json.MarshalIndent(failures, "", "  ")
		os.MkdirAll(".cache", 0755)
		os.WriteFile(".cache/typography-failures.json", data, 0644)
		t.Fatalf("%d/%d typography fixtures differ; .cache/typography-failures.json", len(failures), len(cases))
	}
}

func TestTypographyTrailingPseudos(t *testing.T) {
	data, err := os.ReadFile("testdata/plugins/typography/selectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input  string
		Result [2]string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		pseudo, selector := typographyTrailingPseudos(tt.Input)
		if pseudo != tt.Result[0] || selector != tt.Result[1] {
			t.Errorf("%s: got [%q, %q], want %q", tt.Input, pseudo, selector, tt.Result)
		}
	}
}

func TestTypographyConfiguration(t *testing.T) {
	for _, input := range []string{
		`@plugin "@tailwindcss/typography"; @tailwind utilities;`,
		`@plugin "@tailwindcss/typography" { className: "markdown"; target: "legacy"; } @tailwind utilities;`,
	} {
		c, err := Compile(input, Config{})
		if err != nil {
			t.Fatal(err)
		}
		class := "prose"
		if strings.Contains(input, "markdown") {
			class = "markdown"
		}
		css, err := c.Build([]string{class, class + "-headings:italic"})
		if err != nil || !strings.Contains(css, "."+class+" {") || !strings.Contains(css, "font-style: italic") {
			t.Fatalf("%s %v", css, err)
		}
	}
	if _, err := NewWithPlugins(Config{}, Typography(TypographyOptions{ClassName: "bad.name"})); err == nil {
		t.Fatal("expected invalid variant name")
	}
	base, err := ParseCSS("color: red; text-align: left; text-align: start; p { color: blue; }", "")
	if err != nil {
		t.Fatal(err)
	}
	extra, err := ParseCSS("color: green; text-align: center; text-align: match-parent; p { color: purple; font-weight: 600; }", "")
	if err != nil {
		t.Fatal(err)
	}
	options := TypographyOptions{
		Styles: []TypographyStyle{{Name: "DEFAULT", Nodes: base}},
		Extend: []TypographyStyle{{Name: "DEFAULT", Nodes: extra}, {Name: "tiny", Nodes: declarations("font-size", "10px")}},
	}
	c, err := NewWithPlugins(Config{}, Typography(options))
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"prose", "prose-tiny", "prose-xl"})
	if err != nil {
		t.Fatal(err)
	}
	want := ".prose {\n  color: green;\n  text-align: center;\n  text-align: match-parent;\n  :where(p):not(:where([class~=\"not-prose\"], [class~=\"not-prose\"] *)) {\n    color: purple;\n    font-weight: 600;\n  }\n}\n.prose-tiny {\n  font-size: 10px;\n}\n"
	if css != want {
		t.Fatalf("got:\n%s\nwant:\n%s", css, want)
	}
	if base[0].Value != "red" || base[3].Nodes[0].Value != "blue" {
		t.Fatal("plugin modified caller styles")
	}
}
