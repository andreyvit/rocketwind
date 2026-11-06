package rocketwind

import (
	"cmp"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestSyntaxEdgesAgainstUpstream(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/syntax-edges.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Kind, Input, Base, Root, Error string
		Result                         json.RawMessage
		Types                          []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 44 {
		t.Fatalf("got %d syntax cases", len(cases))
	}
	for _, c := range cases {
		var got any
		var err error
		switch c.Kind {
		case "variant-order":
			// Tailwind permits variants in one ordering group. Compare their
			// names as a tie-breaker, independently of registration order.
			b := buildContext{compiler: New(Config{})}
			a := &parsedVariant{root: c.Input, definition: variantDefinition{Kind: "static", Order: 100}}
			z := &parsedVariant{root: c.Base, definition: a.definition}
			got = cmp.Compare(b.compareParsedVariant(a, z), 0)
		case "selector":
			got = encodeSelectors(parseSelector(c.Input))
		case "attribute":
			got = parseAttribute(c.Input)
		case "value":
			got = encodeValues(parseValue(c.Input))
		case "url":
			got, err = RewriteURLs(c.Input, c.Base, c.Root)
		case "type":
			if value := inferType(c.Input, c.Types...); value != "" {
				got = value
			}
		default:
			t.Fatal(c.Kind)
		}
		if c.Error != "" {
			if err == nil || err.Error() != c.Error {
				t.Errorf("%s %q: %v; want %s", c.Kind, c.Input, err, c.Error)
			}
			continue
		}
		var want any
		if err != nil {
			t.Errorf("%s %q: %v", c.Kind, c.Input, err)
			continue
		}
		if err := json.Unmarshal(c.Result, &want); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var actual any
		if err := json.Unmarshal(encoded, &actual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, want) {
			t.Errorf("%s %q: got %s; want %s", c.Kind, c.Input, encoded, c.Result)
		}
	}
}
