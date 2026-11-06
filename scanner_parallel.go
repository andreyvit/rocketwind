package rocketwind

import (
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
)

// File discovery and reads stay ordered, including reads from custom fs.FS
// implementations. Only extraction runs concurrently. The producer owns the
// legacy fragment cache; each worker owns its tokens and candidate map.
type scanBatch struct {
	scanner *Scanner
	pending []scanText
	bytes   int
	jobs    chan []scanText
	workers []map[string]bool
	wait    sync.WaitGroup
}

type scanText struct {
	text string
	kind Compatibility
	css  bool
}

func (b *scanBatch) add(text string, css bool) {
	b.pending = append(b.pending, scanText{text, b.scanner.compatibility, css})
	b.bytes += len(text)
	if b.bytes < 64*1024 && len(b.pending) < 1024 {
		return
	}
	if b.jobs == nil {
		count := min(8, runtime.GOMAXPROCS(0))
		b.jobs = make(chan []scanText, count)
		b.workers = make([]map[string]bool, count)
		for i := range count {
			b.workers[i] = make(map[string]bool)
			b.wait.Add(1)
			go func() {
				defer b.wait.Done()
				var buffer []Extracted
				for batch := range b.jobs {
					for _, input := range batch {
						buffer = extractScanText(buffer[:0], input)
						addScanTokens(b.workers[i], buffer, nil)
					}
				}
			}()
		}
	}
	b.jobs <- b.pending
	b.pending, b.bytes = nil, 0
}

func (b *scanBatch) finish() {
	if b.jobs == nil {
		for _, input := range b.pending {
			b.scanner.buffer = extractScanText(b.scanner.buffer[:0], input)
			addScanTokens(b.scanner.candidates, b.scanner.buffer, nil)
		}
		clear(b.scanner.buffer)
		return
	}
	if len(b.pending) > 0 {
		b.jobs <- b.pending
	}
	close(b.jobs)
	b.wait.Wait()
	for _, candidates := range b.workers {
		for candidate := range candidates {
			b.scanner.candidates[candidate] = true
		}
	}
}

func extractScanText(dst []Extracted, input scanText) []Extracted {
	if input.kind == Tailwind3 {
		return appendExtractedV3(dst, input.text)
	}
	if input.css {
		return append(dst, CSSVariables(input.text)...)
	}
	return AppendExtracted(dst, input.text)
}

func addScanTokens(candidates map[string]bool, tokens []Extracted, added *[]string) {
	for _, token := range tokens {
		if !candidates[token.Value] && utf8.ValidString(token.Value) {
			value := strings.Clone(token.Value)
			candidates[value] = true
			if added != nil {
				*added = append(*added, value)
			}
		}
	}
}
