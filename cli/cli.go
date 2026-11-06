// Package cli implements Rocketwind's command-line compiler. Applications can
// call Run with registered Go modules to build a compiler with their plugins.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andreyvit/rocketwind"
)

type Options struct {
	Stdin                   io.Reader
	Stdout, Stderr          io.Writer
	Modules                 map[string]rocketwind.Module
	StdinIsTTY, StdoutIsTTY bool
	Columns                 int
}

type flags struct {
	input, output, cwd, config, watch string
	sourceMap                         string
	inlineMap                         bool
	content, exclude, classes         []string
	poll                              time.Duration
	silent, help, version             bool
	minify, optimize                  bool
}

const usage = `Rocketwind — Tailwind CSS 4.3.4 compiler for Go

Usage: rocketwind [build] [options]

  -i, --input FILE     Input CSS; use - for stdin
  -o, --output FILE    Output CSS; defaults to stdout
  -w, --watch         Rebuild on changes; --watch=always survives stdin closing
      --poll[=MS]     Poll interval in milliseconds (default: 250)
      --cwd DIR       Source search directory (default: current directory)
      --config FILE   JSON configuration
      --content GLOB  Explicit source pattern; may be repeated
      --exclude FILE  Omit helpers already defined in this CSS; may be repeated
      --classes LIST  Additional complete class names; may be repeated
  -m, --minify         Optimize and compact the output
      --optimize       Optimize while keeping readable output
      --map[=FILE]    Generate an inline source map, or write one to FILE
      --silent        Suppress progress messages
  -h, --help          Show this help
      --version       Show supported Tailwind version

Without --input, Rocketwind compiles @import "tailwindcss".
JavaScript plugins and configuration require Go implementations.
`

// Run blocks until compilation ends or the context is canceled. It returns a
// process exit status and writes diagnostics to Stderr, keeping CSS on Stdout.
func Run(ctx context.Context, args []string, options Options) int {
	if options.Stdin == nil {
		options.Stdin = os.Stdin
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	if len(args) > 0 && args[0] == "canonicalize" {
		return canonicalizeCommand(ctx, args[1:], options)
	}
	f, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(options.Stderr, err)
		return 1
	}
	if f.help || (len(args) == 0 || len(args) == 1 && args[0] == "build") && (options.StdoutIsTTY || terminalFile(options.Stdout)) {
		fmt.Fprint(options.Stdout, usage)
		return 0
	}
	if f.version {
		fmt.Fprintln(options.Stdout, "Rocketwind (Tailwind CSS 4.3.4)")
		return 0
	}
	if err := build(ctx, f, options); err != nil {
		fmt.Fprintln(options.Stderr, formatError(err, options.Columns))
		return 1
	}
	return 0
}

func parseFlags(args []string) (flags, error) {
	f := flags{output: "-", cwd: ".", poll: 250 * time.Millisecond}
	explicitBuild := len(args) > 0 && args[0] == "build"
	if explicitBuild {
		args = args[1:]
	}
	raw, positional := rawArguments(args)
	values := typedArguments(map[string]argDefinition{
		"--input":    {Type: "string", Alias: "-i"},
		"--output":   {Type: "string", Alias: "-o", Default: "-", HasDefault: true},
		"--cwd":      {Type: "string", Default: ".", HasDefault: true},
		"--watch":    {Type: "boolean | string", Alias: "-w"},
		"--poll":     {Type: "boolean | number", Default: false, HasDefault: true},
		"--map":      {Type: "boolean | string", Default: false, HasDefault: true},
		"--silent":   {Type: "boolean"},
		"--help":     {Type: "boolean", Alias: "-h"},
		"--version":  {Type: "boolean"},
		"--config":   {Type: "string"},
		"--minify":   {Type: "boolean", Alias: "-m"},
		"--optimize": {Type: "boolean"},
	}, raw, positional)
	str := func(key string) string { value, _ := values[key].(string); return value }
	enabled := func(key string) bool { return values[key] == true }
	f.input, f.output, f.cwd, f.config = str("--input"), str("--output"), str("--cwd"), str("--config")
	f.silent, f.help, f.version = enabled("--silent"), enabled("--help"), enabled("--version")
	f.minify, f.optimize = enabled("--minify"), enabled("--optimize")
	if !explicitBuild && len(args) > 0 && !strings.HasPrefix(args[0], "-") && !f.help {
		return f, fmt.Errorf("invalid command %q; use --help", args[0])
	}
	if watch := values["--watch"]; watch == true || str("--watch") != "" {
		f.watch = "stdin"
		if watch == "always" {
			f.watch = "always"
		}
	}
	switch poll := values["--poll"].(type) {
	case bool: // The Go watcher polls in either mode.
	case float64:
		if poll <= 0 || math.IsInf(poll, 0) || poll >= float64(math.MaxInt64)/float64(time.Millisecond) {
			return f, errors.New("Specified polling interval must be a positive number.")
		}
		f.poll = time.Duration(poll * float64(time.Millisecond))
	default:
		return f, errors.New("Use --poll with a non-zero value in milliseconds.")
	}
	f.inlineMap = enabled("--map")
	f.sourceMap = str("--map")
	if f.sourceMap == "-" {
		return f, errors.New("Use --map without a value to inline the source map.")
	}
	for _, value := range raw["content"] {
		for pattern := range rocketwind.Segments(argumentString(value), ',') {
			f.content = append(f.content, pattern)
		}
	}
	for _, value := range raw["exclude"] {
		f.exclude = append(f.exclude, argumentString(value))
	}
	for _, value := range raw["classes"] {
		f.classes = append(f.classes, strings.Fields(argumentString(value))...)
	}
	return f, nil
}

