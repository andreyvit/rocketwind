package rocketwind

import (
	"cmp"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStandardPluginsAgainstUpstream(t *testing.T) {
	for _, name := range []string{"aspect-ratio", "line-clamp", "container-queries"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/plugins/" + name + "/fixtures.json")
			if err != nil {
				t.Fatal(err)
			}
			var cases []struct {
				CSS, Result string
				Candidates  []string
				Original    bool
				Values      []NamedValue
			}
			if err = json.Unmarshal(data, &cases); err != nil {
				t.Fatal(err)
			}
			if len(cases) != 10 {
				t.Fatalf("got %d fixtures, want 10", len(cases))
			}
			var failures []map[string]any
			for i, tt := range cases {
				config := Config{}
				if tt.Original {
					var plugin Plugin
					switch name {
					case "aspect-ratio":
						plugin = AspectRatio(AspectRatioOptions{Values: tt.Values})
					case "line-clamp":
						plugin = LineClamp(LineClampOptions{Values: tt.Values})
					case "container-queries":
						plugin = ContainerQueries(ContainerQueriesOptions{Values: tt.Values})
					}
					config.Plugins = map[string]Plugin{"@tailwindcss/" + name: plugin}
					config.LoadModule = func(id, base, kind string) (Module, error) { return Module{}, nil }
				}
				c, err := Compile(tt.CSS, config)
				got := ""
				if err == nil {
					got, err = c.Build(tt.Candidates)
				}
				tt.Result = strings.TrimPrefix(tt.Result, "/*! tailwindcss v4.3.4 | MIT License | https://tailwindcss.com */\n")
				if err != nil || strings.TrimSpace(got) != strings.TrimSpace(tt.Result) {
					failures = append(failures, map[string]any{"index": i, "got": got, "want": tt.Result, "error": err})
				}
			}
			if len(failures) > 0 {
				data, _ := json.MarshalIndent(failures, "", "  ")
				os.MkdirAll(".cache", 0755)
				file := ".cache/" + name + "-failures.json"
				os.WriteFile(file, data, 0644)
				t.Fatalf("%d/%d fixtures differ: %s", len(failures), len(cases), file)
			}
		})
	}
}

func TestStandardPluginsProgrammatic(t *testing.T) {
	c, err := NewWithPlugins(Config{Theme: map[string]string{"--aspect-ignored--nested": "3", "--line-clamp-empty": ""}},
		AspectRatio(AspectRatioOptions{Values: []NamedValue{{"wide", "16"}}}),
		LineClamp(LineClampOptions{Values: []NamedValue{{"many", "13"}}}),
		ContainerQueries(ContainerQueriesOptions{Values: []NamedValue{{"wide", "100rem"}}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"aspect-w-wide", "line-clamp-many", "@wide:block"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--tw-aspect-w: 16", "-webkit-line-clamp: 13", "@container (min-width: 100rem)"} {
		if !strings.Contains(css, want) {
			t.Fatalf("missing %s:\n%s", want, css)
		}
	}
	// Programmatic construction also works with no pre-existing theme map.
	if _, err := NewWithPlugins(Config{}, ContainerQueries(ContainerQueriesOptions{})); err != nil {
		t.Fatal(err)
	}
}

func TestContainerPluginSort(t *testing.T) {
	c, err := NewWithPlugins(Config{NoDefaultTheme: true}, ContainerQueries(ContainerQueriesOptions{Values: []NamedValue{{"DEFAULT", "100px"}, {"small", "20rem"}, {"large", "200px"}}}))
	if err != nil {
		t.Fatal(err)
	}
	// A fresh variant name uses the plugin's comparator. Replacing the core @
	// variant keeps core's existing sort group, matching Tailwind 4.
	v := *c.matchingVariants["@"]
	v.Name = "cq"
	api := &PluginAPI{compiler: c}
	api.MatchVariant(v)
	classes := []string{"cq-large:block", "cq-small:block", "cq-small/container10:block", "cq-small/container2:block", "cq:block", "cq-[15px]:block", "cq-[1]:block"}
	css, err := c.Build(classes)
	if err != nil {
		t.Fatal(err)
	}
	previous := -1
	for _, class := range []string{"cq-[15px]:block", "cq-small/container2:block", "cq-small/container10:block", "cq-small:block", "cq:block", "cq-large:block"} {
		at := strings.Index(css, "."+Escape(class))
		if at <= previous {
			t.Fatalf("wrong order for %s:\n%s", class, css)
		}
		previous = at
	}
	if strings.Contains(css, "."+Escape("cq-[1]:block")) {
		t.Fatal("unitless width was accepted")
	}
	for _, tt := range []struct {
		A, B VariantValue
		Want int
	}{
		{VariantValue{"10px", ""}, VariantValue{"20px", ""}, -1},
		{VariantValue{"20px", ""}, VariantValue{"20px", "a"}, 1},
		{VariantValue{"20px", "a"}, VariantValue{"20px", ""}, -1},
		{VariantValue{"20px", "container2"}, VariantValue{"20px", "container10"}, -1},
	} {
		if got := cmp.Compare(v.Sort(tt.A, tt.B), 0); got != tt.Want {
			t.Fatalf("sort %v %v: %d", tt.A, tt.B, got)
		}
	}
}

func TestFunctionalPluginVariantComposition(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		api.MatchVariant(Variant{Name: "when", Values: []NamedValue{{"grid", "grid"}}, Match: func(value, modifier string) string { return "@supports (display: " + value + ")" }})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"not-when-grid:block"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(css, "@supports not (display: grid)") || !strings.Contains(css, "display: block") {
		t.Fatal(css)
	}
}
