package rocketwind

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

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
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			c, err := Compile(tt.CSS, Config{Compatibility: Tailwind3})
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
