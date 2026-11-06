package rocketwind

import (
	"strconv"
	"testing"
)

var benchmarkCSS string

func benchmarkCandidates() []string {
	classes := []string{"flex", "grid", "items-center", "justify-between", "rounded-lg", "shadow-lg", "text-xl", "font-semibold", "bg-white", "text-slate-900"}
	for _, variant := range []string{"", "hover:", "sm:", "lg:focus:"} {
		for _, root := range []string{"p", "mx", "gap", "w", "translate-x", "rounded"} {
			for i := 1; i <= 24; i++ {
				classes = append(classes, variant+root+"-"+strconv.Itoa(i))
			}
		}
	}
	return classes
}

func BenchmarkBuild(b *testing.B) {
	classes := benchmarkCandidates()
	for _, size := range []int{10, 100, len(classes)} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			c := New(Config{})
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchmarkCSS, err = c.Build(classes[:size])
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkTemplate(b *testing.B) {
	c := New(Config{InlineTheme: true})
	template, err := c.Prepare("flex items-center rounded-lg p-{gap} text-{size} text-{color} hover:bg-{color}", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"gap": TypeLength, "size": TypeLength, "color": TypeColor})})
	if err != nil {
		b.Fatal(err)
	}
	resolve := func(p Parameter) (Value, error) {
		switch p.Expression {
		case "gap":
			return Value{CSS: "8px"}, nil
		case "size":
			return Value{CSS: "20px"}, nil
		default:
			return Value{CSS: "var(--brand)"}, nil
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		benchmarkCSS, err = template.CSS(".account-card", resolve)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIncrementalUnchanged(b *testing.B) {
	classes := benchmarkCandidates()
	builder := New(Config{}).NewBuilder()
	if _, err := builder.Build(classes); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var err error
		benchmarkCSS, err = builder.Build(classes)
		if err != nil {
			b.Fatal(err)
		}
	}
}
