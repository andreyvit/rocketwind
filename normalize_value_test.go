package rocketwind

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestUpstreamValueNormalization(t *testing.T) {
	for _, suite := range []struct {
		name  string
		count int
	}{{"canonicalize-calc-expressions", 12}, {"constant-fold-declaration", 77}, {"expand-declaration", 46}} {
		data, err := os.ReadFile("testdata/upstream/" + suite.name + ".test.ts.json")
		if err != nil {
			t.Fatal(err)
		}
		var cases []struct {
			Test   string
			Args   []json.RawMessage
			Result json.RawMessage
		}
		if err := json.Unmarshal(data, &cases); err != nil {
			t.Fatal(err)
		}
		if len(cases) != suite.count {
			t.Fatal(len(cases))
		}
		for _, tt := range cases {
			t.Run(suite.name+"/"+tt.Test, func(t *testing.T) {
				if suite.name == "expand-declaration" {
					var n upstreamNode
					var features signatureFeatures
					json.Unmarshal(tt.Args[0], &n)
					json.Unmarshal(tt.Args[1], &features)
					var want []upstreamNode
					json.Unmarshal(tt.Result, &want)
					got := expandDeclaration(convertUpstreamNodes([]upstreamNode{n}, "")[0], features)
					expected := convertUpstreamNodes(want, "")
					if len(got) == 0 && len(expected) == 0 {
						return
					}
					if !reflect.DeepEqual(got, expected) {
						t.Fatalf("got %#v, want %#v", got, expected)
					}
					return
				}
				var input, want string
				json.Unmarshal(tt.Args[0], &input)
				json.Unmarshal(tt.Result, &want)
				var got string
				if suite.name == "canonicalize-calc-expressions" {
					got = canonicalizeCalcExpressions(input)
				} else {
					var rem *float64
					normalize := true
					if len(tt.Args) > 1 {
						json.Unmarshal(tt.Args[1], &rem)
					}
					if len(tt.Args) > 2 {
						json.Unmarshal(tt.Args[2], &normalize)
					}
					got = constantFoldDeclaration(input, rem, normalize)
				}
				if got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestValueNormalizationBoundaries(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/value-normalization-extra.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Kind, Input, Result string
		Rem                 *float64
		Normalize           bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases {
		var got string
		if tt.Kind == "canonicalize" {
			got = canonicalizeCalcExpressions(tt.Input)
		} else {
			got = constantFoldDeclaration(tt.Input, tt.Rem, tt.Normalize)
		}
		if got != tt.Result {
			t.Errorf("%s %q: got %q, want %q", tt.Kind, tt.Input, got, tt.Result)
		}
	}
}

func TestExpandDeclarationBoundaries(t *testing.T) {
	if got := expandDeclaration(&Node{Kind: "declaration", Name: "color", Value: "red"}, 0); got != nil {
		t.Fatal(got)
	}
	got := expandDeclaration(&Node{Kind: "declaration", Name: "overscroll-behavior", Value: "contain none", Important: true}, 0)
	if len(got) != 2 || got[0].Name != "overscroll-behavior-x" || got[0].Value != "contain" || !got[0].Important || got[1].Name != "overscroll-behavior-y" || got[1].Value != "none" {
		t.Fatalf("unexpected expansion: %#v", got)
	}
	if got := expandDeclaration(&Node{Kind: "declaration", Name: "margin", Value: "1px 2px 3px 4px 5px"}, 0); got != nil {
		t.Fatal(got)
	}
}
