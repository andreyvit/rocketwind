package rocketwind

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"slices"
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
