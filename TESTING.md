# Compatibility tests

The current target is Tailwind CSS **4.3.4**, commit
`2626ae6ceddf36069ab6d55c53a5857c1e2e360a`. Go compilation has no JavaScript,
Node, Rust, ICU or native-library dependency. Those tools are used to collect
independent reference results during development.

Run the Go tests and the strict coverage gate:

```sh
go test ./...
go test -race ./...
go vet ./...
python3 scripts/check-coverage.py
```

The coverage script includes the real CLI subprocess. It requires **100% of
statements across all Go packages**, with no source exclusions. The current
checkpoint covers 12,352 of 12,352 statements. Generated registrations are
source-derived data; they do not look up expected CSS by candidate name.

## Tailwind 4

All **44 original core compiler test files** ran successfully: **5,188 passing
tests**, plus one test already skipped upstream. Their captured calls are
replayed through the Go implementation. The combined compiler and helper
corpus contains **17,895 calls**, including candidate syntax, utilities,
variants, configuration, CSS functions, imports, source maps, canonicalization,
AST transformations, source scanning and CLI helpers.

The original CLI integration run passed **68 tests**, with two existing upstream
skips. Go replays **64 applicable cases**; four cases specifically require
JavaScript execution. Their names and reasons are checked in
[cli/integration_test.go](cli/integration_test.go). Declarative configuration and
standard plugins have Go counterparts, so using JavaScript in an upstream test
is not by itself a reason to omit the behavior.

Both readable and compact browser output pass **2,310 independent comparisons**
against the exact optimizer used by Tailwind. These include the compiler CSS
corpus and **306 extra browser inputs**. The comparison normalizes equivalent
CSS spelling without supplying browser targets, so it cannot conceal a missing
prefix by adding it during comparison. Rule order and fallback values remain
part of the comparison.

The original Rust extractor/preprocessor tests and expanded inputs are captured
and run in Go. They include byte offsets, invalid UTF-8, arbitrary values and
supported template languages. Source fixtures and their licenses live under
[testdata](testdata).

## Standard plugins

Go implementations cover forms 0.5.11, typography 0.5.20, aspect-ratio 0.4.2,
line-clamp 0.4.4 and container-queries 0.1.1. Original plugin tests run against
their pinned dependencies, and separate builds compare behavior with Tailwind
4. Forms has no original automated suite; it has independent differential tests.
The Tailwind 3 profile also supplies forms 0.5.9 and typography 0.5.15.

## Tailwind 3

The optional **3.4.17** profile is checked independently against the original
compiler: 11,344 named utilities, 2,759 arbitrary values, 848 variant builds,
258 standard-plugin builds, nine configurations, eighteen stylesheets and three
complete completion lists. It also runs the original 45 extractor tests,
57 normalization tests and six duplicate-declaration tests. Another 1,500
deterministic extractor inputs check the native plain-text scanning path.

This is an application migration profile, not a claim that the entire original
Tailwind 3 suite has been ported. Removed core-plugin switches, experimental
flags and configurable separators are outside this profile. The current
Tailwind 4 implementation is the full-suite target.

## JavaScript host exceptions

The source inventory in `testdata/upstream/manifest.json` records hashes; it is
an inventory, not an assertion that every test in the monorepo ran.

PostCSS, Vite, Webpack and Turbopack adapters, Node module loading and exports,
JavaScript configuration execution, JavaScript upgrade codemods, and WASM/worker
host bridges require their JavaScript hosts. Rocketwind implements the compiler
behavior and Go loading/plugin APIs instead. The Node instrumentation test checks
JavaScript timing, promises and `Symbol.dispose`; it has no CSS behavior to port.

No core utility, variant or standard plugin is excluded because of its behavior.
If a reference comparison fails, fix the implementation before updating verified
Go outputs. `scripts/verify-optimize.mjs --update` writes outputs only after every
independent comparison passes.
