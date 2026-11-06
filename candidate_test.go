package rocketwind

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"
)

type candidateRegistry struct {
	Prefix             string
	Static, Functional []string
	Variants           []struct {
		Root, Kind                      string
		Order, Compounds, CompoundsWith int
	}
}

func TestUpstreamCandidates(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/candidate.test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Kind, Test string
		Args       []json.RawMessage
		Result     json.RawMessage
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2672 {
		t.Fatalf("got %d calls, want 2672", len(cases))
	}
	registries := map[int]*candidateRegistry{}
	for i, tt := range cases {
		var ref struct {
			ID         int `json:"$registry"`
			Definition *candidateRegistry
		}
		json.Unmarshal(tt.Args[1], &ref)
		if ref.Definition != nil {
			registries[ref.ID] = ref.Definition
		}
		registry := registries[ref.ID]
		if registry == nil {
			t.Fatal("missing registry", ref.ID)
		}
		t.Run(fmt.Sprintf("%d_%s", i, tt.Test), func(t *testing.T) {
			lookup := func(name string) (variantDefinition, bool) {
				for _, v := range registry.Variants {
					if name == v.Root {
						return variantDefinition{Kind: v.Kind, Order: v.Order, Compounds: v.Compounds, CompoundsWith: v.CompoundsWith}, true
					}
				}
				return variantDefinition{}, false
			}
			parser := candidateParser{prefix: registry.Prefix, variant: lookup, hasUtility: func(name, kind string) bool {
				if kind == "static" {
					return slices.Contains(registry.Static, name)
				}
				return slices.Contains(registry.Functional, name)
			}}
			var got any
			if tt.Kind == "internal:candidate:parse" {
				var class string
				json.Unmarshal(tt.Args[0], &class)
				result := []any{}
				for candidate := range parser.parse(class) {
					result = append(result, encodeCandidate(candidate))
				}
				got = result
			} else {
				var raw struct {
					Kind, Root, Raw, Property string
					Important                 bool
					Value                     json.RawMessage
					Modifier                  *CandidateModifier
					Variants                  []*CandidateVariant
				}
				json.Unmarshal(tt.Args[0], &raw)
				c := Candidate{Kind: raw.Kind, Root: raw.Root, Raw: raw.Raw, Property: raw.Property, Important: raw.Important, Modifier: raw.Modifier, Variants: raw.Variants}
				if raw.Kind == "arbitrary" {
					var value string
					json.Unmarshal(raw.Value, &value)
					c.Value = &CandidateValue{Kind: "arbitrary", Value: value}
				} else {
					json.Unmarshal(raw.Value, &c.Value)
				}
				got = New(Config{Prefix: registry.Prefix}).Print(c)
			}
			var want, normalized any
			json.Unmarshal(tt.Result, &want)
			encoded, _ := json.Marshal(got)
			json.Unmarshal(encoded, &normalized)
			if !reflect.DeepEqual(normalized, want) {
				t.Fatalf("input %s:\ngot %s\nwant %s", tt.Args[0], encoded, tt.Result)
			}
		})
	}
}

func encodeCandidate(c Candidate) map[string]any {
	variants := []any{}
	for _, v := range c.Variants {
		variants = append(variants, encodeVariant(v))
	}
	r := map[string]any{"kind": c.Kind, "raw": c.Raw, "important": c.Important, "variants": variants}
	if c.Kind == "arbitrary" {
		r["property"], r["value"] = c.Property, c.Value.Value
	} else {
		r["root"] = c.Root
	}
	if c.Kind == "functional" {
		r["value"] = nil
		if c.Value != nil {
			v := map[string]any{"kind": c.Value.Kind, "value": c.Value.Value}
			if c.Value.Kind == "arbitrary" {
				v["dataType"] = nullable(c.Value.DataType)
			} else {
				v["fraction"] = nullable(c.Value.Fraction)
			}
			r["value"] = v
		}
	}
	if c.Kind != "static" {
		r["modifier"] = encodeModifier(c.Modifier)
	}
	return r
}

func encodeVariant(v *CandidateVariant) map[string]any {
	r := map[string]any{"kind": v.Kind}
	if v.Kind == "arbitrary" {
		r["selector"], r["relative"] = v.Selector, v.Relative
		return r
	}
	r["root"] = v.Root
	if v.Kind == "functional" {
		r["value"] = nil
		if v.Value != nil {
			r["value"] = map[string]any{"kind": v.Value.Kind, "value": v.Value.Value}
		}
	}
	if v.Kind == "compound" {
		r["variant"] = encodeVariant(v.Variant)
	}
	if v.Kind != "static" {
		r["modifier"] = encodeModifier(v.Modifier)
	}
	return r
}

func encodeModifier(v *CandidateModifier) any {
	if v == nil {
		return nil
	}
	return map[string]any{"kind": v.Kind, "value": v.Value}
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
