// Diagnostic source-map visualization ported from Tailwind CSS v4.3.4 (MIT).
package rocketwind

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Visualize displays generated CSS beside its original source, with annotated
// mapping ranges. This is intended for diagnostics and snapshot tests.
func (m *SourceMap) Visualize(css string) (string, error) {
	points, err := m.decode()
	if err != nil {
		return "", err
	}
	return visualizeSourceMap(m, points, css), nil
}

func (m *SourceMap) decode() ([]decodedMapping, error) {
	if m.Version != 3 {
		return nil, fmt.Errorf("unsupported source map version %d", m.Version)
	}
	sources := make([]*decodedSource, len(m.Sources))
	for i := range m.Sources {
		sources[i] = &decodedSource{URL: &m.Sources[i]}
		if i < len(m.SourcesContent) {
			sources[i].Content = &m.SourcesContent[i]
		}
	}
	var result []decodedMapping
	line, source, originalLine, originalColumn, name := 1, 0, 0, 0, 0
	for row := range strings.SplitSeq(m.Mappings, ";") {
		column := 0
		for segment := range strings.SplitSeq(row, ",") {
			if segment == "" {
				continue
			}
			var values [5]int
			count := 0
			for segment != "" {
				if count == len(values) {
					return nil, fmt.Errorf("source map segment has more than five fields")
				}
				value, remaining, err := readVLQ(segment)
				if err != nil {
					return nil, err
				}
				values[count], segment = value, remaining
				count++
			}
			if count != 1 && count != 4 && count != 5 {
				return nil, fmt.Errorf("source map segment has %d fields", count)
			}
			column += values[0]
			if column < 0 {
				return nil, fmt.Errorf("negative generated source map column")
			}
			mapping := decodedMapping{Generated: sourcePosition{line, column}}
			if count > 1 {
				source += values[1]
				originalLine += values[2]
				originalColumn += values[3]
				if source < 0 || source >= len(sources) || originalLine < 0 || originalColumn < 0 {
					return nil, fmt.Errorf("invalid original source map position")
				}
				mapping.Original = &originalPosition{sources[source], sourcePosition{originalLine + 1, originalColumn}}
			}
			if count == 5 {
				name += values[4]
				if name < 0 || name >= len(m.Names) {
					return nil, fmt.Errorf("invalid source map name index")
				}
				mapping.Name = &m.Names[name]
			}
			result = append(result, mapping)
		}
		line++
	}
	return result, nil
}

func readVLQ(input string) (int, string, error) {
	var value uint64
	var shift uint
	for i := 0; i < len(input); i++ {
		digit := strings.IndexByte(base64Digits, input[i])
		if digit < 0 || shift >= 60 {
			return 0, "", fmt.Errorf("invalid source map VLQ")
		}
		value |= uint64(digit&31) << shift
		if digit&32 == 0 {
			v := int(value >> 1)
			if value&1 != 0 {
				v = -v
			}
			return v, input[i+1:], nil
		}
		shift += 5
	}
	return 0, "", fmt.Errorf("unterminated source map VLQ")
}

type visualPosition = [2]int
type visualRange struct {
	Start, End visualPosition
}
type visualOriginal struct {
	Source string
	visualRange
}
type visualizationRange struct {
	Original  visualOriginal
	Generated *visualRange
}

func visualizeSourceMap(m *SourceMap, points []decodedMapping, css string) string {
	sources := map[string]string{}
	for i, name := range m.Sources {
		if i < len(m.SourcesContent) {
			sources[name] = m.SourcesContent[i]
		}
	}
	var annotations []visualizationRange
	var current *visualizationRange
	finish := func(next *decodedMapping) {
		if current == nil {
			return
		}
		g := current.Generated
		if g.Start == g.End {
			if next == nil || next.Generated.Line > g.Start[0] {
				g.End = visualPosition{g.Start[0] + 1, 0}
			}
			if next != nil && *next.Original.Source.URL == current.Original.Source && next.Generated == (sourcePosition{g.Start[0] + 1, 0}) && next.Original.sourcePosition == (sourcePosition{current.Original.Start[0] + 1, 0}) {
				current.Original.End = visualPosition{next.Original.Line, next.Original.Column}
			}
		}
		annotations = append(annotations, *current)
		current = nil
	}
	for _, point := range points {
		if point.Original == nil {
			continue
		}
		original := visualPosition{point.Original.Line, point.Original.Column}
		generated := visualPosition{point.Generated.Line, point.Generated.Column}
		if current != nil && current.Original.Source == *point.Original.Source.URL && current.Generated.Start[0] == generated[0] && generated[1] > current.Generated.End[1] && comparePosition(sourcePosition{current.Original.Start[0], current.Original.Start[1]}, point.Original.sourcePosition) <= 0 {
			current.Generated.End, current.Original.End = generated, original
			continue
		}
		finish(&point)
		current = &visualizationRange{visualOriginal{*point.Original.Source.URL, visualRange{original, original}}, &visualRange{generated, generated}}
	}
	finish(nil)
	return visualizeSourceMapRanges(sources, css, annotations)
}

