// Source mapping follows Tailwind CSS v4.3.4 under the MIT license.
package rocketwind

import (
	"cmp"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SourceMap is an ECMA-426 source map. Marshal it with encoding/json to produce
// a .map file. Columns count UTF-16 code units, as required for CSS source maps.
type SourceMap struct {
	Version        int      `json:"version"`
	File           string   `json:"file,omitempty"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent,omitempty"`
	Names          []string `json:"names"`
	Mappings       string   `json:"mappings"`
}

// Comment returns a CSS comment linking to a source-map URL.
func (*SourceMap) Comment(url string) string { return "/*# sourceMappingURL=" + url + " */\n" }

// Inline returns a CSS comment containing this map as a base64 data URL.
func (m *SourceMap) Inline() string {
	data, _ := json.Marshal(m)
	return m.Comment("data:application/json;base64," + base64.StdEncoding.EncodeToString(data))
}

// BuildWithSourceMap generates CSS and its source map. Set Config.From when
// compiling to retain locations; stylesheet loaders should also return Path.
// Like Build, this method may run concurrently on the same Compiler.
func (c *Compiler) BuildWithSourceMap(candidates []string) (string, *SourceMap, error) {
	css, decoded, err := c.build(candidates, true)
	if err != nil {
		return css, nil, err
	}
	return css, decoded.encode(), nil
}

// BuildWithSourceMap adds candidates and returns the cumulative CSS and map.
// It can be mixed with Build. Unchanged builds reuse both cached results.
func (builder *Builder) BuildWithSourceMap(candidates []string) (string, *SourceMap, error) {
	css, err := builder.build(candidates, true)
	if err != nil {
		return css, nil, err
	}
	if builder.sourceMap == nil {
		b := buildContext{compiler: builder.compiler, used: map[string]bool{}, track: true, externalVariables: builder.externalVariables}
		if builder.compiler.hasInput && builder.compiler.features == 0 && builder.compiler.config.Compatibility != Tailwind3 {
			builder.sourceMap = builder.compiler.identitySourceMap().encode()
		} else {
			css = b.renderRules(builder.rules)
			builder.sourceMap = createSourceMap(css, b.mappings).encode()
		}
	}
	return css, builder.sourceMap, nil
}

type cssSource struct{ file, code string }
type cssLocation struct {
	source     *cssSource
	start, end int
}
type cssMapping struct {
	original   cssLocation
	start, end int
}

func location(n *Node) cssLocation { return cssLocation{n.source, n.Start, n.End} }
func withLocation(n *Node, loc cssLocation) *Node {
	n.source, n.Start, n.End = loc.source, loc.start, loc.end
	return n
}
func setLocations(nodes []*Node, loc cssLocation, override bool) {
	for _, n := range nodes {
		if override || n.source == nil {
			withLocation(n, loc)
		}
		setLocations(n.Nodes, loc, override)
	}
}

type sourcePosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}
type unicodeOffset struct{ end, delta int }
type lineTable struct {
	lines   []int
	unicode []unicodeOffset
}

