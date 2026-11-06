package rocketwind

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Glob identifies files for a watcher. Base is a directory; Pattern is relative.
type Glob struct{ Base, Pattern string }

type scanGlob struct {
	text string
	re   *regexp.Regexp
	kind uint8
}

func newScanGlob(pattern string) scanGlob {
	if !strings.ContainsAny(pattern, "*?[\\") {
		return scanGlob{text: pattern}
	}
	if strings.HasPrefix(pattern, "*") && !strings.ContainsAny(pattern[1:], "*?[\\") {
		return scanGlob{text: pattern[1:], kind: 1}
	}
	if strings.HasSuffix(pattern, "*") && !strings.ContainsAny(pattern[:len(pattern)-1], "*?[\\") {
		return scanGlob{text: pattern[:len(pattern)-1], kind: 2}
	}
	var out strings.Builder
	out.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch ch := pattern[i]; ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					out.WriteString("(?:.*/)?")
				} else {
					out.WriteString(".*")
				}
			} else {
				out.WriteString("[^/]*")
			}
		case '?':
			out.WriteString("[^/]")
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				out.WriteString(`\[`)
				continue
			}
			body := pattern[i+1 : i+1+end]
			if strings.HasPrefix(body, "!") {
				body = "^" + body[1:]
			}
			out.WriteByte('[')
			out.WriteString(body)
			out.WriteByte(']')
			i += end + 1
		case '\\':
			if i+1 < len(pattern) {
				i++
				out.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			} else {
				out.WriteString(`\\`)
			}
		default:
			out.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	out.WriteByte('$')
	re, _ := regexp.Compile(out.String())
	return scanGlob{text: pattern, re: re, kind: 3}
}

func (g scanGlob) match(path string) bool {
	switch g.kind {
	case 0:
		return path == g.text
	case 1:
		return strings.HasSuffix(path, g.text) && !strings.Contains(path[:len(path)-len(g.text)], "/")
	case 2:
		return strings.HasPrefix(path, g.text) && !strings.Contains(path[len(g.text):], "/")
	default:
		return g.re != nil && g.re.MatchString(path)
	}
}

type scanIgnore struct {
	base                                 string
	glob                                 scanGlob
	allow, directory, anchored, fileOnly bool
}

func newScanIgnores(base, input string, fileOnly bool) []scanIgnore {
	var rules []scanIgnore
	for line := range strings.SplitSeq(input, "\n") {
		line = strings.TrimRight(line, "\r")
		for strings.HasSuffix(line, " ") && !strings.HasSuffix(line, `\ `) {
			line = line[:len(line)-1]
		}
		if line == "" || line[0] == '#' {
			continue
		}
		allow := line[0] == '!'
		if allow {
			line = line[1:]
		}
		dir := strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		anchored := strings.Contains(line, "/")
		line = strings.TrimPrefix(line, "/")
		patterns, err := Expand(line)
		if err != nil {
			patterns = []string{line}
		}
		for _, pattern := range patterns {
			rules = append(rules, scanIgnore{base: base, glob: newScanGlob(pattern), allow: allow, directory: dir, anchored: anchored, fileOnly: fileOnly})
		}
	}
	return rules
}

func scanRelative(base, name string) (string, bool) {
	if base == "" {
		return filepath.ToSlash(name), true
	}
	if name == base {
		return "", true
	}
	if len(name) > len(base) && strings.HasPrefix(name, base) && (base == string(filepath.Separator) || name[len(base)] == filepath.Separator) {
		return filepath.ToSlash(strings.TrimPrefix(name[len(base):], string(filepath.Separator))), true
	}
	return "", false
}

// -1 allows, 1 ignores, 0 leaves the decision to lower-priority rules.
func scanIgnoreMatch(rules []scanIgnore, name string, dir bool) int {
	for i := len(rules) - 1; i >= 0; i-- {
		rule := &rules[i]
		if rule.directory && !dir || rule.fileOnly && dir {
			continue
		}
		relative, ok := scanRelative(rule.base, name)
		if !ok {
			continue
		}
		if !rule.anchored {
			relative = filepath.Base(relative)
		}
		if rule.glob.match(relative) {
			if rule.allow {
				return -1
			}
			return 1
		}
	}
	return 0
}
