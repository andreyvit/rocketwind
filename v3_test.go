package rocketwind

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestTailwind3Completions(t *testing.T) {
	f, err := os.Open("testdata/v3/completions.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var cases []struct {
		Config  ConfigObject
		Classes []json.RawMessage
	}
	if err := json.NewDecoder(z).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		c, err := NewWithPlugins(Config{Compatibility: Tailwind3}, Legacy(tt.Config))
		if err != nil {
			t.Fatal(err)
		}
		got := c.ClassList()
		if len(got) != len(tt.Classes) {
			t.Fatalf("class count %d != %d", len(got), len(tt.Classes))
		}
		for i, raw := range tt.Classes {
			var want ClassSuggestion
			if raw[0] == '[' {
				var parts []json.RawMessage
				if err := json.Unmarshal(raw, &parts); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(parts[0], &want.Name); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(parts[1], &want); err != nil {
					t.Fatal(err)
				}
			} else if err := json.Unmarshal(raw, &want.Name); err != nil {
				t.Fatal(err)
			}
			if got[i].Name != want.Name || !slices.Equal(got[i].Modifiers, want.Modifiers) {
				t.Fatalf("class %d: got %+v, want %+v", i, got[i], want)
			}
		}
	}
}

func TestTailwind3StandardPlugins(t *testing.T) {
	data, err := os.ReadFile("testdata/v3/plugins.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Name, Prefix, Candidate, Result string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	compilers := map[string]*Compiler{}
	var failures []map[string]any
	for _, tt := range cases {
		key := tt.Name + tt.Prefix
		c := compilers[key]
		if c == nil {
			c, err = Compile(`@plugin "@tailwindcss/`+tt.Name+`"; @tailwind components; @tailwind utilities;`, Config{Compatibility: Tailwind3, Prefix: tt.Prefix})
			if err != nil {
				t.Fatal(err)
			}
			compilers[key] = c
		}
		got, err := c.Build([]string{tt.Candidate})
		if err != nil {
			t.Fatal(err)
		}
		if normalizeV3CSS(t, got) != normalizeV3CSS(t, tt.Result) {
			failures = append(failures, map[string]any{"name": tt.Name, "candidate": tt.Candidate, "got": got, "want": tt.Result})
		}
	}
	if len(failures) > 0 {
		data, _ := json.MarshalIndent(failures, "", "  ")
		os.MkdirAll(".cache", 0755)
		os.WriteFile(".cache/v3-plugin-failures.json", data, 0644)
		t.Fatalf("%d/%d failures: .cache/v3-plugin-failures.json", len(failures), len(cases))
	}
}

func TestTailwind3DefaultUtilities(t *testing.T) {
	checkV3Utilities(t, "utilities", 11344)
}

func TestTailwind3ArbitraryUtilities(t *testing.T) {
	checkV3Utilities(t, "arbitrary", 2759)
}

func checkV3Utilities(t *testing.T, name string, count int) {
	t.Helper()
	f, err := os.Open("testdata/v3/" + name + ".json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var cases []struct{ Candidate, CSS string }
	if err := json.NewDecoder(z).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if count >= 0 && len(cases) != count {
		t.Fatalf("got %d cases", len(cases))
	}
	c := New(Config{Compatibility: Tailwind3})
	failed := 0
	for _, tt := range cases {
		got, err := c.Build([]string{tt.Candidate})
		if err != nil {
			t.Fatal(err)
		}
		if normalizeV3CSS(t, got) != normalizeV3CSS(t, tt.CSS) {
			if failed < 15 {
				t.Errorf("%s:\ngot %s\nwant %s", tt.Candidate, got, tt.CSS)
			}
			failed++
		}
	}
	if failed > 0 {
		t.Fatalf("%d/%d mismatches", failed, len(cases))
	}
}

func TestTailwind3Stylesheets(t *testing.T) {
	data, err := os.ReadFile("testdata/v3/stylesheets.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, CSS, Result, Error string
		Classes                  []string
		Config                   ConfigObject
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			c, err := Compile("@config \"test\";"+tt.CSS, Config{Compatibility: Tailwind3, LoadModule: func(_, _, _ string) (Module, error) { return Module{Legacy: tt.Config}, nil }})
			if tt.Error != "" {
				if err == nil || !strings.Contains(err.Error(), tt.Error) {
					t.Fatalf("got %v, want %s", err, tt.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Build(tt.Classes)
			if err != nil {
				t.Fatal(err)
			}
			if g, w := normalizeV3CSS(t, got), normalizeV3CSS(t, tt.Result); g != w {
				i := 0
				for i < min(len(g), len(w)) && g[i] == w[i] {
					i++
				}
				t.Errorf("at %d (lengths %d/%d):\ngot  %s\nwant %s", i, len(g), len(w), g[max(0, i-40):min(len(g), i+250)], w[max(0, i-40):min(len(w), i+250)])
			}
		})
	}
}

func normalizeV3CSS(t *testing.T, input string) string {
	t.Helper()
	nodes, err := ParseCSS(input, "")
	if err != nil {
		t.Fatal(err)
	}
	w := optimizedCSSWriter{compact: true}
	w.write(flattenRulesMode(nodes, true), 0)
	return strings.TrimSpace(w.out.String())
}

func TestTailwind3Variants(t *testing.T) {
	data, err := os.ReadFile("testdata/v3/variants.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Candidate, CSS string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	c := New(Config{Compatibility: Tailwind3})
	failed := 0
	for _, tt := range cases {
		got, err := c.Build([]string{tt.Candidate})
		if err != nil {
			t.Fatal(err)
		}
		if normalizeV3CSS(t, got) != normalizeV3CSS(t, tt.CSS) {
			if failed < 20 {
				t.Errorf("%s:\ngot %s\nwant %s", tt.Candidate, got, tt.CSS)
			}
			failed++
		}
	}
	if failed > 0 {
		t.Fatalf("%d/%d mismatches", failed, len(cases))
	}
}

func TestTailwind3Configuration(t *testing.T) {
	data, err := os.ReadFile("testdata/v3/configuration.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name             string
		Config           ConfigObject
		Classes, Results []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			c, err := NewWithPlugins(Config{Compatibility: Tailwind3}, Legacy(tt.Config))
			if err != nil {
				t.Fatal(err)
			}
			for i, class := range tt.Classes {
				got, err := c.Build([]string{class})
				if err != nil {
					t.Fatal(err)
				}
				if normalizeV3CSS(t, got) != normalizeV3CSS(t, tt.Results[i]) {
					t.Errorf("%s:\ngot %s\nwant %s", class, got, tt.Results[i])
				}
			}
		})
	}
}
