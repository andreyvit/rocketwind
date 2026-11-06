package rocketwind

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestUpstreamPreprocessors(t *testing.T) {
	extensions := map[string]string{"Clojure": "clj", "Elixir": "ex", "Haml": "haml", "Json": "json", "Markdown": "md", "Pug": "pug", "Razor": "razor", "Ruby": "rb", "Rust": "rs", "Slim": "slim", "Svelte": "svelte", "TemplateToolkit": "tt", "Twig": "twig", "Vue": "vue"}
	count := 0
	var failures []any
	for i, tt := range readOxideCases(t) {
		if !strings.HasPrefix(tt.Kind, "preprocess:") {
			continue
		}
		count++
		name := tt.Kind[strings.LastIndex(tt.Kind, "::")+2:]
		ext, ok := extensions[name]
		if !ok {
			t.Fatal(name)
		}
		var want string
		if len(tt.Result) > 0 && tt.Result[0] == '{' {
			var binary struct{ Bytes []byte }
			if err := json.Unmarshal(tt.Result, &binary); err != nil {
				t.Fatal(err)
			}
			want = string(binary.Bytes)
		} else if err := json.Unmarshal(tt.Result, &want); err != nil {
			t.Fatal(err)
		}
		got := Preprocess(tt.Input, ext)
		if got != want || len(got) != len(tt.Input) {
			failures = append(failures, map[string]any{"index": i, "kind": tt.Kind, "input": tt.Input, "got": got, "want": want})
		}
	}
	if count != 271 {
		t.Fatalf("got %d preprocessor calls, want 271", count)
	}
	if len(failures) > 0 {
		data, _ := json.MarshalIndent(failures, "", "  ")
		os.WriteFile(".cache/preprocess-failures.json", data, 0644)
		t.Fatalf("%d/%d upstream preprocessor calls match; see .cache/preprocess-failures.json", count-len(failures), count)
	}
}

func TestPreprocessPlainSources(t *testing.T) {
	input := `<section class="grid p-4 hover:bg-red-500"></section>`
	if got := Preprocess(input, "gox"); got != input {
		t.Fatal(got)
	}
	if got := Preprocess(`.p-4.text-red-500`, ".pug"); got != " p-4 text-red-500" {
		t.Fatal(got)
	}
	for _, ext := range []string{"clj", "ex", "haml", "json", "md", "pug", "razor", "rb", "rs", "slim", "svelte", "tt", "twig", "vue", "gox"} {
		if got := Preprocess("", ext); got != "" {
			t.Fatal(ext, got)
		}
	}
}

func FuzzSourceExtraction(f *testing.F) {
	for _, input := range []string{`<div class="p-4 hover:flex"></div>`, `[color:var(--color-brand)]`, `%w[flex p-4]`, "\xff\xf0\x9f", strings.Repeat("[", 35) + "red" + strings.Repeat("]", 35)} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		for _, ext := range []string{"", "pug", "haml", "slim", "rb", "clj", "json", "ex", "vue", "rs", "md", "tt", "twig"} {
			processed := Preprocess(input, ext)
			if len(processed) != len(input) {
				t.Fatalf("%s changed source length", ext)
			}
			for _, token := range Extract(processed) {
				if token.Start < 0 || token.End > len(input) || token.Start >= token.End || processed[token.Start:token.End] != token.Value {
					t.Fatalf("invalid token: %+v", token)
				}
			}
		}
	})
}
