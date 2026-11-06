package rocketwind

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSourceVariableReferences(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/references.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, CSS string
		Steps     [][]string
		Results   []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 11 {
		t.Fatalf("got %d cases, want 11", len(cases))
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			c, err := Compile(tt.CSS, Config{})
			if err != nil {
				t.Fatal(err)
			}
			builder := c.NewBuilder()
			var all []string
			for i, step := range tt.Steps {
				all = append(all, step...)
				want := strings.TrimPrefix(tt.Results[i], "/*! tailwindcss v4.3.4 | MIT License | https://tailwindcss.com */\n")
				for _, build := range []func() (string, error){func() (string, error) { return builder.Build(step) }, func() (string, error) { return c.Build(all) }} {
					got, err := build()
					if err != nil || strings.TrimSpace(got) != strings.TrimSpace(want) {
						t.Fatalf("step %d: %v\ngot:\n%s\nwant:\n%s", i, err, got, want)
					}
				}
			}
			builder.Reset()
			want, err := c.Build(nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := builder.Build(nil)
			if err != nil || got != want {
				t.Fatalf("reset: %v\n%s", err, got)
			}
		})
	}
}

func TestSourceVariableReferencesProgrammatic(t *testing.T) {
	c := New(Config{Theme: map[string]string{"--color-brand": "red"}, ExistingClasses: ClassSet{"flex": {}}})
	want := ":root, :host {\n  --color-brand: red;\n}\n"
	for _, build := range []func([]string) (string, error){c.Build, c.NewBuilder().Build} {
		css, err := build(Candidates(`<div class="flex" style="color:var(--color-brand)"></div>`))
		if err != nil || css != want {
			t.Fatalf("%v\n%s", err, css)
		}
	}
	css, err := c.Build(nil)
	if err != nil || css != "" {
		t.Fatalf("independent build: %v\n%s", err, css)
	}
}
