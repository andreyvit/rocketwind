package rocketwind

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
)

func TestExtractionEdgesAgainstRust(t *testing.T) {
	file, err := os.Open("testdata/upstream/extraction-edges.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var cases []struct {
		Input     []byte
		Outputs   map[string][]byte
		Extracted []struct {
			Kind       string
			Start, End int
		}
	}
	if err := json.NewDecoder(reader).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 784 {
		t.Fatalf("got %d extraction inputs", len(cases))
	}
	for _, c := range cases {
		if len(c.Outputs) != 14 {
			t.Fatalf("got %d preprocessors", len(c.Outputs))
		}
		input := string(c.Input)
		for extension, output := range c.Outputs {
			if got := Preprocess(input, extension); got != string(output) {
				t.Errorf("%s %q: got %q; want %q", extension, input, got, output)
			}
		}
		got := Extract(input)
		if len(got) != len(c.Extracted) {
			t.Errorf("%q: got %v; want %v", input, got, c.Extracted)
			continue
		}
		for i, v := range got {
			want := c.Extracted[i]
			if v.Kind != want.Kind || v.Start != want.Start || v.End != want.End || v.Value != input[want.Start:want.End] {
				t.Errorf("%q: got %v; want %v", input, got, c.Extracted)
				break
			}
		}
	}
}
