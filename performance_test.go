package rocketwind

import (
	"encoding/json"
	"os"
	"testing"
)

func BenchmarkLoyaltyCompiler(b *testing.B) {
	var fixture struct {
		Input   string
		Classes []string
	}
	data, err := os.ReadFile("testdata/performance/loyalty.json")
	if err != nil {
		b.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		compiler, err := Compile(fixture.Input, Config{})
		if err != nil {
			b.Fatal(err)
		}
		css, err := compiler.Build(fixture.Classes)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkCSS, err = Optimize(css, OptimizeOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}
