package rocketwind

import (
	"encoding/json"
	"os"
	"testing"
)

func BenchmarkLoyaltyCompiler(b *testing.B) {
	benchmarkLoyaltyCompiler(b, Tailwind4)
}

func BenchmarkLoyaltyCompilerV3(b *testing.B) {
	benchmarkLoyaltyCompiler(b, Tailwind3)
}

func benchmarkLoyaltyCompiler(b *testing.B, compatibility Compatibility) {
	var fixture struct {
		Input   string
		InputV3 string
		Classes []string
	}
	data, err := os.ReadFile("testdata/performance/loyalty.json")
	if err != nil {
		b.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		b.Fatal(err)
	}
	if compatibility == Tailwind3 {
		fixture.Input = `@plugin "@tailwindcss/forms" { strategy: class; }
@plugin "@tailwindcss/typography";
` + fixture.InputV3
	}
	b.ReportAllocs()
	for b.Loop() {
		compiler, err := Compile(fixture.Input, Config{Compatibility: compatibility})
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
