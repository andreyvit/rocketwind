package rocketwind

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Content supplies source text or a file. When Path is set, Text is ignored.
// Extension selects a template preprocessor; omit its leading dot.
type Content struct{ Path, Text, Extension string }

// Scanner follows Tailwind's source and ignore rules. It retains candidates
// across scans, including candidates removed from a file, for incremental
// builds. Construct a new scanner for a clean build. A Scanner is not safe for
// concurrent use. Missing and unreadable files are skipped, as in Tailwind.
type Scanner struct {
	compatibility           Compatibility
	sources                 []scannedSource
	rules                   []scanIgnore
	candidates              map[string]bool
	files, dirs, extensions map[string]bool
	mtimes                  map[string]time.Time
	scanned                 []string
	discovered              bool
	buffer                  []Extracted
	v3Chunks                map[string]bool
}

// ScannerOptions controls filesystem discovery. Base is the working directory
// used to bound ancestor ignore rules; an empty Base uses the process directory.
type ScannerOptions struct {
	Base          string
	Compatibility Compatibility
}

func NewScanner(sources []Source, options ...ScannerOptions) *Scanner {
	base := ""
	compatibility := Tailwind4
	if len(options) > 0 {
		base = options[0].Base
		compatibility = options[0].Compatibility
	}
	prepared, rules := prepareScanSources(sources, base)
	return &Scanner{compatibility: compatibility, sources: prepared, rules: rules, candidates: map[string]bool{}, files: map[string]bool{}, dirs: map[string]bool{}, extensions: map[string]bool{}, mtimes: map[string]time.Time{}}
}

// NewScanner combines the stylesheet's @source directives with its automatic
// source root. Base is the default search directory; source(none) disables it.
func (c *Compiler) NewScanner(base string) *Scanner {
	var sources []Source
	if c.root == nil {
		sources = append(sources, Source{Base: base, Pattern: "**/*"})
	} else if !c.root.Disabled {
		sources = append(sources, Source{Base: c.root.Base, Pattern: c.root.Pattern})
	}
	return NewScanner(append(sources, c.sources...), ScannerOptions{Base: base, Compatibility: c.config.Compatibility})
}

// Scan discovers files, reads additions and changes, and returns all candidates.
func (s *Scanner) Scan() []string {
	s.discovered = false
	s.scanned = nil
	batch := scanBatch{scanner: s}
	for _, content := range s.discover() {
		s.addContent(content, true, nil, &batch)
	}
	batch.finish()
	return slices.Sorted(maps.Keys(s.candidates))
}

// ScanContent returns only new candidates. Raw text is always scanned. Paths
// must match the scanner's sources and ignore rules; known files remain allowed.
func (s *Scanner) ScanContent(contents []Content) []string {
	var added []string
	var allowed map[string][]string
	for _, content := range contents {
		if content.Path != "" {
			canonical, err := filepath.EvalSymlinks(content.Path)
			if err != nil {
				continue
			}
			canonical, _ = filepath.Abs(canonical)
			content.Path = canonical
			if !s.files[canonical] {
				if allowed == nil {
					allowed = map[string][]string{}
					s.walk(func(name, canonical string, info os.FileInfo) {
						if !info.IsDir() {
							allowed[canonical] = append(allowed[canonical], name)
						}
					})
				}
				if len(allowed[canonical]) == 0 {
					continue
				}
				for _, path := range allowed[canonical] {
					s.files[path] = true
				}
			}
		}
		s.addContent(content, false, &added, nil)
	}
	slices.Sort(added)
	return added
}

func (s *Scanner) addContent(content Content, cssOnly bool, added *[]string, batch *scanBatch) {
	text, ok := readScanContent(content)
	if !ok {
		return
	}
	for line := range strings.SplitSeq(text, "\n") {
		if s.compatibility == Tailwind3 {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if s.v3Chunks == nil {
				s.v3Chunks = map[string]bool{}
			}
			s.buffer = s.buffer[:0]
			// None of the legacy expressions cross ASCII whitespace. Smaller
			// inputs avoid the regexp engine's large-input NFA and let repeated
			// template fragments and bundled JavaScript share extraction work.
			for chunk := range strings.FieldsFuncSeq(line, func(r rune) bool { return r < 128 && cssSpace(byte(r)) }) {
				if s.v3Chunks[chunk] {
					continue
				}
				chunk = strings.Clone(chunk)
				s.v3Chunks[chunk] = true
				if batch != nil {
					batch.add(chunk, false)
				} else {
					s.buffer = appendExtractedV3(s.buffer, chunk)
				}
			}
		} else if batch != nil {
			batch.add(line, cssOnly && content.Extension == "css")
			s.buffer = s.buffer[:0]
		} else {
			s.buffer = extractScanText(s.buffer[:0], scanText{line, s.compatibility, cssOnly && content.Extension == "css"})
		}
		addScanTokens(s.candidates, s.buffer, added)
	}
	clear(s.buffer)
}

func readScanContent(content Content) (string, bool) {
	if content.Path != "" {
		data, err := os.ReadFile(content.Path)
		if err != nil {
			return "", false
		}
		content.Text = string(data)
	}
	return Preprocess(content.Text, content.Extension), true
}