func visualizeSourceMapRanges(sources map[string]string, generated string, annotations []visualizationRange) string {
	lines := strings.Split(generated, "\n")
	sourceLines := map[string][]string{}
	generatedWidth, sourceWidth := len(strconv.Itoa(len(lines))), 1
	for name, content := range sources {
		sourceLines[name] = strings.Split(content, "\n")
		sourceWidth = max(sourceWidth, len(strconv.Itoa(len(sourceLines[name]))))
	}
	labels := map[string]string{}
	sourceSet := map[string]bool{}
	leftWidth := visualLength(withLineNumber(generatedWidth, "output.css", 0))
	for _, a := range annotations {
		key := visualSourceKey(a.Original)
		if _, exists := labels[key]; !exists {
			labels[key] = annotationLabel(len(labels))
		}
		label := labels[key]
		sourceSet[a.Original.Source] = true
		if a.Generated == nil {
			leftWidth = max(leftWidth, visualLength(withoutLineNumber(generatedWidth, "unmapped "+label)))
			continue
		}
		g := *a.Generated
		line := visualLine(lines, g.Start[0])
		leftWidth = max(leftWidth, visualLength(withLineNumber(generatedWidth, line, g.Start[0])), visualLength(withoutLineNumber(generatedWidth, visualizeRange(g, label, line))))
		for _, line := range visualizeContinuation(lines, generatedWidth, g, label) {
			leftWidth = max(leftWidth, visualLength(line))
		}
	}
	leftWidth = min(100, leftWidth)
	var out strings.Builder
	out.WriteByte('\n')
	add := func(left, right string) {
		out.WriteString(fitColumn(left, leftWidth, true))
		out.WriteString(" | ")
		out.WriteString(fitColumn(right, 100, false))
		out.WriteByte('\n')
	}
	header := "original"
	if len(sourceSet) == 1 {
		header = annotations[0].Original.Source
	}
	add(withLineNumber(generatedWidth, "output.css", 0), withLineNumber(sourceWidth, header, 0))
	add("", "")
	lastLine, lastSource := 0, ""
	lastSourceLine := map[string]int{}
	rendered := map[string]bool{}
	for _, a := range annotations {
		label := labels[visualSourceKey(a.Original)]
		if a.Generated != nil {
			for i := lastLine + 1; i < a.Generated.Start[0]; i++ {
				add(withLineNumber(generatedWidth, visualLine(lines, i), i), "")
			}
		}
		before, line, marker, after := visualizeOriginal(sourceLines, lastSourceLine, rendered, sourceWidth, a.Original, label)
		if len(sourceSet) > 1 && line != "" && a.Original.Source != lastSource {
			add("", withoutLineNumber(sourceWidth, "--- "+a.Original.Source+" ---"))
			lastSource = a.Original.Source
		}
		for _, line := range before {
			add("", line)
		}
		if a.Generated == nil {
			add(withoutLineNumber(generatedWidth, "unmapped "+label), line)
			add("", marker)
			for _, line := range after {
				add("", line)
			}
			continue
		}
		g := *a.Generated
		text := visualLine(lines, g.Start[0])
		add(withLineNumber(generatedWidth, text, g.Start[0]), line)
		add(withoutLineNumber(generatedWidth, visualizeRange(g, label, text)), marker)
		generatedAfter := visualizeContinuation(lines, generatedWidth, g, label)
		for i := 0; i < max(len(after), len(generatedAfter)); i++ {
			add(visualLine(generatedAfter, i+1), visualLine(after, i+1))
		}
		lastLine = coveredEndLine(g)
	}
	for i := lastLine + 1; i <= len(lines); i++ {
		add(withLineNumber(generatedWidth, lines[i-1], i), "")
	}
	return out.String()
}

