package rocketwind

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFormsAgainstUpstream(t *testing.T) {
	data, err := os.ReadFile("testdata/plugins/forms/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		CSS, Result string
		Candidates  []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 24 {
		t.Fatalf("got %d fixtures, want 24", len(cases))
	}
	var failures []map[string]any
	for i, tt := range cases {
		c, err := Compile(tt.CSS, Config{})
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		got, err := c.Build(tt.Candidates)
		// The npm distribution adds a compiler branding comment. Compare all CSS.
		tt.Result = strings.TrimPrefix(tt.Result, "/*! tailwindcss v4.3.4 | MIT License | https://tailwindcss.com */\n")
		if err != nil || strings.TrimSpace(got) != strings.TrimSpace(tt.Result) {
			failures = append(failures, map[string]any{"index": i, "input": tt.CSS, "got": got, "want": tt.Result, "error": err})
		}
	}
	if len(failures) > 0 {
		data, _ := json.MarshalIndent(failures, "", "  ")
		os.MkdirAll(".cache", 0755)
		os.WriteFile(".cache/forms-failures.json", data, 0644)
		t.Fatalf("%d/%d forms fixtures differ; .cache/forms-failures.json", len(failures), len(cases))
	}
}

func TestFormsSVGEncoding(t *testing.T) {
	data, err := os.ReadFile("testdata/plugins/forms/svg.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Input, Result string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		if got := svgDataURI(tt.Input); got != tt.Result {
			t.Errorf("%s: got %s, want %s", tt.Input, got, tt.Result)
		}
	}
}

func TestFormsProgrammatic(t *testing.T) {
	c, err := NewWithPlugins(Config{}, Forms(FormsOptions{Strategy: "class"}))
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"form-select"})
	if err != nil || strings.Contains(css, "RW_") || strings.Contains(css, "@layer base") || !strings.Contains(css, ".form-select") {
		t.Fatalf("%s %v", css, err)
	}
}

func TestStandardPluginLoadingErrors(t *testing.T) {
	for _, input := range []string{`@plugin "unknown-plugin";`, `@config "@tailwindcss/forms";`} {
		_, err := Compile(input, Config{})
		if err == nil || !strings.Contains(err.Error(), "requires a Go configuration or plugin") {
			t.Fatalf("%s: %v", input, err)
		}
	}
}
