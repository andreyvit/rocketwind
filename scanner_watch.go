package rocketwind

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Globs returns watch patterns for new files. Watch Files as well to cover
// existing files in directories, such as public, that mix inputs and outputs.
func (s *Scanner) Globs() []Glob {
	s.discover()
	var result []Glob
	for _, source := range s.sources {
		switch source.kind {
		case "auto", "external":
			result = append(result, s.watchDirectory(source.base)...)
		case "pattern":
			result = append(result, Glob{source.base, source.pattern})
		}
	}
	return optimizeScanGlobs(result)
}

func (s *Scanner) watchDirectory(base string) []Glob {
	exts := maps.Clone(s.extensions)
	for _, ext := range scanKnownExtensions {
		exts[ext] = true
	}
	extensions := strings.Join(slices.Sorted(maps.Keys(exts)), ",")
	shallow, deep := map[string]bool{}, map[string]bool{}
	var walk func(string, bool)
	walk = func(name string, findShallow bool) {
		info, err := os.Stat(name)
		if err != nil || !info.IsDir() {
			return
		}
		if findShallow {
			if !s.dirs[name] {
				for parent := filepath.Dir(name); parent != base && parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
					shallow[parent] = true
				}
				return
			}
		} else if name != base {
			if slices.Contains(scanIgnoredDirectories, filepath.Base(name)) || !s.dirs[name] {
				return
			}
			if !shallow[name] {
				if name != filepath.Join(base, "public") {
					deep[name] = true
				}
				return
			}
		}
		if link, err := os.Lstat(name); err != nil || link.Mode()&os.ModeSymlink != 0 {
			return
		}
		entries, _ := os.ReadDir(name)
		for _, entry := range entries {
			walk(filepath.Join(name, entry.Name()), findShallow)
		}
	}
	walk(base, true)
	walk(base, false)
	result := []Glob{{base, "*"}}
	for name := range shallow {
		result = append(result, Glob{name, "*/*.{" + extensions + "}"})
	}
	for name := range deep {
		result = append(result, Glob{name, "**/*.{" + extensions + "}"})
	}
	return result
}

func optimizeScanGlobs(input []Glob) []Glob {
	return optimizeScanGlobsWithStat(input, os.Stat)
}

func optimizeScanGlobsWithStat(input []Glob, stat func(string) (os.FileInfo, error)) []Glob {
	groups := map[string]map[string]bool{}
	add := func(base, pattern string) {
		if groups[base] == nil {
			groups[base] = map[string]bool{}
		}
		groups[base][pattern] = true
	}
	for _, glob := range input {
		base, pattern := glob.Base, glob.Pattern
		static := ""
		if !strings.Contains(pattern, "*") {
			static, pattern = pattern, ""
		} else {
			last := -1
			for i, ch := range pattern {
				if ch == '/' {
					last = i
				}
				if ch == '*' || ch == '!' {
					break
				}
			}
			if last >= 0 {
				static, pattern = pattern[:last], pattern[last+1:]
			}
		}
		if filepath.IsAbs(static) {
			base = static
		} else {
			base = filepath.Join(base, static)
		}
		base, err := filepath.EvalSymlinks(base)
		if err != nil {
			continue
		}
		info, err := stat(base)
		if err != nil {
			continue
		}
		if pattern == "" {
			if info.IsDir() {
				pattern = "**/*"
			} else {
				add(filepath.Dir(base), filepath.Base(base))
			}
		}
		add(base, pattern)
	}
	var result []Glob
	for _, base := range slices.Sorted(maps.Keys(groups)) {
		patterns := slices.Sorted(maps.Keys(groups[base]))
		pattern := patterns[0]
		if len(patterns) > 1 {
			pattern = "{" + strings.Join(patterns, ",") + "}"
		}
		result = append(result, Glob{base, pattern})
	}
	return result
}
