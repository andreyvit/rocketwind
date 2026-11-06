package rocketwind

import (
	"strings"
	"sync"
	"testing"
)

func TestInternalVariablePrefix(t *testing.T) {
	for _, version := range []Compatibility{Tailwind4, Tailwind3} {
		c, err := Compile(`@tailwind utilities; .custom {--tw-custom: 3px; width:var(--tw-custom);content:"--tw-shadow";color:var(--brand);}`, Config{Compatibility: version, InternalVariablePrefix: "gox"})
		if err != nil {
			t.Fatal(err)
		}
		css, err := c.Build([]string{"shadow-[1px_2px_3px_black]", "rotate-12", "before:block"})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"--gox-tw-shadow", "--gox-tw-custom: 3px", "width: var(--gox-tw-custom)", `content: "--tw-shadow"`, "color: var(--brand)"} {
			if !strings.Contains(css, want) {
				t.Errorf("missing %q:\n%s", want, css)
			}
		}
		if strings.Contains(css, "var(--tw-") || strings.Contains(css, "@property --tw-") {
			t.Fatal(css)
		}
		ast := c.CandidateAST([]string{"shadow-[1px_2px_3px_black]"})
		if len(ast) != 1 || len(ast[0]) == 0 {
			t.Fatal(ast)
		}
		preview := c.CandidateCSS([]string{"shadow-[1px_2px_3px_black]"})[0]
		if !strings.Contains(preview, "--gox-tw-shadow") {
			t.Fatal(preview)
		}
		builder := c.NewBuilder()
		got, _, err := builder.BuildWithSourceMap([]string{"shadow-[1px_2px_3px_black]", "rotate-12", "before:block"})
		if err != nil || got != css {
			t.Fatal(err, got)
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				got, err := c.Build([]string{"shadow-[1px_2px_3px_black]", "rotate-12", "before:block"})
				if err != nil || got != css {
					t.Errorf("concurrent build changed: %v", err)
				}
			})
		}
		wg.Wait()
	}
}
