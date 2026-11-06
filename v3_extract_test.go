package rocketwind

import (
	"compress/gzip"
	"encoding/json"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestTailwind3OriginalExtractor(t *testing.T) {
	f, err := os.Open("testdata/v3/extractor.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var tests []struct {
		Name, Input string
		Results     []string
	}
	if err := json.NewDecoder(z).Decode(&tests); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			var got []string
			for _, token := range appendExtractedV3(nil, tt.Input) {
				got = append(got, token.Value)
			}
			slices.Sort(got)
			got = slices.Compact(got)
			scanner := NewScanner(nil, ScannerOptions{Compatibility: Tailwind3})
			if scanned := scanner.ScanContent([]Content{{Text: tt.Input}}); !slices.Equal(scanned, tt.Results) {
				t.Fatalf("scanner = %q; want %q", scanned, tt.Results)
			}
			if !slices.Equal(got, tt.Results) {
				var missing, extra []string
				for _, v := range tt.Results {
					if _, ok := slices.BinarySearch(got, v); !ok {
						missing = append(missing, v)
					}
				}
				for _, v := range got {
					if _, ok := slices.BinarySearch(tt.Results, v); !ok {
						extra = append(extra, v)
					}
				}
				t.Fatalf("missing %q; extra %q", missing, extra)
			}
		})
	}
}

func TestTailwind3BracketFreeExpressions(t *testing.T) {
	random := rand.New(rand.NewPCG(1729, 42))
	alphabet := []rune("abcZ01!@:-/.$={}<>\\\"'`()#%; \t\v\n\r\f[]é日\u2003")
	for range 16384 {
		text := make([]rune, random.IntN(128))
		for i := range text {
			text[i] = alphabet[random.IntN(len(alphabet))]
		}
		input := string(text)
		if strings.ContainsRune(input, '[') && (strings.ContainsRune(input, ':') || strings.Contains(input, "-[")) {
			continue
		}
		for i, fast := range v3SimpleRegexps[:2] {
			want := v3ExtractRegexps[i].FindAllStringIndex(input, -1)
			got := fast.FindAllStringIndex(input, -1)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("pattern %d, input %q: %v != %v", i, input, got, want)
			}
			var spans [][]int
			for _, token := range appendV3VariantTokens(nil, input, i == 1) {
				spans = append(spans, []int{token.Start, token.End})
			}
			if !reflect.DeepEqual(spans, want) {
				t.Fatalf("native pattern %d, input %q: %v != %v", i, input, spans, want)
			}
		}
	}
}
