package rocketwind

import (
	"encoding/json"
	"os"
	"testing"
)

func TestUpstreamArbitraryValues(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/utils_decode-arbitrary-value.test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Test   string
		Args   []string
		Result string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 66 {
		t.Fatalf("got %d fixtures, want 66", len(cases))
	}
	for _, tt := range cases {
		t.Run(tt.Test, func(t *testing.T) {
			if got := DecodeArbitraryValue(tt.Args[0]); got != tt.Result {
				t.Errorf("%q: got %q, want %q", tt.Args[0], got, tt.Result)
			}
		})
	}
}
