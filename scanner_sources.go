package rocketwind

import (
	"embed"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Lists copied from Tailwind CSS 4.3.4, crates/oxide/src/scanner/fixtures.
// MIT license: testdata/upstream/LICENSE.
//
//go:embed data/scanner/*.txt
var scanData embed.FS

var scanIgnoredDirectories = scanDataLines("ignored-content-dirs")
var scanKnownExtensions = scanDataLines("template-extensions")
var scanBinaryPattern = "*.{" + strings.Join(scanDataLines("binary-extensions"), ",") + "}"
var scanDefaultIgnores = func() []scanIgnore {
	var rules []scanIgnore
	for _, name := range scanIgnoredDirectories {
		rules = append(rules, newScanIgnores("", name+"/", false)...)
	}
	for _, ext := range scanDataLines("ignored-extensions") {
		rules = append(rules, newScanIgnores("", "*."+ext, false)...)
	}
	for _, name := range scanDataLines("ignored-files") {
		rules = append(rules, newScanIgnores("", name, false)...)
	}
	return append(rules, newScanIgnores("", scanBinaryPattern, true)...)
}()

func scanDataLines(name string) []string {
	data, _ := scanData.ReadFile("data/scanner/" + name + ".txt")
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line != "" && line[0] != '#' {
			lines = append(lines, line)
		}
	}
	return lines
}

type scannedSource struct {
	base, pattern, kind string
	glob                scanGlob
}

func normalizeScanSource(source Source) scannedSource {
	base, err := filepath.EvalSymlinks(source.Base)
	if err != nil {
		return scannedSource{base: source.Base, pattern: source.Pattern, kind: "pattern", glob: newScanGlob(source.Pattern)}
	}
	base, _ = filepath.Abs(base)
	pattern := filepath.ToSlash(source.Pattern)
	if strings.HasPrefix(pattern, "/") {
		base = string(filepath.Separator)
	}
	parts := strings.Split(pattern, "/")
	var dynamic []string
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if len(dynamic) > 0 || strings.Contains(part, "*") {
			dynamic = append(dynamic, part)
			continue
		}
		if part == ".." {
			base = filepath.Dir(base)
			continue
		}
		full := filepath.Join(base, part)
		info, _ := os.Stat(full)
		if i < len(parts)-1 || info != nil && info.IsDir() {
			base = full
		} else {
			dynamic = append(dynamic, part)
		}
	}
	pattern = "/" + strings.Join(dynamic, "/")
	if pattern == "/" {
		pattern = "/**/*"
	}
	kind := "pattern"
	if source.Negated {
		kind = "ignored"
	} else if pattern == "/**/*" {
		kind = "auto"
		for _, part := range strings.Split(filepath.ToSlash(base), "/") {
			if slices.Contains(scanIgnoredDirectories, part) {
				kind = "external"
				break
			}
		}
		if kind == "auto" && scanIgnoredSource(base) {
			kind = "external"
		}
	}
	return scannedSource{base: base, pattern: pattern, kind: kind, glob: newScanGlob(strings.TrimPrefix(pattern, "/"))}
}

func scanIgnoredSource(base string) bool {
	for dir := filepath.Dir(base); ; dir = filepath.Dir(dir) {
		data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		rules := newScanIgnores(dir, string(data), false)
		for path := base; path != dir; path = filepath.Dir(path) {
			if scanIgnoreMatch(rules, path, true) == 1 {
				return true
			}
			if filepath.Dir(path) == path {
				break
			}
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return false
}

func prepareScanSources(input []Source) ([]scannedSource, []scanIgnore) {
	var sources []scannedSource
	for _, source := range input {
		patterns, err := Expand(source.Pattern)
		if err != nil {
			patterns = []string{source.Pattern}
		}
		for _, pattern := range patterns {
			source.Pattern = pattern
			sources = append(sources, normalizeScanSource(source))
		}
	}
	sources = restrictScanSources(sources)
	var rules []scanIgnore
	emit := func(base, pattern string, fileOnly bool) {
		rules = append(rules, newScanIgnores(base, pattern, fileOnly)...)
	}
	for _, source := range sources {
		switch source.kind {
		case "external":
			emit(source.base, "!/**/*", false)
			emit(source.base, scanBinaryPattern, true)
		case "ignored":
			emit(source.base, source.pattern, false)
		case "pattern":
			if !strings.Contains(source.pattern, "**") {
				emit(source.base, "!"+source.pattern, false)
			} else if ext := filepath.Ext(source.pattern); ext != "" {
				emit(source.base, "!*"+ext, false)
			}
		}
	}
	return sources, rules
}

func restrictScanSources(sources []scannedSource) []scannedSource {
	var unrestricted, roots []string
	for _, source := range sources {
		if source.kind == "auto" || source.kind == "external" || source.kind == "pattern" && strings.Contains(source.pattern, "**") {
			unrestricted = append(unrestricted, source.base)
		}
		if source.kind == "pattern" {
			roots = append(roots, source.base)
		}
	}
	var expanded []scannedSource
	restricted := map[string]bool{}
	emit := func(base, pattern string) {
		expanded = append(expanded, scannedSource{base: base, pattern: pattern, kind: "ignored"})
	}
	for _, source := range sources {
		if source.kind == "pattern" {
			isUnrestricted := false
			for _, root := range unrestricted {
				if _, ok := scanRelative(root, source.base); ok {
					isUnrestricted = true
					break
				}
			}
			if !isUnrestricted {
				if !restricted[source.base] {
					restricted[source.base] = true
					pattern := "*"
					for _, root := range append(slices.Clone(unrestricted), roots...) {
						if _, ok := scanRelative(source.base, root); ok && root != source.base {
							pattern = "/*"
							break
						}
					}
					emit(source.base, pattern)
				}
				parts := strings.Split(strings.TrimPrefix(source.pattern, "/"), "/")
				for i := 1; i < len(parts); i++ {
					emit(source.base, "!/"+strings.Join(parts[:i], "/")+"/")
				}
			}
		}
		expanded = append(expanded, source)
	}
	return expanded
}
