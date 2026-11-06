# Performance

Measured on 6 November 2026, on an Apple M5 Max with 18 logical CPUs, Go 1.27.3
and Node 26.4.0. Benchmarks ran separately from tests and profiling. Go figures
are medians of three runs; allocation figures include garbage produced during
each operation, not just the memory retained afterward.

## Current Tailwind comparison

The public [loyalty fixture](testdata/performance/loyalty.json) contains 592
classes: layout, spacing, colors, responsive and state variants, forms and
typography. It is a synthetic stress fixture, not a captured customer's page.
Each operation creates a compiler, configures both standard plugins, builds all
classes and optimizes the CSS for browsers. It excludes source scanning.

Rocketwind takes **5.50 ms**, allocating 10.38 MB in 91,634 allocations. The
official Tailwind **4.3.4** compiler and optimizer take about **12 ms**. Neither
measurement reuses a completed compiler or an unchanged-build cache. Node
loads modules before timing, discards 20 warmup runs, then measures 100 fresh
compilations. The Go benchmark also keeps process startup outside the timer.

Run the Go side:

```sh
go test -run '^$' -bench '^BenchmarkLoyaltyCompiler$' -benchtime=2s -count=3 -benchmem
```

For the independent reference, install development tools outside the Go module:

```sh
npm install --prefix .cache --no-save tailwindcss@4.3.4 @tailwindcss/node@4.3.4 @tailwindcss/forms@0.5.11 @tailwindcss/typography@0.5.20 postcss@8.5.32
node scripts/benchmark-compiler.mjs
```

The script writes `.cache/performance/official-loyalty.css`. An optional third
argument names a Rocketwind output file to compare. That check normalizes CSS
spelling, grouped selectors and adjacent identical selectors. It preserves rule
order and supplies no browser targets that could conceal missing prefixes.

To generate and check the Rocketwind side of the fixture:

```sh
go build -o .cache/rocketwind ./cmd/rocketwind
python3 - <<'PY'
import json, pathlib, subprocess
fixture = json.loads(pathlib.Path('testdata/performance/loyalty.json').read_text())
subprocess.run([
    '.cache/rocketwind', 'build', '-i', '-', '--content', '__missing_perf_input__',
    '--classes', ' '.join(fixture['classes']), '--optimize',
    '-o', '.cache/performance/rocketwind-loyalty.css',
], input=fixture['input'], text=True, check=True)
PY
node scripts/benchmark-compiler.mjs .cache .cache/performance/rocketwind-loyalty.css
```

## Application workloads

The Bubblehouse profiles use its actual embedded templates, JavaScript bundles,
CSS, configuration and native plugins. The baseline is Rocketwind commit
`13b1602`; both versions receive the same application snapshot. These private
sources are not copied into this public repository.

| Work | Before | After | Speedup |
| --- | ---: | ---: | ---: |
| Scan embedded frontend assets | 754 ms | 78.1 ms | 9.7x |
| Compile and optimize the application stylesheet | 120 ms | 28.5 ms | 4.2x |
| Initialize shared CSS and the GoX compiler | 920 ms | 131 ms | 7.0x |
| Collect known source classes, fresh CLI process | 551 ms | 170 ms | 3.2x |

Startup allocations fell from 5.37 million / 520 MB to 994 thousand / 123 MB.
The 960-class source manifest, complete runtime candidate set and 216,307-byte
stylesheet are byte-for-byte unchanged. The CLI figures are medians of ten
fresh processes over the same source tree, with a warm operating-system cache.

For the legacy application input, official Tailwind 3.4.17 plus current browser
optimization takes a median **341 ms**, versus **49.8 ms** for Rocketwind
configuration plus CSS generation. Both receive the same candidate list and
use the old forms/typography versions. Neither timing includes source scanning
or process startup. The official custom extractor passes the supplied candidates
through directly, avoiding an extra scan of the candidate manifest.

Application measurements and reproduction commands also live in Bubblehouse's
`_readme/rocketwind.md`. They separate configuration, scanning, compilation,
startup, GoX CSS generation and whole-tenant loading. The latter loads existing
large loyalty-page fixtures through the application's production-repro harness.
Existing tenant theming remains on its original engine, so a CSS compiler
speedup does not imply the same speedup for every part of tenant loading.

## What changed

Candidate parsing uses stack buffers and internal visitors. Builds flatten each
legacy color namespace once, and ignore unrelated source words without keeping
a large duplicate-detection map for them. Theme configuration uses ordered
indexes and temporary conversion caches; plugin callbacks still receive
independent mutable values.

Scanning uses up to eight extraction workers and up to eight disk readers,
bounded by `GOMAXPROCS`. Readers prefetch at most two files each and consume
results in discovery order. Custom `fs.FS` reads remain sequential. Small scans
stay synchronous. Output remains sorted, and incremental scans keep their
existing candidate-retention behavior.

Legacy extraction uses native loops and small precomputed state tables derived
from the original patterns. Unicode, uncommon states and searches exceeding a
linear work budget fall back to Go's regexp engine. Tables are immutable and
bounded; there is no growing process-wide candidate cache. Safelist modifier
names are expanded once across patterns.

## Other benchmarks

```sh
go test -run '^$' -bench '^(BenchmarkBuild|BenchmarkBuildSourceTokens|BenchmarkTemplate|BenchmarkIncrementalUnchanged|BenchmarkOptimize|BenchmarkScannerColdSourceTree)$' -benchtime=2s -count=3 -benchmem
```

`BenchmarkScannerColdSourceTree` creates 1,000 source files outside the timer
and constructs a fresh scanner on each iteration. The operating system's file
cache is warm; "cold" here means no retained scanner state. The existing
`BenchmarkScanner/files-cached` instead measures an unchanged incremental scan.

Use `-cpuprofile` and `-memprofile` on one benchmark at a time when investigating
regressions. Do not compare a profiled run against an unprofiled run, or run
heavy tests alongside timing measurements. Correctness gates are documented in
[TESTING.md](TESTING.md).
