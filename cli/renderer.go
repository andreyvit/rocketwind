package cli

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/andreyvit/rocketwind"
)

func formatNanoseconds(ns int64) string {
	for _, unit := range []struct {
		name  string
		limit int64
	}{{"ns", 1000}, {"µs", 1000}, {"ms", 1000}, {"s", 60}, {"m", 60}, {"h", 24}} {
		if ns < unit.limit {
			return strconv.FormatInt(ns, 10) + unit.name
		}
		ns /= unit.limit
	}
	return strconv.FormatInt(ns, 10) + "d"
}

func relativePath(to, from string, preferAbsolute bool) string {
	absTo, err := filepath.Abs(to)
	if err != nil {
		return to
	}
	absFrom, err := filepath.Abs(from)
	if err != nil {
		return to
	}
	result, err := filepath.Rel(absFrom, absTo)
	return displayRelativePath(to, result, err, preferAbsolute)
}

func displayRelativePath(to, result string, err error, preferAbsolute bool) string {
	if err != nil {
		return to
	}
	if result == "." {
		result = ""
	}
	if !strings.HasPrefix(result, "..") {
		result = "." + string(filepath.Separator) + result
	}
	if preferAbsolute && len(result) > len(to) {
		return to
	}
	return result
}

// Node's stripVTControlCharacters uses this ansi-regex pattern. Keeping escape
// sequences in the output preserves terminal styling while measuring text.
// MIT, Sindre Sorhus; see testdata/upstream/ANSI-REGEX-LICENSE.
var ansiControl = regexp.MustCompile(`[\x{001B}\x{009B}][[\]()#;?]*(?:(?:(?:(?:;[-a-zA-Z\d\/#&.:=?%@~_]+)*|[a-zA-Z\d]+(?:;[-a-zA-Z\d\/#&.:=?%@~_]*)*)?(?:\x{0007}|\x{001B}\\|\x{009C}))|(?:(?:\d{1,4}(?:;\d{0,4})*)?[\dA-PR-TZcf-nq-uy=><~]))`)

func wordWrap(text string, width int) []string {
	lines := make([]string, 0)
	line, length := "", 0
	for _, word := range strings.Split(text, " ") {
		visible := word
		if strings.ContainsAny(word, "\x1b\u009b") {
			visible = ansiControl.ReplaceAllString(word, "")
		}
		size := 0
		for _, r := range visible {
			size++
			if r > 0xffff {
				size++
			}
		}
		if length > 0 && length+size+1 > width {
			lines = append(lines, line)
			line, length = "", 0
		}
		if length > 0 {
			line += " "
			length++
		}
		line += word
		length += size
	}
	if length > 0 {
		lines = append(lines, line)
	}
	return lines
}

func formatError(err error, columns int) string {
	if _, ok := err.(*rocketwind.SyntaxError); ok {
		return "CssSyntaxError: " + err.Error()
	}
	if columns <= 0 {
		columns = int(^uint(0) >> 1)
	}
	var output strings.Builder
	depth := 0
	// Comparable error values cover ordinary wrapped errors and pointer cycles.
	// Non-comparable custom errors still stop at the depth limit.
	seen := map[error]bool{}
	for err != nil {
		indent := strings.Repeat("  ", depth)
		label := "Error:"
		if depth > 0 {
			label = "Caused by:"
		}
		fmt.Fprintf(&output, "%s%s\n%s┌\n", indent, label, indent)
		for _, line := range strings.Split(err.Error(), "\n") {
			chunks := wordWrap(line, columns-2*(depth+1))
			if len(chunks) == 0 {
				chunks = []string{""}
			}
			for _, chunk := range chunks {
				fmt.Fprintf(&output, "%s│ %s\n", indent, chunk)
			}
		}
		fmt.Fprintf(&output, "%s└", indent)
		wrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			break
		}
		if reflect.TypeOf(err).Comparable() {
			seen[err] = true
		}
		err = wrapped.Unwrap()
		if err == nil || reflect.TypeOf(err).Comparable() && seen[err] || depth == 100 {
			break
		}
		depth++
		output.WriteByte('\n')
	}
	return output.String()
}
