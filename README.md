# Rocketwind — Tailwind CSS in Go

[![Go reference](https://pkg.go.dev/badge/github.com/andreyvit/rocketwind.svg)](https://pkg.go.dev/github.com/andreyvit/rocketwind)
[![Zero dependencies](https://img.shields.io/badge/deps-zero-brightgreen)](go.mod)
[![Tailwind 3 & 4](https://img.shields.io/badge/Tailwind-3%20%26%204-38bdf8)](#compatibility)
[![100% coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](TESTING.md)

**A drop-in replacement for the Tailwind CSS compiler, written in Go.**
A standalone CLI and a library for generating CSS inside your app.


Why?
----

Tailwind is usually a build-time tool. We wanted the same utilities in a running
Go app, where templates and settings can change after deployment:

* **Compile CSS on app startup.** Scan your templates and scripts, compile the
  classes they use, and serve the result from memory. No separate frontend build
  step or generated CSS file on disk.
* **Compile CSS for each account.** Give each user or organization its own theme,
  pages and widgets. Generate its stylesheet when you load its configuration.
* **Recompile when settings change.** Let a page editor change a block's spacing,
  responsive layout or class presets. Blocks can declare their own typed
  parameters and use them in classes such as `min-{breakpoint}:flex-row`.
* **Use a self-contained Go dependency.** Keep your toolchain in Go, with fewer
  packages to trust and audit for supply-chain attacks. Rocketwind is one Go
  module with no third-party dependencies; the CLI is a standalone binary.

Rocketwind runs entirely inside your process, with no Node installation or
compiler subprocess. Compile when something changes; serve cached CSS afterward.
For regular frontend builds, the standalone CLI works in any project.


Features
--------

* **Tailwind v4 and v3.** Utilities, variants, CSS directives, themes and standard
  plugins. Targets **4.3.4**, with a **3.4.17** compatibility profile for older apps.
* **Zero dependencies.** Pure Go, using only the standard library.
* **Faster than the official compiler.** **2.2×** for v4 and **10.7×** for v3 on
  Apple Silicon; **1.2×** and **5.3×** on AMD EPYC. [Benchmarks](#performance).
* **A complete CLI.** Build, watch, minify, scan classes and generate source maps.
* **A Go API.** Configure in Go or CSS. Generate utility stylesheets or named
  component classes. Read from disk, embedded assets or your own storage.
* **Typed, local parameters.** Resolve settings in your app, inline other class
  lists, and preserve their scope. [Example](#block-local-parameters-and-editable-settings).
* **Native Go plugins.** Forms, typography, aspect ratio, line clamp and container
  queries are included; custom plugins use Go callbacks.
* **Existing CSS stays useful.** Generate only the classes missing from your
  shared stylesheet.
* **100% statement coverage**, backed by Tailwind's own tests and independent
  comparisons against the official compilers.

JavaScript configuration needs a JSON or Go equivalent; custom JavaScript
plugins need a Go port. The standard plugins are already included. See
[compatibility](#compatibility) for exact versions and the v3 profile's limits.

[Usage](#usage) · [Compatibility](#compatibility) ·
[Advanced usage](#advanced-usage) · [Implementation details](#implementation-details)


Usage
-----

### On the command line

Install with Go **1.25 or later**, then replace your Tailwind build command:

```sh
go install github.com/andreyvit/rocketwind/cmd/rocketwind@latest
rocketwind -i src/input.css -o public/app.css --minify
rocketwind -i src/input.css -o public/app.css --watch
```

The resulting binary runs on its own. Your project does not need to use Go.
Tailwind 4 is the default; `src/input.css` can be as small as
`@import "tailwindcss";`. The CLI scans your project using Tailwind's source
discovery and ignore rules.

The CLI also supports stdin, stdout, source maps and exclusions.
See [CLI options](#cli-options).

### In Go

Install:

```sh
go get github.com/andreyvit/rocketwind@latest
```

Pass class names, get CSS:

```go
compiler := rocketwind.New(rocketwind.Config{})
css, err := compiler.Build([]string{"flex", "gap-4", "text-xl", "font-bold"})
```

`css` is a string. Store it, serve it, or append it to another stylesheet.
`New` supplies the default theme and generates utility CSS. To include
Tailwind's preflight reset or use a CSS entry point, use `Compile`:

```go
compiler, err := rocketwind.Compile(`@import "tailwindcss";`, rocketwind.Config{})
```

The theme, preflight and standard plugins are built in. There is nothing to
install with npm. The snippets below omit repetitive error checks; the complete
startup example includes them.

### Compile at startup, serve from memory

This is a complete server. CSS is compiled once, optimized for browsers, and
served at `/app.css`. Requests only write the cached string:

```go
package main

import (
    "io"
    "log"
    "net/http"

    "github.com/andreyvit/rocketwind"
)

func main() {
    compiler, err := rocketwind.Compile(`@import "tailwindcss";`, rocketwind.Config{})
    if err != nil {
        log.Fatal(err)
    }
    css, err := compiler.Build([]string{"flex", "gap-4", "text-xl", "font-bold"})
    if err != nil {
        log.Fatal(err)
    }
    css, err = rocketwind.Optimize(css, rocketwind.OptimizeOptions{})
    if err != nil {
        log.Fatal(err)
    }

    http.HandleFunc("/app.css", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/css; charset=utf-8")
        io.WriteString(w, css)
    })
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

In a real application, [scan your templates and scripts](#scanning-templates-and-scripts)
instead of writing the class list by hand. Embedded assets can be scanned at
startup too. Neither compilation nor serving requires a writable filesystem.


Advanced usage
--------------

### Configuration in Go or CSS

Use Go configuration without a CSS entry point:

```go
compiler := rocketwind.New(rocketwind.Config{
    Theme: map[string]string{"--color-brand": "#663399"},
})
css, err := compiler.Build([]string{"grid", "p-4", "text-xl", "bg-brand"})
```

Or pass a stylesheet to `Compile`. It supports `@theme`, `@utility`, `@apply`,
`@source`, `@plugin` and the other Tailwind directives. Use `Config.LoadStylesheet`
for imports and `Config.LoadModule` for configuration and Go plugins.

`rocketwind.Filesystem` supplies both loaders for disk files or any `fs.FS`,
including `embed.FS`. For an embedded `assets/app.css` with relative imports:

```go
files := rocketwind.Filesystem{FS: embeddedFiles, Root: "assets"}
sheet, err := files.LoadStylesheet("app.css", "assets")
compiler, err := rocketwind.Compile(sheet.Content, rocketwind.Config{
    From:           sheet.Path,
    Base:           sheet.Base,
    LoadStylesheet: files.LoadStylesheet,
    LoadModule:     files.LoadModule,
})
```

`Root` is the entry stylesheet's directory; it preserves relative asset URLs
across imports. Set `Compatibility: rocketwind.Tailwind3` on both the filesystem
loader and compiler when loading v3 configuration.

Object-style Tailwind configuration also has a Go equivalent:

```go
compiler, err := rocketwind.NewWithPlugins(rocketwind.Config{},
    rocketwind.Legacy(rocketwind.ConfigObject{
        {Key: "theme", Value: map[string]any{
            "extend": map[string]any{
                "colors": map[string]string{"brand": "#663399"},
            },
        }},
    }),
)
```

`Legacy` supports theme replacements and extensions, presets, breakpoints,
container settings, dark mode, prefixes and blocklists. Use `ConfigObject` when
field order matters; nested values can be ordinary maps. `ThemeFunc` resolves
references to other settings. This API follows v4 unless you select `Tailwind3`.

### Scanning templates and scripts

For source text:

```go
source := rocketwind.Preprocess(templateSource, "vue")
css, err := compiler.Build(rocketwind.Candidates(source))
```

For embedded or virtual filesystems:

```go
scanner := rocketwind.NewScanner(nil)
classes, err := scanner.ScanFS(embeddedViews, embeddedScripts)
css, err := compiler.Build(classes)
```

`ScanFS` reads frontend text assets and skips binary files, CSS, lockfiles and
dependency directories. CSS goes through the stylesheet loader. Set
`ScannerOptions{Compatibility: rocketwind.Tailwind3}` for v3 extraction.

If templates and JavaScript already ship in your binary, **scan them at runtime**.
For source code the binary does not contain, such as Go files, collect a manifest:

```sh
rocketwind scan -i assets/app.css --content '**/*.go' -o assets/classes.txt
```

Embed that sorted, unique, one-name-per-line file and combine it with runtime
results using `append(classes, strings.Split(classList, "\n")...)`. The manifest
needs regeneration only when those source files change. `scan --known` keeps
only classes recognized by the current configuration, including Go plugins;
plain `scan` retains unknown names for later configuration changes.

In development, scan frontend files on disk so template and script edits can
refresh CSS without rebuilding the binary or its source manifest:

```go
scanner := compiler.NewScanner(projectDirectory)
css, err := compiler.Build(scanner.Scan())
```

The scanner applies `@source`, automatic source roots, `.gitignore` and `.ignore`.
Use `NewScanner([]rocketwind.Source{...})` to supply paths directly.

As with Tailwind, class names must be complete: the scanner does not evaluate
`"text-" + color`. Supply those classes explicitly, or use typed settings below.
`Preprocess` handles Tailwind's template languages, including Vue, Svelte, Pug,
Haml, Slim, Ruby, Rust, Clojure and Elixir. HTML and GoX pass through unchanged.

### Block-local parameters and editable settings

Suppose you're building a page editor. A reward-card block declares its settings
alongside its template; every instance of that block stores its own values:

| Setting | Type | Controls |
| --- | --- | --- |
| `gap` | Length | Space between the image and text. |
| `breakpoint` | Length | When this card switches to its wide layout. |
| `wideLayout` | Class list | The layout to use above that breakpoint. |

The block's classes are `flex flex-col gap-{gap} min-{breakpoint}:{wideLayout}`.
Prepare them once using the block's setting declarations:

```go
types := map[string]rocketwind.ValueType{
    "gap":        rocketwind.TypeLength,
    "breakpoint": rocketwind.TypeLength,
    "wideLayout": rocketwind.TypeClasses,
}
card, err := compiler.Prepare("flex flex-col gap-{gap} min-{breakpoint}:{wideLayout}",
    rocketwind.TemplateOptions{
        TypeOf: func(name string) (rocketwind.ValueType, error) {
            return types[name], nil
        },
    })
```

When you load a page, resolve each instance's settings and generate named classes:

```go
row, err := compiler.Prepare("flex-row items-center", rocketwind.TemplateOptions{})
grid, err := compiler.Prepare("grid grid-cols-2", rocketwind.TemplateOptions{})

resolve := func(gap, breakpoint string, layout *rocketwind.Template) rocketwind.ResolveFunc {
    values := map[string]rocketwind.Value{
        "gap":        {CSS: gap},
        "breakpoint": {CSS: breakpoint},
        "wideLayout": {Classes: layout},
    }
    return func(p rocketwind.Parameter) (rocketwind.Value, error) {
        value, ok := values[p.Expression]
        if !ok {
            return rocketwind.Value{}, fmt.Errorf("unknown setting %q", p.Expression)
        }
        return value, nil
    }
}

css, err := compiler.Render([]rocketwind.Rule{
    {Selector: ".rewards-main", Template: card, Resolve: resolve("1.5rem", "48rem", row)},
    {Selector: ".rewards-sidebar", Template: card, Resolve: resolve("0.75rem", "64rem", grid)},
})
```

The main card becomes a row at 48rem; the sidebar card becomes a two-column grid
at 64rem. Editing either instance changes only its named rule. New block types
can bring their own settings without changing a global Tailwind configuration.

A CSS variable already handles a configurable gap well. The extra value here is
the breakpoint and the whole layout: [`var()` cannot supply a media-query
condition](https://www.w3.org/TR/css-variables-1/#using-variables), and a class-list
setting can add several declarations and variants. Your application can expose
a choice of presets or an editable class list.

Rocketwind treats parameter names as opaque strings. Your code owns the block
schema, editor, lookup, inheritance and validation. Types are available before
values: `text-{size}` with `TypeLength` means font size; `text-{color}` with
`TypeColor` means color, even when both resolve to CSS variables.

Values are literal CSS, preserving spaces and underscores. Use `Named: true`
for a theme token such as `xl`, or `Omit: true` to leave out an unset setting.
Parameters also work inside arbitrary values, such as `w-[calc(100%_-_{gap})]`.

A whole-class parameter such as `hover:{button.styles}` has type `TypeClasses`.
Return `Value{Classes: anotherTemplate}` to inline it. Its `Resolve` field can
provide a different resolver, preserving the scope where the classes were
defined. For example, a button preset can keep its own `color` setting when a
card inlines it. References inherit variants and `!`; cycles are errors.

`Render` shares theme variables and property registrations across the named
rules. For one selector, use `card.CSS(".rewards-main", resolve("1.5rem", "48rem", row))`.
Named rules preserve class-list order, so later declarations can override
earlier ones. `template.Nodes(resolve)` returns declarations and nested variants
for an existing CSS writer.

For older spellings, opt into `Config.Aliases`: for example,
`"family": {Utility: "font", Type: rocketwind.TypeFamilyName}` makes `family-*`
mean a font family. `TemplateOptions.DollarReferences` also accepts `$name`.
`template.UtilityRoots()` reports utility names after alias resolution.

### Standard and custom plugins

These are native Go implementations; no JavaScript runs during compilation:

| Plugin | Versions | Go constructor |
| --- | --- | --- |
| `@tailwindcss/forms` | 0.5.11; also 0.5.9 for existing apps | `Forms(FormsOptions{})` |
| `@tailwindcss/typography` | 0.5.20; also 0.5.15 for existing apps | `Typography(TypographyOptions{})` |
| `@tailwindcss/aspect-ratio` | 0.4.2 | `AspectRatio(AspectRatioOptions{})` |
| `@tailwindcss/line-clamp` | 0.4.4 | `LineClamp(LineClampOptions{})` |
| `@tailwindcss/container-queries` | 0.1.1 | `ContainerQueries(ContainerQueriesOptions{})` |

Use their normal `@plugin` names in CSS, list them in a JSON configuration's
`plugins`, or configure them directly in Go:

```go
compiler, err := rocketwind.NewWithPlugins(rocketwind.Config{},
    rocketwind.Forms(rocketwind.FormsOptions{Strategy: "class"}),
    rocketwind.Typography(rocketwind.TypographyOptions{}),
)
```

Forms supports `base` and `class` strategies. Typography supports custom class
names, prose modifiers, element variants and the legacy selector option;
`Styles` replaces defaults and `Extend` customizes them. Select older versions
with the options' `Version` field or module names such as
`@tailwindcss/forms@0.5.9`. Modern Tailwind already includes line-clamp and
container queries in core.

Custom Go callbacks use `PluginAPI` to register base styles, component selectors,
utilities and variants. Functional utilities can accept named values, types,
fractions, modifiers and negative values. Read theme scalars with `Theme`, or
objects and tuples with `ThemeValue`. `NewWithPlugins` runs callbacks while
constructing the compiler.

### Reusing existing CSS

Generate only helpers absent from your shared stylesheets:

```go
existing, err := rocketwind.DefinedClasses(mainCSS, widgetsCSS)
compiler := rocketwind.New(rocketwind.Config{ExistingClasses: existing})
css, err := compiler.Build(templateClasses)
```

Exclusions use complete class names from CSS selectors, including escaped names
and nested rules. Comments, declaration values, attributes, `:not()` and `:has()`
do not define classes. Pass imported stylesheets separately. Named rules and
`@apply` can still resolve excluded utilities.

Append account-specific CSS after the shared rules. The existing stylesheet
owns each excluded class's behavior, including when mixing v3 and v4. Avoid a
second preflight or global forms reset. If the combined CSS moves to another
route, `ResolveURLs(css, originalStylesheetURL)` preserves relative asset URLs;
font imports must still precede ordinary rules.

Keep shared `--tw-*` variables when utilities compose on the same element,
including rings/shadows and prose color modifiers. For independent widgets,
`Config.InternalVariablePrefix: "widget"` isolates them as `--widget-tw-*`
without renaming classes or application variables.

### Reuse, watch mode and development reloads

`Build` produces CSS for the supplied classes and configured safelists/inline
sources. Calls are independent: accounts do not accumulate each other's classes.
Compilers and prepared templates can be shared across goroutines.

For a growing set of templates, keep `compiler.NewBuilder()`. It compiles only
additions and returns cached CSS when nothing changed. `Reset` starts fresh.
Keep a scanner alongside it: `Scan` returns all candidates found so far;
`ScanContent` returns additions from supplied text or files. Removed candidates
remain until a fresh scanner is created, as in Tailwind watch mode.

Each scanner, incremental builder and canonicalizer belongs to one caller.
Use `Files`, `Globs` and `ScannedFiles` for watch inputs. `Filesystem.OnDependency`
reports CSS/configuration dependencies; reload the compiler when those change.
Your application controls when to rebuild and replace its cached CSS.

### Compatibility

| Tailwind version | How to select it | Scope |
| --- | --- | --- |
| **4.3.4** | Default, or `--compat=4` | Core compiler, utilities, variants, CSS configuration, standard plugins, scanning, source maps and CLI behavior. |
| **3.4.17** | `Config{Compatibility: rocketwind.Tailwind3}` or `--compat=3` | Older utilities, variants, defaults, stylesheet directives, prefixes, important syntax, and string/regexp safelists. |

For an existing v3 stylesheet and JSON configuration:

```sh
rocketwind --compat=3 --config theme.json -i src/input.css -o public/app.css
```

In Go, select `Tailwind3` on the compiler, filesystem loader and scanner.
Configuration can use the `Legacy` API shown above or a JSON file loaded through
`Filesystem.LoadModule`. Convert JavaScript configuration to JSON or Go first.

The v3 profile lets an existing app keep its styles while new templates use v4.
It omits removed `corePlugins` switches, experimental flags and configurable
separators. Browser optimization uses current targets in both profiles, so v3
compatibility does not restore v3's old browser support matrix.

Rocketwind replaces CSS compilation. It does not execute JavaScript config or
plugins, or act as a PostCSS/Vite/Webpack adapter. Use JSON or Go configuration
and Go plugin callbacks. It has no Node, Rust, ICU or native-library runtime
dependency.

The Tailwind 4 core test suites and extensive independent v3 comparisons back
these claims. The entire original v3 suite has not been ported.
[TESTING.md](TESTING.md) records the exact coverage and JavaScript-host exceptions.

### CLI options

`rocketwind build` is the default command. Without `-i`, it starts with
`@import "tailwindcss"`; `scan` writes class names instead of CSS.

| Option | Purpose |
| --- | --- |
| `-i -`, `-o -` | Read stdin or write stdout. Output defaults to stdout. |
| `--cwd DIR`, `--content GLOB` | Choose the search root or explicit source patterns. |
| `--config theme.json`, `--compat=3` | Load object-style configuration or select v3 behavior. |
| `--classes 'flex p-4'` | Supply extra complete class names. |
| `--exclude shared.css` | Omit helpers already defined in a stylesheet. |
| `--minify`, `--optimize` | Optimize for browsers, with compact or readable output. |
| `--map`, `--map app.css.map` | Include an inline source map or write a separate map. |
| `--watch`, `--watch=always` | Rebuild on edits; `always` survives stdin closing. |
| `--poll=100` | Set the watch interval in milliseconds. |

`--content`, `--classes` and `--exclude` can be repeated. Watch mode keeps the
last valid CSS after errors and avoids rewriting unchanged output.

Canonicalize and sort class lists from arguments or stdin:

```sh
rocketwind canonicalize 'mt-2 mb-2 flex'
# my-2 flex
rocketwind canonicalize --css src/input.css --format json 'block [display:flex]'
rocketwind canonicalize --stream --format jsonl
```

Quote each group of classes. Unknown classes stay first; JSON output includes
the original input, result and whether it changed. Streaming handles one group
per line. `--css` supplies the theme and custom utilities.

Applications can call `cli.Run` with registered modules in `cli.Options`.
Use `ResolveModule` for configuration that changes on disk; report dependency
filenames through its callback so watch mode can rebuild.

### Browser output and source maps

`Optimize(css, options)` lowers nesting and media ranges, expands custom media,
adds browser prefixes and optionally minifies. It preserves license comments,
fallbacks, value precision and custom property names. Browser targets match
Tailwind 4.3.4: Safari/iOS 16.4, Firefox 128 and Chrome 111. Output need not use
every shorthand or shorter color spelling chosen by Lightning CSS.

Set `Config.From` and call `BuildWithSourceMap` on a compiler or builder to trace
CSS back to definitions and `@apply` uses, including imports. Pass the map to
`OptimizeWithSourceMap` when optimizing. Maps support JSON marshaling, `Inline()`,
`Comment(url)` and `Visualize(css)`. Leave `From` empty when locations are unneeded.

### Editor APIs

The same compiler can power completions, previews and class-list editing:

| API | Purpose |
| --- | --- |
| `Parse`, `Print` | Inspect typed candidates and print normalized class names. |
| `ClassList`, `VariantList`, `VariantSelectors` | Offer theme/plugin-aware completions and inspect variant selectors. |
| `CandidateCSS`, `CandidateAST` | Preview individual classes, without unrelated stylesheet rules. ASTs are independent copies. |
| `Walk` | Visit and edit CSS trees, with ancestor/sibling context and replace/remove/stop hooks. |
| `Extract`, `AppendExtracted`, `CSSVariables` | Inspect source tokens and byte offsets; reuse an extraction buffer. |
| `compiler.Candidates` | Sort and deduplicate a manifest, including safelists/inline sources and exclusions. |
| `ClassOrder`, `SortClasses`, `Canonicalize` | Order classes and rewrite equivalent spellings. |

`Canonicalize` preserves unknown classes and theme-variable dependencies.
`Collapse: true` combines `mt-2 mb-2` into `my-2`; `Rem` controls unit conversion,
and `LogicalToPhysical` opts into interchangeable writing directions.
For repeated edits, keep `NewCanonicalizer(options)` while the theme stays the
same. Ordinary CSS compilation does not need canonicalization first.


Implementation details
----------------------

Rocketwind is a **pure Go compiler using only the standard library**. It parses
CSS and class names, resolves themes and plugins, and generates CSS in process.
Registrations and data tables derived from Tailwind sources are checked in;
they do not look up precomputed output for a list of known classes.

Large scans use bounded parallel extraction and disk reads. Custom `fs.FS`
reads remain sequential, and small inputs avoid worker overhead. Legacy
extraction uses native loops and bounded state tables with regexp fallback.
Compiler state is reusable; each build owns its temporary data.

### Performance

Fresh compilation of the [592-class fixture](testdata/performance/loyalty.json),
including forms, typography and browser optimization:

| Machine | Tailwind 3.4.17 | Rocketwind v3 | Tailwind 4.3.4 | Rocketwind v4 |
| --- | ---: | ---: | ---: | ---: |
| Apple M5 Max | 47.60 ms | **4.45 ms** | 12.49 ms | **5.65 ms** |
| AMD EPYC 7502P | 160.47 ms | **30.31 ms** | 40.08 ms | **33.58 ms** |

Go 1.27.3, Node 26.4.0; medians of five runs on each machine. Each Rocketwind
profile uses the matching Tailwind defaults and plugin versions. Source scanning
and process startup are timed separately.

In an application using Rocketwind, scanning embedded frontend assets and
initializing shared CSS plus the GoX compiler takes **131 ms**. A prepared component template resolves
and renders in **2.05 µs**; an unchanged incremental build allocates nothing.
See [PERFORMANCE.md](PERFORMANCE.md) for methodology, allocation counts, source
scanning measurements and runnable benchmarks.

### Tests and contributing

The Tailwind 4 corpus replays **17,895 captured upstream calls** from all 44 core
compiler suites and associated helpers. The CLI runs all 64 applicable upstream
integration cases. Browser optimization has another **2,310 independent
comparisons**. Standard plugins and the v3 profile have their own reference tests.

The whole Go implementation has **100% statement coverage**, including the real
CLI subprocess, with no source exclusions. Contributions should preserve that
coverage and the upstream comparisons:

```sh
go test ./...
go test -race ./...
go vet ./...
python3 scripts/check-coverage.py
```

Bug fixes, better examples and measured performance improvements are welcome.
Keep public APIs and plugin behavior compatible. [TESTING.md](TESTING.md) lists
the pinned suites, reference tools and permitted JavaScript-host exceptions.
Python and the upstream JavaScript/Rust tools are development-only dependencies;
building and using Rocketwind needs only Go.


License
-------

MIT. Copyright (c) 2026, Andrey Tarantsov.

Parts adapted from Tailwind CSS retain Tailwind Labs' MIT license; see
[the upstream license](testdata/upstream/LICENSE). Plugin sources and the small
SVG encoder retain their licenses under [testdata/plugins](testdata/plugins).
Unicode collation data retains its [Unicode license](data/collation/LICENSE).
CLI helpers retain the [mri](testdata/upstream/MRI-LICENSE) and
[ansi-regex](testdata/upstream/ANSI-REGEX-LICENSE) MIT licenses.
