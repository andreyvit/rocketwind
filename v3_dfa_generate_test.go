package rocketwind

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"regexp/syntax"
	"strconv"
	"strings"
	"testing"
)

type extractorDFAState struct {
	pcs    []uint32
	accept bool
	next   []uint16
}

func TestGeneratedLegacyExtractorDFA(t *testing.T) {
	var output bytes.Buffer
	out := &output
	fmt.Fprintln(out, "// Code generated from the legacy extractor patterns; DO NOT EDIT.\n// Copyright Tailwind Labs. MIT license: testdata/upstream/LICENSE.\npackage rocketwind")
	fmt.Fprintln(out, "var v3ExtractDFAs = []extractDFA{")
	for _, pattern := range v3ExtractRegexps[:2] {
		source := pattern.String()
		expression, err := syntax.Parse(source, syntax.Perl)
		if err != nil {
			t.Fatal(err)
		}
		program, err := syntax.Compile(expression.Simplify())
		if err != nil {
			t.Fatal(err)
		}
		// Keep instruction order: alternatives and greedy repetitions choose
		// earlier threads. An accepting thread discards lower-priority paths,
		// while preceding threads can still extend the current match.
		closure := func(seeds []uint32) []uint32 {
			seen := make([]bool, len(program.Inst))
			var result []uint32
			done := false
			var add func(uint32)
			add = func(pc uint32) {
				if seen[pc] || done {
					return
				}
				seen[pc] = true
				ins := program.Inst[pc]
				switch ins.Op {
				case syntax.InstAlt, syntax.InstAltMatch:
					add(ins.Out)
					add(ins.Arg)
				case syntax.InstCapture, syntax.InstNop:
					add(ins.Out)
				case syntax.InstMatch:
					result = append(result, pc)
					done = true
				case syntax.InstRune, syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
					result = append(result, pc)
				case syntax.InstFail:
				default:
					t.Fatalf("unsupported extractor instruction: %v", ins.Op)
				}
			}
			for _, pc := range seeds {
				add(pc)
			}
			return result
		}
		var classes [128]uint8
		var representatives []byte
		bySig := map[string]uint8{}
		for ch := range 128 {
			var sig strings.Builder
			for _, ins := range program.Inst {
				switch ins.Op {
				case syntax.InstRune, syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
					if ins.MatchRune(rune(ch)) {
						sig.WriteByte(1)
					} else {
						sig.WriteByte(0)
					}
				}
			}
			key := sig.String()
			id, ok := bySig[key]
			if !ok {
				id = uint8(len(representatives))
				representatives = append(representatives, byte(ch))
				bySig[key] = id
			}
			classes[ch] = id
		}
		states := []extractorDFAState{{}}
		byPC := map[string]uint16{"[]": 0}
		intern := func(pcs []uint32) uint16 {
			key := fmt.Sprint(pcs)
			if id, ok := byPC[key]; ok {
				return id
			}
			id := uint16(len(states))
			// Bound generated data; the interpreter falls back to regexp if
			// an input needs a state beyond this table.
			if id >= 1024 {
				return 65535
			}
			accept := len(pcs) > 0 && program.Inst[pcs[len(pcs)-1]].Op == syntax.InstMatch
			states = append(states, extractorDFAState{pcs: pcs, accept: accept})
			byPC[key] = id
			return id
		}
		intern(closure([]uint32{uint32(program.Start)}))
		for i := 1; i < len(states); i++ {
			next := make([]uint16, len(representatives))
			for class, ch := range representatives {
				var seeds []uint32
				for _, pc := range states[i].pcs {
					ins := program.Inst[pc]
					if ins.Op != syntax.InstMatch && ins.MatchRune(rune(ch)) {
						seeds = append(seeds, ins.Out)
					}
				}
				next[class] = intern(closure(seeds))
			}
			states[i].next = next
		}
		fmt.Fprintf(out, "{classes: [128]uint8{%s}, width: %d, accepting: []bool{", dfaByteClasses(classes[:]), len(representatives))
		for _, s := range states {
			fmt.Fprintf(out, "%t,", s.accept)
		}
		fmt.Fprint(out, "}, transitions: []uint16{")
		for _, s := range states {
			next := s.next
			if next == nil {
				next = make([]uint16, len(representatives))
			}
			for _, n := range next {
				fmt.Fprintf(out, "%d,", n)
			}
		}
		fmt.Fprintln(out, "}},")
	}
	fmt.Fprintln(out, "}")
	generated, err := format.Source(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ROCKETWIND_REGENERATE_DFA") == "1" {
		if err := os.WriteFile("v3_dfa_generated.go", generated, 0644); err != nil {
			t.Fatal(err)
		}
	}
	checked, err := os.ReadFile("v3_dfa_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, checked) {
		t.Fatal("extractor patterns changed; run ROCKETWIND_REGENERATE_DFA=1 go test -run '^TestGeneratedLegacyExtractorDFA$'")
	}
}
func dfaByteClasses(values []uint8) string {
	s := make([]string, len(values))
	for i, v := range values {
		s[i] = strconv.Itoa(int(v))
	}
	return strings.Join(s, ",")
}