// Positions returns candidate tokens and source byte offsets, without changing
// scanner state. It also recognizes legacy group-[] classes used by migrations.
func (s *Scanner) Positions(content Content) []Extracted {
	text, _ := readScanContent(content)
	processed := strings.ReplaceAll(text, "-[]", "XYZ")
	var result []Extracted
	tokens := Extract(processed)
	if s.compatibility == Tailwind3 {
		tokens = appendExtractedV3(nil, processed)
	}
	for _, token := range tokens {
		if token.Kind == "candidate" && utf8.ValidString(token.Value) {
			token.Value = text[token.Start:token.End]
			result = append(result, token)
		}
	}
	return result
}

func (s *Scanner) Files() []string        { s.discover(); return slices.Sorted(maps.Keys(s.files)) }
func (s *Scanner) ScannedFiles() []string { return slices.Clone(s.scanned) }
func (s *Scanner) Sources() []Glob {
	var result []Glob
	for _, source := range s.sources {
		if source.kind != "ignored" {
			pattern := source.pattern
			if source.kind != "pattern" {
				pattern = "**/*"
			}
			result = append(result, Glob{source.base, pattern})
		}
	}
	return result
}

func (s *Scanner) discover() []Content {
	if s.discovered {
		return nil
	}
	s.discovered = true
	clear(s.files)
	clear(s.dirs)
	clear(s.extensions)
	var changed []Content
	s.walk(func(name, canonical string, info os.FileInfo) {
		if info.IsDir() {
			s.dirs[name] = true
			return
		}
		if s.files[name] {
			return
		}
		s.files[name] = true
		s.files[canonical] = true
		ext := strings.TrimPrefix(filepath.Ext(name), ".")
		s.extensions[ext] = true
		previous, exists := s.mtimes[name]
		s.mtimes[name] = info.ModTime()
		if exists && previous.Equal(info.ModTime()) {
			return
		}
		changed = append(changed, Content{Path: name, Extension: ext})
	})
	for name := range s.mtimes {
		if !s.files[name] {
			delete(s.mtimes, name)
		}
	}
	for _, content := range changed {
		s.scanned = append(s.scanned, content.Path)
	}
	slices.Sort(s.scanned)
	return changed
}

func (s *Scanner) accepts(name string) bool {
	for _, source := range s.sources {
		if source.kind == "ignored" {
			continue
		}
		rel, ok := scanRelative(source.base, name)
		if !ok {
			continue
		}
		if source.kind != "pattern" || source.glob.match(rel) {
			return true
		}
	}
	return false
}

type scanDiskRules struct{ git, ignore, exclude []scanIgnore }

func (r scanDiskRules) child(dir string) scanDiskRules {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		r.git = nil
		r.exclude = nil
	}
	for _, item := range []struct {
		name  string
		rules *[]scanIgnore
	}{{".gitignore", &r.git}, {".ignore", &r.ignore}, {".git/info/exclude", &r.exclude}} {
		data, err := os.ReadFile(filepath.Join(dir, item.name))
		if err == nil {
			*item.rules = append(slices.Clone(*item.rules), newScanIgnores(dir, string(data), false)...)
		}
	}
	return r
}

func (s *Scanner) ignored(name string, dir bool, r scanDiskRules) bool {
	for _, rules := range [][]scanIgnore{s.rules, scanDefaultIgnores, r.ignore, r.git, r.exclude} {
		if result := scanIgnoreMatch(rules, name, dir); result != 0 {
			return result == 1
		}
	}
	return false
}

func (s *Scanner) walk(visit func(string, string, os.FileInfo)) {
	s.walkWithStat(visit, os.Stat)
}

// Separate metadata lookup to test files disappearing between discovery,
// stat, and symlink resolution without timing-dependent filesystem races.
func (s *Scanner) walkWithStat(visit func(string, string, os.FileInfo), stat func(string) (os.FileInfo, error)) {
	roots := map[string]bool{}
	for _, source := range s.sources {
		if source.kind != "ignored" {
			roots[source.base] = true
		}
	}
	var walk func(string, string, os.FileInfo, scanDiskRules, map[string]bool, bool)
	walk = func(name, canonical string, info os.FileInfo, rules scanDiskRules, ancestors map[string]bool, root bool) {
		if !info.Mode().IsRegular() && !info.IsDir() {
			return
		}
		if !root && s.ignored(name, info.IsDir(), rules) {
			return
		}
		if !info.IsDir() {
			if s.accepts(name) {
				visit(name, canonical, info)
			}
			return
		}
		if ancestors[canonical] {
			return
		}
		ancestors[canonical] = true
		defer delete(ancestors, canonical)
		visit(name, canonical, info)
		rules = rules.child(name)
		entries, _ := os.ReadDir(name)
		for _, entry := range entries {
			child := filepath.Join(name, entry.Name())
			childInfo, err := entry.Info()
			if err != nil {
				continue
			}
			real := child
			if entry.Type()&os.ModeSymlink != 0 {
				childInfo, err = stat(child)
				if err != nil {
					continue
				}
				real, err = filepath.EvalSymlinks(child)
				if err != nil {
					continue
				}
			} else if name != canonical {
				real = filepath.Join(canonical, entry.Name())
			}
			walk(child, real, childInfo, rules, ancestors, false)
		}
	}
	for _, root := range slices.Sorted(maps.Keys(roots)) {
		var parents []string
		for parent := filepath.Dir(root); ; parent = filepath.Dir(parent) {
			parents = append(parents, parent)
			if parent == filepath.Dir(parent) {
				break
			}
		}
		var rules scanDiskRules
		for i := len(parents) - 1; i >= 0; i-- {
			rules = rules.child(parents[i])
		}
		info, err := stat(root)
		if err != nil {
			continue
		}
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		walk(root, canonical, info, rules, map[string]bool{}, true)
	}
}
