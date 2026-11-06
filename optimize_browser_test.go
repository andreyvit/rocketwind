package rocketwind

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// These snapshots are written only after the independent Lightning CSS
// comparison passes. Keep both the original Tailwind results and the verified
// Go spelling: tests need neither Node nor the reference optimizer at runtime.
func TestOptimizeVerifiedOutputs(t *testing.T) {
	var inputs struct {
		Cases []struct{ Test, CSS, Error string }
	}
	readOptimizerJSON(t, "testdata/upstream/optimize.json.gz", &inputs)
	var edges struct {
		Cases []struct{ Test, CSS, Error string }
	}
	readOptimizerJSON(t, "testdata/upstream/browser-edges.json", &edges)
	inputs.Cases = append(inputs.Cases, edges.Cases...)
	var outputs struct {
		Cases []struct{ InputHash, Pretty, Compact, Error string }
	}
	readOptimizerJSON(t, "testdata/upstream/optimize-go.json.gz", &outputs)
	if len(inputs.Cases) != len(outputs.Cases) {
		t.Fatalf("%d inputs, %d verified outputs", len(inputs.Cases), len(outputs.Cases))
	}
	for i, input := range inputs.Cases {
		t.Run(input.Test, func(t *testing.T) {
			want := outputs.Cases[i]
			if fmt.Sprintf("%x", sha256.Sum256([]byte(input.CSS))) != want.InputHash {
				t.Fatal("input changed; rerun independent optimizer verification")
			}
			for _, minify := range []bool{false, true} {
				options := OptimizeOptions{Minify: minify}
				got, err := Optimize(input.CSS, options)
				if want.Error != "" {
					if err == nil || err.Error() != want.Error || want.Error != input.Error {
						t.Fatalf("error: %v, want %q", err, want.Error)
					}
					continue
				}
				expected := want.Pretty
				if minify {
					expected = want.Compact
				}
				if err != nil || got != expected {
					t.Fatalf("minify=%v: %v\ngot: %s\nwant: %s", minify, err, got, expected)
				}
				again, err := Optimize(got, options)
				if err != nil || again != got {
					t.Fatalf("not idempotent: %v\nfirst: %s\nsecond: %s", err, got, again)
				}
			}
		})
	}
}

func readOptimizerJSON(t *testing.T, filename string, result any) {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if strings.HasSuffix(filename, ".gz") {
		reader, err := gzip.NewReader(file)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		if err := json.NewDecoder(reader).Decode(result); err != nil {
			t.Fatal(err)
		}
	} else {
		err = json.NewDecoder(file).Decode(result)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// Normalization for a semantic comparison must not be the only check of
// browser fallbacks: the normalizer can itself lower stretch or image-set.
func TestOptimizeBrowserFallbacks(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{`.x{width:stretch}`, `.x{width:-webkit-fill-available;width:-moz-available;width:stretch}`},
		{`.x{user-select:none!important}`, `.x{-webkit-user-select:none!important;user-select:none!important}`},
		{`.x{background-image:image-set("a.png" 1x)}`, `.x{background-image:-webkit-image-set(url("a.png") 1x);background-image:image-set("a.png" 1x)}`},
		{`@media (width>1px){.x{display:block}}`, `@media not all and (max-width: 1px){.x{display:block}}`},
		{`@container name (width<1px){.x{display:block}}`, `@container name not (min-width: 1px){.x{display:block}}`},
		{`@custom-media --small (width<1px);@media (--small){.x{display:block}}`, `@media not all and (min-width: 1px){.x{display:block}}`},
	} {
		got, err := Optimize(test.input, OptimizeOptions{Minify: true})
		if err != nil || got != test.want {
			t.Errorf("%s: %v\ngot %s\nwant %s", test.input, err, got, test.want)
		}
	}
}

func TestOptimizeBrowserSourceMap(t *testing.T) {
	input := ".a {\n  user-select: none;\n}\n@media (width > 40rem) { .b { display: flex } }"
	css, sourceMap, err := OptimizeWithSourceMap(input, OptimizeOptions{From: "browser.css"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	points, err := sourceMap.decode()
	if err != nil {
		t.Fatal(err)
	}
	foundPrefix, foundQuery := false, false
	lines := strings.Split(css, "\n")
	for _, point := range points {
		if point.Original == nil {
			continue
		}
		line := lines[point.Generated.Line-1]
		if strings.Contains(line, "-webkit-user-select") {
			foundPrefix = true
			if point.Original.Line != 2 {
				t.Fatalf("prefix mapped to %#v", point.Original)
			}
		}
		if strings.Contains(line, "@media not all") {
			foundQuery = true
			if point.Original.Line != 4 {
				t.Fatalf("query mapped to %#v", point.Original)
			}
		}
	}
	if !foundPrefix || !foundQuery {
		t.Fatalf("missing mapped transforms: prefix=%v query=%v", foundPrefix, foundQuery)
	}
}
