# Implementation gates

Target: Tailwind CSS v4.3.4. Bubblehouse's existing build targets v3.4.17.

1. Repository, upstream source pin, test inventory and compatibility audit.
2. CSS/value/candidate parsing, typed parameters, identifier escaping.
3. Themes, all core utilities and variants; differential upstream tests.
4. Stylesheet directives, imports, custom utilities, programmatic Go plugins.
5. Standard plugins, source scanning, CLI, existing-stylesheet exclusion.
6. Benchmark, allocation profiling, race tests, fuzzing, 100% Go coverage.
7. Bubblehouse CSS generation tests and disabled runtime integration proof.
8. Bubblehouse compiler replacement after CSS equivalence verification.

Do not mark a gate complete while tests fail or cases remain unported.
Only JavaScript execution itself and JavaScript-specific host integrations may
be excluded; core behavior and standard plugins must be implemented in Go.

## Current state

- Public repository created; SSH clone initialized.
- Upstream v4.3.4 checked out for reference.
- CSS parser: 222 upstream calls passing. Arbitrary value decoder: 66 passing.
- Utilities: all 749 captured compiler calls from 400 upstream tests match exactly.
- Variants: all 1,009 captured compiler calls from 104 upstream tests match exactly;
  the upstream matchVariant test plugin is ported to a Go callback.
- General stylesheets: 252 calls; prefixes: 14; important: 4; CSS functions: 75;
  imports: 27; compiler lifetime: 1; plugin entry points: 2; legacy utilities: 13. All match exactly.
  Total compiler calls: 2,146. Declarative JS test plugins replay through the Go
  API; functional test callbacks have explicit Go ports. No Go conformance skips.
- Original upstream runs: 400 utility tests, 104 variant tests, and 376 tests in
  the eight additional source files pass. One upstream sorting test is already
  skipped upstream. All 14 captured class-order calls now pass.
- Core Go suite, race detector and vet pass. Statement coverage: 95.3%.
- Candidate syntax/normalization: all 2,672 captured calls from 1,388 upstream
  tests pass. CSS generation now uses the same parser exposed by Compiler.Parse.
- Nine more syntax/helper suites: 2,573 calls from 169 upstream tests pass;
  covers values, selectors, attributes, braces, escaping, segmenting, comparison,
  theme paths and shadow colors. Total captured calls checked: 7,677.
- CSS AST/nesting: all 604 calls from 208 upstream tests pass. Total captured
  calls checked: 8,297.
- Typed templates, scoped class-list references, named selectors and opt-in aliases
  implemented. Templates and aliases have 100% statement coverage.
- Existing-CSS exclusion implemented, with escaped selectors, nested rules and
  independent named-rule resolution; 100% statement coverage.
- Incremental builder caches additions and reuses unchanged output with zero
  allocations; independent compiler builds remain stateless. Builder and class
  ordering have 100% statement coverage.
- Removed repeated variant parsing during sorting and unnecessary CSS tree copies.
- Remaining: full internal and compatibility test suites,
  standard plugins, scanning/CLI, coverage/performance
  and integration gates.
- Implementation and conformance gates remain open.

## Initial performance measurements

Apple M5 Max, Go 1.27.3, three local runs per case. Before/after medians:

| Workload | Before | After | Allocations before / after |
| --- | --- | --- | --- |
| Build 10 candidates | 34.9 us | 24.7 us | 942 / 583 |
| Build 100 candidates | 162 us | 133 us | 4,899 / 3,724 |
| Build 586 candidates, including variants | 3.42 ms | 1.20 ms | 109,122 / 33,044 |
| Resolve and render an account template | 2.93 us | 1.95 us | 102 / 58 |
| Repeat unchanged incremental build, 586 candidates | — | 3.44 us | 0 |

The large build's allocated bytes fell from about 6.74 MB to 1.76 MB.
This is an initial profile pass, not the final performance gate. Benchmark code
is in benchmark_test.go; raw local results and profiles are in .cache/.