func visualLine(lines []string, line int) string {
	if line < 1 || line > len(lines) {
		return ""
	}
	return lines[line-1]
}
func visualLength(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}
func visualPrefix(s string, n int) string {
	for i, r := range s {
		if n <= 0 {
			return s[:i]
		}
		n--
		if r > 0xffff {
			n--
			if n < 0 {
				return s[:i] + string(utf8.RuneError)
			}
		}
	}
	return s
}
func withLineNumber(width int, value string, line int) string {
	label := ""
	if line != 0 {
		label = strconv.Itoa(line)
	}
	return strings.Repeat(" ", max(0, width-len(label))) + label + "  " + value
}
func withoutLineNumber(width int, value string) string {
	if value == "" {
		return ""
	}
	return withLineNumber(width, value, 0)
}
func fitColumn(value string, width int, pad bool) string {
	if length := visualLength(value); length <= width {
		if pad {
			return value + strings.Repeat(" ", width-length)
		}
		return value
	}
	if marker := strings.Index(value, " @ "); marker >= 0 {
		label := strings.LastIndexByte(value[:marker], ' ') + 1
		suffix := value[label:]
		prefixWidth := width - visualLength(suffix) - 4
		if prefixWidth > 0 {
			return fitColumn(visualPrefix(value, prefixWidth)+"... "+suffix, width, pad)
		}
	}
	return visualPrefix(value, max(0, width-3)) + "..."
}
func annotationLabel(index int) string {
	var label string
	for index >= 0 {
		label = string(rune('A'+index%26)) + label
		index = index/26 - 1
	}
	return label
}
func visualizeRange(r visualRange, label, line string) string {
	width := r.End[1] - r.Start[1]
	if r.Start[0] != r.End[0] {
		width = visualLength(line) - r.Start[1]
	}
	return strings.Repeat(" ", max(0, r.Start[1])) + strings.Repeat("^", max(1, width)) + " " + label + " @ " + formatVisualRange(r)
}
func visualizeOriginal(sources map[string][]string, last map[string]int, rendered map[string]bool, width int, original visualOriginal, label string) (before []string, line, marker string, after []string) {
	lines := sources[original.Source]
	r := original.visualRange
	start := r.Start[0]
	if start < 1 || start > len(lines) {
		return
	}
	lastLine := last[original.Source]
	key := visualSourceKey(original)
	if rendered[key] {
		last[original.Source] = max(lastLine, start)
		return
	}
	if lastLine > 0 && start > lastLine && start-lastLine <= 4 {
		for i := lastLine + 1; i < start; i++ {
			if strings.TrimSpace(lines[i-1]) != "" {
				before = append(before, withLineNumber(width, lines[i-1], i))
			}
		}
	}
	rendered[key] = true
	end := coveredEndLine(r)
	after = visualizeContinuation(lines, width, r, label)
	if start > lastLine {
		for i := end + 1; i <= min(end+3, len(lines)); i++ {
			if strings.TrimSpace(lines[i-1]) != "}" {
				break
			}
			after = append(after, withLineNumber(width, lines[i-1], i))
			end = i
		}
	}
	last[original.Source] = max(lastLine, end)
	return before, withLineNumber(width, lines[start-1], start), withoutLineNumber(width, visualizeRange(r, label, lines[start-1])), after
}
func visualizeContinuation(lines []string, width int, r visualRange, label string) []string {
	var result []string
	for i := r.Start[0] + 1; i <= coveredEndLine(r); i++ {
		line := visualLine(lines, i)
		end := visualLength(line)
		if i == r.End[0] {
			end = r.End[1]
		}
		result = append(result, withLineNumber(width, line, i), withoutLineNumber(width, strings.Repeat("^", max(1, end))+" "+label))
	}
	return result
}
func coveredEndLine(r visualRange) int {
	if r.Start[0] != r.End[0] && r.End[1] == 0 {
		return r.End[0] - 1
	}
	return r.End[0]
}
func visualSourceKey(o visualOriginal) string {
	return fmt.Sprintf("%s:%d:%d:%d:%d", o.Source, o.Start[0], o.Start[1], o.End[0], o.End[1])
}
func formatVisualRange(r visualRange) string {
	if r.Start[0] == r.End[0] {
		if r.Start[1] == r.End[1] {
			return fmt.Sprintf("%d:%d", r.Start[0], r.Start[1])
		}
		return fmt.Sprintf("%d:%d-%d", r.Start[0], r.Start[1], r.End[1])
	}
	return fmt.Sprintf("%d:%d-%d:%d", r.Start[0], r.Start[1], r.End[0], r.End[1])
}
