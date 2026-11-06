package rocketwind

import (
	"reflect"
	"testing"
)

func TestFlattenColors(t *testing.T) {
	colors := ConfigObject{
		{"brand", ConfigObject{{"DEFAULT", "red"}, {"light", "pink"}}},
		{"brand-light", "white"}, {"unset", nil},
		{"__CSS_VALUES__", ConfigObject{{"brand-light", 0}, {"unset", 4}}},
	}
	want := ConfigObject{{"brand", "red"}, {"brand-light", "white"}, {"unset", nil}}
	if got := FlattenColors(colors); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if got := FlattenColors(nil); len(got) != 0 {
		t.Fatal(got)
	}
}
