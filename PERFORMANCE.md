# Performance

Measured on 6 November 2026 with Go 1.27.3 and Node 26.4.0:

* Apple M5 Max, 18 logical CPUs, macOS 26.5.2.
* AMD EPYC 7502P, 32 cores / 64 logical CPUs, Linux 6.18.2.

Benchmarks ran separately from tests and profiling. The server was lightly
loaded; benchmark processes ran at niceness 10. Go used its default `GOMAXPROCS`
on both machines. The Linux benchmark executable was cross-compiled with the
same Go 1.27.3 toolchain used on the Mac. The server used the official Node 26.4.0
Linux binary.

## Tailwind 3 and 4 comparison

The public [loyalty fixture](testdata/performance/loyalty.json) contains 592
classes: layout, spacing, colors, responsive and state variants, forms and
typography. It is a synthetic page-builder workload.
Each operation creates a compiler, configures both standard plugins, builds all
classes and optimizes the CSS for browsers. Both versions use forms 0.5.11
(class strategy) and typography 0.5.20. Each Rocketwind profile uses the matching
Tailwind defaults, including v3's smaller spacing scale. The fixture omits
preflight; source scanning, file I/O and process startup are outside the timer.

| Compiler | Apple M5 Max | AMD EPYC 7502P |
| --- | ---: | ---: |
| Tailwind 3.4.17 | 47.60 ms | 160.47 ms |
| Rocketwind v3 | **4.45 ms (10.7×)** | **30.31 ms (5.3×)** |
| Tailwind 4.3.4 | 12.49 ms | 40.08 ms |
| Rocketwind v4 | **5.65 ms (2.2×)** | **33.58 ms (1.2×)** |

Rocketwind v3 allocates about 8.33 MB in 120,620 allocations per build; v4
allocates about 10.38 MB in 91,634 allocations. These count all allocated memory,
including garbage collected during compilation.

Go figures are medians of five runs with `-benchtime=2s`. Node figures are medians
of five process medians: each process loads modules, discards 20 warmup runs,
then times 100 fresh compilations. Tailwind v3's compilation-context maps are
cleared each iteration; creating a new PostCSS processor alone would reuse
compiled rules. [Raw samples and environment details](testdata/performance/2026-11-06.json).

Run the Go side:

```sh
go test -run '^$' -bench '^BenchmarkLoyaltyCompiler(V3)?$' -benchtime=2s -count=5 -benchmem
```

For the independent reference, install development tools outside the Go module:

```sh
npm install --prefix .cache --no-save tailwindcss@4.3.4 @tailwindcss/node@4.3.4 @tailwindcss/forms@0.5.11 @tailwindcss/typography@0.5.20 postcss@8.5.32
npm install --prefix .cache/benchmark-v3 --no-save tailwindcss@3.4.17 @tailwindcss/node@4.3.4 @tailwindcss/forms@0.5.11 @tailwindcss/typography@0.5.20 postcss@8.5.32
for trial in 1 2 3 4 5; do
    node scripts/benchmark-compiler.mjs .cache/benchmark-v3 --compat=3
    node scripts/benchmark-compiler.mjs .cache
done
```

The script writes `.cache/performance/official-loyalty.css` and
`official-loyalty-v3.css`. An optional filename after the tooling directory names
a Rocketwind output file to compare. The check normalizes CSS spelling, grouped
selectors, adjacent identical selectors and the position of independent `display`
declarations. It preserves rule order, fallback declarations and prefixes.
Both profiles matched the official output from both machines.

To generate and check the Rocketwind side of the fixture:

```sh
go build -o .cache/rocketwind ./cmd/rocketwind
python3 - <<'PY'
import json, pathlib, subprocess
fixture = json.loads(pathlib.Path('testdata/performance/loyalty.json').read_text())
for version in (3, 4):
    css = fixture['input']
    if version == 3:
        css = ('@plugin "@tailwindcss/forms" { strategy: class; }\n'
               '@plugin "@tailwindcss/typography";\n' + fixture['inputV3'])
    subprocess.run([
        '.cache/rocketwind', 'build', f'--compat={version}', '-i', '-',
        '--content', '__missing_perf_input__', '--classes', ' '.join(fixture['classes']),
        '--optimize', '-o', f'.cache/performance/rocketwind-loyalty-v{version}.css',
    ], input=css, text=True, check=True)
PY
node scripts/benchmark-compiler.mjs .cache/benchmark-v3 .cache/performance/rocketwind-loyalty-v3.css --compat=3
node scripts/benchmark-compiler.mjs .cache .cache/performance/rocketwind-loyalty-v4.css
```

## Application workloads

The application profiles use actual embedded templates, JavaScript bundles,
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

Application measurements and reproduction commands also live in the application's
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
