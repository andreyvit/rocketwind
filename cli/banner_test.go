package cli

import (
	"testing"

	"github.com/andreyvit/rocketwind"
)

func TestBannerPreservesSourceMapsAndCachesOutput(t *testing.T) {
	var cache bannerCache
	input := ".flex {\n  display: flex;\n}\n"
	source := &rocketwind.SourceMap{Version: 3, Mappings: "AAAA;EAAA"}
	css, mapped := cache.run(input, source)
	if css != tailwindBanner+input || mapped == source || mapped.Mappings != ";AAAA;EAAA" || source.Mappings != "AAAA;EAAA" {
		t.Fatal(css, mapped, source)
	}
	if allocations := testing.AllocsPerRun(100, func() { _, _ = cache.run(input, source) }); allocations != 0 {
		t.Fatalf("unchanged output: %g allocations", allocations)
	}
	_, again := cache.run(input, source)
	if again != mapped {
		t.Fatal("unchanged map was copied")
	}
	css, mapped = cache.run(input, nil)
	if css != tailwindBanner+input || mapped != nil {
		t.Fatal(css, mapped)
	}
	css, mapped = cache.run("", nil)
	if css != tailwindBanner || mapped != nil {
		t.Fatal(css, mapped)
	}
}