func newLineTable(source string) lineTable {
	t := lineTable{lines: []int{0}}
	delta := 0
	for i := 0; i < len(source); {
		if source[i] == '\n' {
			t.lines = append(t.lines, i+1-delta)
		}
		if source[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(source[i:])
		i += size
		delta += size - 1
		if r > 0xffff {
			delta--
		}
		t.unicode = append(t.unicode, unicodeOffset{i, delta})
	}
	return t
}

// find takes a UTF-16 offset, matching Tailwind's internal line-table API.
func (t lineTable) find(offset int) sourcePosition {
	offset = max(0, offset)
	line, _ := slices.BinarySearch(t.lines, offset+1)
	return sourcePosition{line, offset - t.lines[line-1]}
}
func (t lineTable) bytePosition(offset int) sourcePosition {
	i, _ := slices.BinarySearchFunc(t.unicode, offset+1, func(a unicodeOffset, b int) int { return cmp.Compare(a.end, b) })
	if i > 0 {
		offset -= t.unicode[i-1].delta
	}
	return t.find(offset)
}
func (t lineTable) findOffset(pos sourcePosition) int {
	line := min(max(pos.Line-1, 0), len(t.lines)-1)
	a, z := t.lines[line], t.lines[line]
	if line+1 < len(t.lines) {
		z = t.lines[line+1]
	}
	return min(max(a+pos.Column, 0), z)
}

type decodedSource struct {
	URL     *string `json:"url"`
	Content *string `json:"content"`
	Ignore  bool    `json:"ignore"`
}
type originalPosition struct {
	Source *decodedSource `json:"source"`
	sourcePosition
}
type decodedMapping struct {
	Original  *originalPosition `json:"originalPosition"`
	Generated sourcePosition    `json:"generatedPosition"`
	Name      *string           `json:"name"`
}
type decodedSourceMap struct {
	File     *string          `json:"file"`
	Sources  []*decodedSource `json:"sources"`
	Mappings []decodedMapping `json:"mappings"`
}

func emptySourceMap() *decodedSourceMap {
	return &decodedSourceMap{Sources: []*decodedSource{}, Mappings: []decodedMapping{}}
}
func createSourceMap(css string, mappings []cssMapping) *decodedSourceMap {
	return createSourceMapFrom(&cssSource{code: css}, mappings)
}
func (c *Compiler) identitySourceMap() *decodedSourceMap {
	var mappings []cssMapping
	var source *cssSource
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if n.source != nil {
				source = n.source
				mappings = append(mappings, cssMapping{location(n), n.Start, n.End})
			}
			walk(n.Nodes)
		}
	}
	walk(c.input)
	return createSourceMapFrom(source, mappings)
}
func createSourceMapFrom(generated *cssSource, mappings []cssMapping) *decodedSourceMap {
	result := emptySourceMap()
	if len(mappings) == 0 {
		return result
	}
	tables := map[*cssSource]lineTable{}
	sources := map[*cssSource]*decodedSource{}
	add := func(src *cssSource) (lineTable, *decodedSource) {
		if table, exists := tables[src]; exists {
			return table, sources[src]
		}
		table := newLineTable(src.code)
		source := &decodedSource{Content: &src.code}
		if src.file != "" {
			source.URL = &src.file
		}
		tables[src], sources[src] = table, source
		result.Sources = append(result.Sources, source)
		return table, source
	}
	for _, m := range mappings {
		loc := m.original
		if loc.source == nil || loc.source.code == "" {
			continue
		}
		originalTable, source := add(loc.source)
		generatedTable, _ := add(generated)
		start := min(max(0, loc.start), len(loc.source.code))
		end := min(max(start, loc.end), len(loc.source.code))
		offset := start
		for line := range strings.SplitSeq(loc.source.code[start:end], "\n") {
			if strings.TrimSpace(line) != "" {
				result.Mappings = append(result.Mappings, decodedMapping{
					Original:  &originalPosition{source, originalTable.bytePosition(offset)},
					Generated: generatedTable.bytePosition(m.start),
				})
			}
			offset += len(line) + 1
		}
		result.Mappings = append(result.Mappings, decodedMapping{
			Original:  &originalPosition{source, originalTable.bytePosition(loc.end)},
			Generated: generatedTable.bytePosition(m.end),
		})
	}
	slices.SortStableFunc(result.Mappings, compareMappings)
	return result
}
func comparePosition(a, b sourcePosition) int {
	return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
}
func compareMappings(a, b decodedMapping) int {
	order := comparePosition(a.Generated, b.Generated)
	if order != 0 {
		return order
	}
	var ao, bo sourcePosition
	if a.Original != nil {
		ao = a.Original.sourcePosition
	}
	if b.Original != nil {
		bo = b.Original.sourcePosition
	}
	return comparePosition(ao, bo)
}

func (m *decodedSourceMap) encode() *SourceMap {
	result := &SourceMap{Version: 3, Sources: []string{}, SourcesContent: []string{}, Names: []string{}}
	if m.File != nil {
		result.File = *m.File
	}
	indexes := map[string]int{}
	unknown := map[*decodedSource]string{}
	var encoded strings.Builder
	line, column, sourceID, originalLine, originalColumn := 1, 0, 0, 0, 0
	var previous *decodedMapping
	for i := range m.Mappings {
		mapping := &m.Mappings[i]
		if previous != nil && mapping.Generated == previous.Generated && mapping.Original != nil && previous.Original != nil && *mapping.Original == *previous.Original {
			continue
		}
		if mapping.Generated.Line > line {
			encoded.WriteString(strings.Repeat(";", mapping.Generated.Line-line))
			line, column = mapping.Generated.Line, 0
		} else if previous != nil {
			encoded.WriteByte(',')
		}
		writeVLQ(&encoded, mapping.Generated.Column-column)
		column = mapping.Generated.Column
		if original := mapping.Original; original != nil {
			name, content := "", ""
			if original.Source.URL != nil {
				name = *original.Source.URL
			} else {
				name = unknown[original.Source]
				if name == "" {
					name = "<unknown " + strconv.Itoa(len(unknown)+1) + ">"
					unknown[original.Source] = name
				}
			}
			if original.Source.Content != nil {
				content = *original.Source.Content
			}
			id, exists := indexes[name]
			if !exists {
				id = len(result.Sources)
				indexes[name] = id
				result.Sources = append(result.Sources, name)
				result.SourcesContent = append(result.SourcesContent, content)
			} else {
				result.SourcesContent[id] = content
			}
			writeVLQ(&encoded, id-sourceID)
			writeVLQ(&encoded, original.Line-1-originalLine)
			writeVLQ(&encoded, original.Column-originalColumn)
			sourceID, originalLine, originalColumn = id, original.Line-1, original.Column
		}
		previous = mapping
	}
	result.Mappings = encoded.String()
	return result
}

const base64Digits = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func writeVLQ(out *strings.Builder, value int) {
	u := uint(value) << 1
	if value < 0 {
		u = uint(-value)<<1 | 1
	}
	for u >= 32 {
		out.WriteByte(base64Digits[u&31|32])
		u >>= 5
	}
	out.WriteByte(base64Digits[u])
}
