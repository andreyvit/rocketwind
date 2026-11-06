package rocketwind

import (
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
)

// Discovery and custom fs.FS reads stay ordered. Disk scans prefetch files in
// parallel and consume them in discovery order. The producer owns the legacy
// fragment cache; each extraction worker owns its tokens and candidate map.
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

// Each reader can hold at most two file contents ahead of the consumer. Reads
// overlap without retaining the entire source tree or changing output order.
func (s *Scanner) scanDisk(contents []Content, batch *scanBatch) {
	count := min(8, runtime.GOMAXPROCS(0))
	if len(contents) < 32 || count == 1 {
		for _, content := range contents {
			s.addContent(content, true, nil, batch)
		}
		return
	}
	type result struct {
		text string
		ok   bool
	}
	readers := make([]chan result, count)
	for i := range count {
		readers[i] = make(chan result, 1)
		go func() {
			for index := i; index < len(contents); index += count {
				text, ok := readScanContent(contents[index])
				readers[i] <- result{text, ok}
			}
		}()
	}
	for i, content := range contents {
		read := <-readers[i%count]
		if read.ok {
			s.addText(read.text, content.Extension, true, nil, batch)
		}
	}
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
	if len(b.scanner.candidates) == 0 {
		count := 0
		for _, candidates := range b.workers {
			count += len(candidates)
		}
		b.scanner.candidates = make(map[string]bool, count)
	}
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
