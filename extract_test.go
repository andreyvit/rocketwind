package rocketwind

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

type oxideCase struct {
	Kind, Input string
	InputBytes  []byte
	Result      json.RawMessage
}

func readOxideCases(t *testing.T) []oxideCase {
	t.Helper()
	data, err := os.ReadFile("testdata/upstream/oxide.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []oxideCase
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2968 {
		t.Fatalf("got %d calls, want 2968", len(cases))
	}
	for i := range cases {
		if cases[i].InputBytes != nil {
			cases[i].Input = string(cases[i].InputBytes)
		}
	}
	return cases
}

func TestUpstreamExtractor(t *testing.T) {
	var failures []any
	count := 0
	for i, tt := range readOxideCases(t) {
		if strings.HasPrefix(tt.Kind, "preprocess:") {
			continue
		}
		count++
		var got any
		if tt.Kind == "extract" || tt.Kind == "css-variables" {
			values := Extract(tt.Input)
			if tt.Kind == "css-variables" {
				values = CSSVariables(tt.Input)
			}
			items := []any{}
			for _, v := range values {
				items = append(items, map[string]any{"kind": v.Kind, "value": v.Value, "start": v.Start, "end": v.End})
			}
			got = items
		} else {
			got = extractWithMachine(t, tt.Kind, tt.Input)
		}
		data, _ := json.Marshal(got)
		json.Unmarshal(data, &got)
		var want any
		json.Unmarshal(tt.Result, &want)
		if !reflect.DeepEqual(got, want) {
			failures = append(failures, map[string]any{"index": i, "kind": tt.Kind, "input": tt.Input, "got": got, "want": want})
		}
	}
	if count != 2697 {
		t.Fatalf("got %d extractor calls, want 2697", count)
	}
	if len(failures) > 0 {
		data, _ := json.MarshalIndent(failures, "", "  ")
		os.WriteFile(".cache/extract-failures.json", data, 0644)
		t.Fatalf("%d/%d upstream extractor calls match; see .cache/extract-failures.json", count-len(failures), count)
	}
}

func extractWithMachine(t *testing.T, kind, input string) []string {
	t.Helper()
	values := []string{}
	for _, line := range strings.Split(strings.TrimSuffix(input, "\n"), "\n") {
		c := scanCursor{input: line}
		var next func(*scanCursor) (scanSpan, bool)
		switch kind {
		case "arbitrary_property_machine::ArbitraryPropertyMachine":
			next = scanArbitraryProperty
		case "arbitrary_value_machine::ArbitraryValueMachine":
			next = scanArbitraryValue
		case "arbitrary_variable_machine::ArbitraryVariableMachine":
			next = scanArbitraryVariable
		case "candidate_machine::CandidateMachine":
			next = (&scanCandidateMachine{}).next
		case "css_variable_machine::CssVariableMachine":
			next = scanCSSVariable
		case "modifier_machine::ModifierMachine":
			next = scanModifier
		case "named_utility_machine::NamedUtilityMachine":
			next = scanNamedUtility
		case "named_variant_machine::NamedVariantMachine":
			next = (&scanNamedVariantMachine{}).next
		case "string_machine::StringMachine":
			next = scanString
		case "utility_machine::UtilityMachine":
			next = scanUtility
		case "variant_machine::VariantMachine":
			next = (&scanVariantMachine{}).next
		default:
			t.Fatal(kind)
		}
		for c.pos < len(line) {
			if span, ok := next(&c); ok {
				values = append(values, line[span.start:span.end])
			}
			c.pos++
		}
	}
	return values
}

func TestExtractedCandidates(t *testing.T) {
	input := `<div class="p-4 hover:text-red-500 p-4" style="color: var(--color-brand)"></div>`
	want := []string{"--color-brand", "class", "color", "hover:text-red-500", "p-4", "style"}
	if got := Candidates(input); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	var storage [32]Extracted
	tokens := AppendExtracted(storage[:0], input)
	if len(tokens) == 0 || &tokens[0] != &storage[0] {
		t.Fatal("output buffer was not reused")
	}
	for _, token := range tokens {
		if input[token.Start:token.End] != token.Value {
			t.Fatal(token)
		}
	}
	css := `--color-unused: red; color:var(--color-brand); x:var(--size, var(--fallback)); y:--not-a-reference;`
	wantVars := []string{"--color-brand", "--size", "--fallback"}
	var gotVars []string
	for _, token := range CSSVariables(css) {
		gotVars = append(gotVars, token.Value)
	}
	if !reflect.DeepEqual(gotVars, wantVars) {
		t.Fatal(gotVars)
	}
	if got := Extract(""); len(got) != 0 {
		t.Fatal(got)
	}
	if got := CSSVariables("--a --b var( --space-is-not-extracted)"); len(got) != 0 {
		t.Fatal(got)
	}
}

func BenchmarkExtract(b *testing.B) {
	for _, n := range []int{1, 100} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			input := strings.Repeat(`<div class="absolute -top-1 -left-1.5 flex items-center justify-center p-1.5 text-gray-400 hover:text-gray-500 [--a:var(--color-brand)]"></div>`+"\n", n)
			out := Extract(input)
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			for b.Loop() {
				out = AppendExtracted(out[:0], input)
			}
		})
	}
	b.Run("upstream-html", func(b *testing.B) {
		data, err := os.ReadFile("testdata/upstream/example.html")
		if err != nil {
			b.Fatal(err)
		}
		input := string(data)
		out := Extract(input)
		b.ReportAllocs()
		b.SetBytes(int64(len(input)))
		for b.Loop() {
			out = AppendExtracted(out[:0], input)
		}
	})
}
