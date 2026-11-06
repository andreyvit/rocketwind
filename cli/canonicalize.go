package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/andreyvit/rocketwind"
)

const canonicalizeUsage = `Rocketwind — Tailwind CSS 4.3.4 compiler for Go

Usage: rocketwind canonicalize [options] [class lists...]
       rocketwind canonicalize --stream [options]

      --css FILE       Theme stylesheet (default: @import "tailwindcss")
      --format FORMAT  text, json, or jsonl (default: text)
      --stream         Process each stdin line as it arrives
      --cwd DIR        Base directory (default: current directory)
  -h, --help           Show this help

Quote each class list to process it as one group. Without class lists,
read groups from stdin. Unknown classes are kept and sorted first.
`

type canonicalizeFlags struct {
	css, cwd, format string
	stream, help     bool
	inputs           []string
}

type candidateGroupResult struct {
	Input   string `json:"input"`
	Output  string `json:"output"`
	Changed bool   `json:"changed"`
}

func canonicalizeCommand(ctx context.Context, args []string, options Options) int {
	f, err := parseCanonicalizeFlags(args)
	if err != nil {
		fmt.Fprintln(options.Stderr, err)
		return 1
	}
	if f.help || len(args) == 0 && (options.StdoutIsTTY || terminalFile(options.Stdout)) {
		fmt.Fprint(options.Stdout, canonicalizeUsage)
		return 0
	}
	if err := runCanonicalize(ctx, f, options); err != nil {
		fmt.Fprintln(options.Stderr, err)
		return 1
	}
	return 0
}

func parseCanonicalizeFlags(args []string) (canonicalizeFlags, error) {
	raw, positional := rawArguments(args)
	values := typedArguments(map[string]argDefinition{
		"--css":    {Type: "string"},
		"--cwd":    {Type: "string", Default: ".", HasDefault: true},
		"--format": {Type: "string", Default: "text", HasDefault: true},
		"--stream": {Type: "boolean"},
		"--help":   {Type: "boolean", Alias: "-h"},
	}, raw, positional)
	f := canonicalizeFlags{inputs: positional}
	f.css, _ = values["--css"].(string)
	f.cwd, _ = values["--cwd"].(string)
	f.format, _ = values["--format"].(string)
	f.stream, f.help = values["--stream"] == true, values["--help"] == true
	if f.format != "text" && f.format != "json" && f.format != "jsonl" {
		return f, fmt.Errorf("Invalid value for --format: %s", f.format)
	}
	return f, nil
}

func terminalFile(value any) bool {
	f, ok := value.(*os.File)
	if !ok {
		return false
	}
	return terminalDescriptor(f.Fd())
}

func runCanonicalize(ctx context.Context, f canonicalizeFlags, options Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	inputs := f.inputs
	if !f.stream && len(inputs) == 0 && !options.StdinIsTTY && !terminalFile(options.Stdin) {
		data, err := io.ReadAll(options.Stdin)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				inputs = append(inputs, line)
			}
		}
	}
	if !f.stream && len(inputs) == 0 {
		fmt.Fprint(options.Stdout, canonicalizeUsage)
		return fmt.Errorf("No candidate groups provided")
	}
	base, err := filepath.Abs(f.cwd)
	if err != nil {
		return err
	}
	input := `@import "tailwindcss";`
	if f.css != "" {
		file := f.css
		if !filepath.IsAbs(file) {
			file = filepath.Join(base, file)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		input = string(data)
		base = filepath.Dir(file)
	}
	fs := rocketwind.Filesystem{Root: base, Modules: options.Modules}
	c, err := rocketwind.Compile(input, rocketwind.Config{Base: base, LoadStylesheet: fs.LoadStylesheet, LoadModule: fs.LoadModule})
	if err != nil {
		return err
	}
	k := c.NewCanonicalizer(rocketwind.CanonicalizeOptions{Collapse: true, LogicalToPhysical: true})
	process := func(input string) (candidateGroupResult, error) {
		if err := ctx.Err(); err != nil {
			return candidateGroupResult{}, err
		}
		var classes []string
		for class := range rocketwind.Segments(strings.TrimSpace(input), ' ') {
			if class = strings.TrimSpace(class); class != "" {
				classes = append(classes, class)
			}
		}
		classes, err := c.SortClasses(k.Canonicalize(classes))
		if err != nil {
			return candidateGroupResult{}, err
		}
		output := strings.Join(classes, " ")
		return candidateGroupResult{Input: input, Output: output, Changed: input != output}, nil
	}
	if f.stream {
		return streamCanonicalize(ctx, options.Stdin, options.Stdout, f.format, process)
	}
	results := make([]candidateGroupResult, 0, len(inputs))
	for _, input := range inputs {
		result, err := process(input)
		if err != nil {
			return err
		}
		results = append(results, result)
	}
	output := formatCandidateResults(results, f.format)
	if output != "" {
		output += "\n"
	}
	_, err = io.WriteString(options.Stdout, output)
	return err
}

func formatCandidateResults(results []candidateGroupResult, format string) string {
	if format == "json" {
		return canonicalJSON(results, true)
	}
	parts := make([]string, len(results))
	for i, result := range results {
		if format == "jsonl" {
			parts[i] = canonicalJSON(result, false)
		} else {
			parts[i] = result.Output
		}
	}
	return strings.Join(parts, "\n")
}

func canonicalJSON(value any, indent bool) string {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if indent {
		encoder.SetIndent("", "  ")
	}
	// Values are fixed structs of strings and booleans, or slices of those structs.
	_ = encoder.Encode(value)
	return strings.TrimSuffix(output.String(), "\n")
}

func streamCanonicalize(ctx context.Context, input io.Reader, output io.Writer, format string, process func(string) (candidateGroupResult, error)) error {
	write := func(value string) error { _, err := io.WriteString(output, value); return err }
	first := true
	if format == "json" {
		if err := write("["); err != nil {
			return err
		}
	}
	lines := make(chan string)
	finished := make(chan error, 1)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), int(^uint(0)>>1))
		scanner.Split(canonicalLine)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-stop:
				return
			}
		}
		finished <- scanner.Err()
	}()
read:
	for {
		var line string
		select {
		case <-ctx.Done():
			return ctx.Err()
		case value, ok := <-lines:
			if !ok {
				break read
			}
			line = value
		}
		result, err := process(line)
		if err != nil {
			return err
		}
		value := result.Output + "\n"
		if format == "jsonl" {
			value = canonicalJSON(result, false) + "\n"
		} else if format == "json" {
			value = "\n"
			if !first {
				value = ",\n"
			}
			value += "  " + strings.ReplaceAll(canonicalJSON(result, true), "\n", "\n  ")
		}
		if err := write(value); err != nil {
			return err
		}
		first = false
	}
	if err := <-finished; err != nil {
		return err
	}
	if format == "json" {
		if first {
			return write("]")
		}
		return write("\n]")
	}
	return nil
}

func canonicalLine(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		advance = i + 1
		if data[i] == '\r' {
			if advance == len(data) && !atEOF {
				return 0, nil, nil
			}
			if advance < len(data) && data[advance] == '\n' {
				advance++
			}
		}
		return advance, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
