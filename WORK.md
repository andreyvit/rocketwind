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
9. GoX blocks contribute their Tailwind classes to tenant CSS, excluding classes
   already defined in admin.css. Keep the broader runtime theming replacement
   disabled; test tenant isolation and CSS cache invalidation.

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
- Core Go suite, race detector and vet pass. Statement coverage: 96.4%.
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
- Programmatic plugin construction, component selector registration and typed
  utility matching implemented; new Go plugin code has 100% statement coverage.
- Forms v0.5.11 port implemented from pinned source registrations. All 24
  differential CSS fixtures and 20 SVG encoder fixtures pass. No forms tests
  exist in that upstream release. New forms code has 100% statement coverage.
- Typography v0.5.20 port implemented from pinned source defaults. All 16
  original upstream tests pass on their pinned Tailwind 3.2.2 dependency;
  20 latest-Tailwind differential CSS fixtures and 20 selector cases pass.
  Original v3 output is retained for the pending compatibility profile.
  New typography code has 100% statement coverage.
- Aspect-ratio v0.4.2, line-clamp v0.4.4 and container-queries v0.1.1 ported.
  All six original upstream tests pass with legacy dependencies; their inputs
  and 24 additional configurations match latest Tailwind output (30 fixtures).
  Original legacy results are retained for the compatibility profile. The
  v4 differential harness supplies the removed variants() API to line-clamp.
  New standard-plugin code has 100% statement coverage.
- Remaining: full internal and compatibility test suites,
  scanning/CLI, coverage/performance
  and integration gates.
- Implementation and conformance gates remain open.

### Legacy configuration and expanded compiler checks

- Ported the object-style compatibility API: ordered configuration, theme
  replacement/extension and Go callbacks, presets, font tuples, dark mode,
  prefixes, blocklists, screens and container overrides. Generated defaults
  come from pinned source, not expected test results.
- Added typed utility fallbacks, parallel plugin variants, independent static
  utility registrations, and dynamic plugin `@apply`/`@variant` expansion.
  Expansion detects cycles and excessive depth; failed incremental builds
  retain their previous CSS and can be retried.
- Captured all 149 original tests in the four plugin/configuration suites.
  The harness now checks successful compile-only calls too, including complete
  plugin theme/config lookups, content paths and source roots.
- Re-ran ten original compiler suites: all 874 tests pass. Across fourteen
  compiler suites, 4,545 checks pass: 2,225 CSS builds, 68 errors and 2,252
  metadata results. With the existing helper suites, 10,696 captured calls pass.
- Added programmatic configuration, JSON ordering/errors, source discovery,
  concurrent build and dynamic plugin failure tests.
- Go tests, race detector and vet pass; statement coverage is 96.3%. The 100%
  coverage gate remains open.
- This extends the Tailwind 4 compatibility API. Tailwind 3 output parity and
  Bubblehouse integration remain pending.

### Remaining compatibility internals

- All 31 original tests in the five remaining compatibility suites pass.
  Their 95 captured calls now pass in Go: configuration resolution, CSS-to-legacy
  namespace mappings, configuration-to-CSS mappings, keyframes and color palettes.
- Fixed the order of replaced default theme variables and rejected invalid
  configuration keys. Added `FlattenColors` for Go utility plugins.
- Total captured upstream calls checked: 10,791. Go tests, race detector and
  vet pass; statement coverage is 96.4%. Other internal suites and integration
  gates remain open.

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