type stamp struct {
	time   time.Time
	size   int64
	exists bool
}

func fileStamp(name string) stamp {
	info, err := os.Stat(name)
	if err != nil {
		return stamp{}
	}
	return stamp{info.ModTime(), info.Size(), true}
}

func build(ctx context.Context, f flags, options Options) error {
	base, err := filepath.Abs(f.cwd)
	if err != nil {
		return err
	}
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		return fmt.Errorf("source directory %q does not exist", base)
	}
	resolve := func(name string) string {
		if name == "" || name == "-" || filepath.IsAbs(name) {
			return name
		}
		return filepath.Join(base, name)
	}
	f.input, f.output, f.config = resolve(f.input), resolve(f.output), resolve(f.config)
	f.sourceMap = resolve(f.sourceMap)
	for i := range f.exclude {
		f.exclude[i] = resolve(f.exclude[i])
	}
	if f.sourceMap != "" {
		for _, name := range append([]string{f.input, f.output, f.config}, f.exclude...) {
			if samePath(f.sourceMap, name) {
				return fmt.Errorf("source map file %q must differ from CSS and configuration files", f.sourceMap)
			}
		}
	}
	if f.input != "" && f.input != "-" {
		info, err := os.Stat(f.input)
		if err != nil {
			return fmt.Errorf("Specified input file `%s` does not exist.", relativePath(f.input, base, true))
		}
		outInfo, _ := os.Stat(f.output)
		if f.input == f.output || outInfo != nil && os.SameFile(info, outInfo) {
			return fmt.Errorf("Specified input file `%s` and output file `%s` are identical.", relativePath(f.input, base, true), relativePath(f.output, base, true))
		}
	}
	input := `@import "tailwindcss";`
	if f.input == "-" {
		data, err := io.ReadAll(options.Stdin)
		if err != nil {
			return err
		}
		input = string(data)
	}
	var builder *rocketwind.Builder
	var scanner *rocketwind.Scanner
	dependencies := map[string]stamp{}
	var lastCSS, lastMap string
	written := false
	optimizer := optimizationCache{options: rocketwind.OptimizeOptions{Minify: f.minify, From: f.output}}
	compile := func() error {
		deps := map[string]stamp{}
		dependency := func(path string) { deps[path] = fileStamp(path) }
		entryBase := base
		text := input
		if f.input != "" && f.input != "-" {
			dependency(f.input)
			data, err := os.ReadFile(f.input)
			if err != nil {
				return err
			}
			text = string(data)
			entryBase = filepath.Dir(f.input)
		}
		if f.config != "" {
			text = "@config " + strconv.Quote(f.config) + ";\n" + text
		}
		var existing []string
		for _, name := range f.exclude {
			dependency(name)
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			existing = append(existing, string(data))
		}
		classes, err := rocketwind.DefinedClasses(existing...)
		if err != nil {
			return err
		}
		fs := rocketwind.Filesystem{Root: entryBase, Modules: options.Modules, OnDependency: dependency}
		from := ""
		if f.inlineMap || f.sourceMap != "" {
			from = f.input
			if from == "" || from == "-" {
				from = "<stdin>"
			}
		}
		compiled, err := rocketwind.Compile(text, rocketwind.Config{From: from, Base: entryBase, LoadStylesheet: fs.LoadStylesheet, LoadModule: fs.LoadModule, ExistingClasses: classes})
		if err != nil {
			return err
		}
		for name := range deps {
			if samePath(f.sourceMap, name) {
				return fmt.Errorf("source map file %q would overwrite an input dependency", f.sourceMap)
			}
		}
		var sources []rocketwind.Source
		root := compiled.Root()
		if len(f.content) > 0 {
			for _, pattern := range f.content {
				sources = append(sources, rocketwind.Source{Base: base, Pattern: pattern})
			}
		} else if root == nil {
			sources = append(sources, rocketwind.Source{Base: base, Pattern: "**/*"})
		} else if !root.Disabled {
			// Tailwind reports a missing automatic root instead of silently emitting no helpers.
			static := root.Pattern
			if at := strings.IndexAny(static, "*{"); at >= 0 {
				static = static[:strings.LastIndexAny(static[:at], "/\\")+1]
			}
			if !filepath.IsAbs(static) {
				static = filepath.Join(root.Base, static)
			}
			if _, err := os.Stat(static); err != nil {
				return fmt.Errorf("source path %q does not exist", static)
			}
			sources = append(sources, rocketwind.Source{Base: root.Base, Pattern: root.Pattern})
		}
		sources = append(sources, compiled.Sources()...)
		for _, output := range []string{f.output, f.sourceMap} {
			if output != "" && output != "-" {
				sources = append(sources, rocketwind.Source{Base: filepath.Dir(output), Pattern: filepath.Base(output), Negated: true})
			}
		}
		if f.input != "" && f.input != "-" {
			sources = append(sources, rocketwind.Source{Base: filepath.Dir(f.input), Pattern: filepath.Base(f.input)})
		}
		if executable, err := os.Executable(); err == nil {
			sources = append(sources, rocketwind.Source{Base: filepath.Dir(executable), Pattern: filepath.Base(executable), Negated: true})
		}
		builder = compiled.NewBuilder()
		scanner = rocketwind.NewScanner(sources)
		dependencies = deps
		return nil
	}
	emit := func() error {
		start := time.Now()
		classes := append(scanner.Scan(), f.classes...)
		var css, rawMap string
		var err error
		if f.inlineMap || f.sourceMap != "" {
			var sourceMap *rocketwind.SourceMap
			css, sourceMap, err = builder.BuildWithSourceMap(classes)
			if err == nil && (f.minify || f.optimize) {
				css, sourceMap, err = optimizer.run(css, sourceMap)
			}
			if err == nil {
				if f.inlineMap {
					css += "\n" + sourceMap.Inline()
				} else {
					data, _ := json.Marshal(sourceMap)
					rawMap = string(data)
					outputBase := base
					if f.output != "-" {
						outputBase = filepath.Dir(f.output)
					}
					var url string
					url, err = filepath.Rel(outputBase, f.sourceMap)
					css += "\n" + sourceMap.Comment(filepath.ToSlash(url))
				}
			}
		} else {
			css, err = builder.Build(classes)
			if err == nil && (f.minify || f.optimize) {
				css, _, err = optimizer.run(css, nil)
			}
		}
		if err != nil {
			return err
		}
		if written && css == lastCSS && rawMap == lastMap {
			return nil
		}
		if f.sourceMap != "" && (!written || rawMap != lastMap) {
			if err := outputFile(f.sourceMap, rawMap); err != nil {
				return err
			}
		}
		if !written || css != lastCSS {
			if f.output == "-" {
				_, err = io.WriteString(options.Stdout, css)
			} else {
				err = outputFile(f.output, css)
			}
		}
		if err != nil {
			return err
		}
		lastCSS, lastMap = css, rawMap
		written = true
		if !f.silent {
			fmt.Fprintf(options.Stderr, "Done in %s\n", formatNanoseconds(time.Since(start).Nanoseconds()))
		}
		return nil
	}
	if err := compile(); err != nil {
		return err
	}
	if err := emit(); err != nil {
		return err
	}
	if f.watch == "" {
		return nil
	}
	var closed <-chan struct{}
	if f.watch != "always" {
		done := make(chan struct{})
		closed = done
		go func() { io.Copy(io.Discard, options.Stdin); close(done) }()
	}
	ticker := time.NewTicker(f.poll)
	defer ticker.Stop()
	lastError := ""
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-closed:
			return nil
		case <-ticker.C:
			var err error
			for name, old := range dependencies {
				if fileStamp(name) != old {
					err = compile()
					break
				}
			}
			if err == nil {
				err = emit()
			}
			if err != nil {
				if err.Error() != lastError {
					fmt.Fprintln(options.Stderr, err)
					lastError = err.Error()
				}
			} else {
				lastError = ""
			}
		}
	}
}

func samePath(a, b string) bool {
	if a == "" || a == "-" || b == "" || b == "-" {
		return false
	}
	if a == b {
		return true
	}
	ai, ae := os.Stat(a)
	bi, be := os.Stat(b)
	return ae == nil && be == nil && os.SameFile(ai, bi)
}

func outputFile(name, css string) error {
	if info, err := os.Lstat(name); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if target, err := filepath.EvalSymlinks(name); err == nil {
			name = target
		} else {
			return os.WriteFile(name, []byte(css), 0644)
		}
	}
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".rocketwind-*.css")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	mode := os.FileMode(0644)
	if info, err := os.Stat(name); err == nil {
		mode = info.Mode().Perm()
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := io.WriteString(file, css); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), name)
}
