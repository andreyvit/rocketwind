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
- Core Go suite, race detector and vet pass. Statement coverage: 97.5%.
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
- Remaining: internal and CLI suites, optimization, Tailwind 3 compatibility,
  coverage/performance and integration gates.
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

### Source extraction

- Ported all Rust extraction machines and fourteen language preprocessors from
  Tailwind 4.3.4. All 97 original Rust tests pass (12 performance tests are
  already ignored upstream). All 2,968 distinct captured calls pass in Go:
  2,697 extraction calls and 271 preprocessing calls.
- Source offsets and invalid UTF-8 behavior match upstream. Added source
  variable tracking to full and incremental compiler builds, with eleven
  differential fixtures for dependencies, prefixes, keyframes and exclusions.
- Reusing an extraction buffer needs zero allocations. Benchmarks cover repeated
  markup and upstream's separate 27,895-byte example HTML fixture, copied from
  `crates/oxide/src/fixtures/example.html` under the upstream MIT license.
- Median extraction time: 759 ns for a short template, 75.2 us for 100 repeated
  lines, and 135.5 us for the separate upstream HTML file (206 MB/s).
- Two 15-second fuzz runs passed, including malformed input across thirteen
  preprocessing paths. Go tests, race detector and vet pass. The whole Go
  coverage gate remains open.
- Captured upstream calls checked: 13,759, plus standard plugin and additional
  variable-reference fixtures. Filesystem scanning and CLI are next.

### Filesystem scanning

- All 125 original Rust unit tests and 71 filesystem tests pass; twelve
  upstream performance tests remain ignored upstream. Captured and ported
  411 filesystem/source/glob calls, plus the cursor test directly. Every
  non-ignored original Oxide test now has a passing Go counterpart.
- Fixture capture retains file contents, invalid UTF-8, timestamps, directory
  changes and symlinks. macOS `/var` and `/private/var` aliases normalize to one
  fixture path; resulting duplicate file identities are collapsed.
- Added scanner APIs for stylesheet sources, raw content, incremental files,
  byte positions and watch globs. Tests cover explicit sources, ignore rules,
  repository boundaries, CSS variables, new files and recursive symlinks.
- Profiled a cached scan of 1,000 files. Avoiding repeated symlink resolution
  and regular expressions for simple globs reduced median time from 37.7 ms
  to 4.56 ms, allocations from 45,538 to 6,841, and allocated bytes from
  4.64 MB to 742 KB. Repeating unchanged raw content uses zero allocations.
- Captured upstream calls checked: 14,170, plus standard plugin fixtures,
  variable-reference fixtures and directly ported tests. CLI, remaining
  TypeScript internals, coverage and integration gates remain open.
- Go tests, race detector and vet pass; overall statement coverage is 96.3%.

### CLI and filesystem loaders

- Added the standalone command and reusable `cli.Run`, with file/stdin input,
  file/stdout output, watch mode, explicit sources, extra classes and exclusion
  stylesheets. JSON configuration can name built-in or registered Go plugins.
- Added stylesheet/package resolution, dependency tracking and asset URL
  rebasing. Both original Tailwind URL tests and their captured calls pass.
- CLI tests cover GoX source text, exclusions, registered Go plugins, command
  errors, dependency changes, recovery from invalid CSS and unchanged output.
- Go tests, race detector and vet pass. The built executable passed a separate
  stdin/GoX/exclusion/output smoke test. Current combined statement coverage is
  96.1%; command entry-point coverage remains part of the integration gate.
- Minification, source maps, canonicalization and the original CLI integration
  suite remain pending. This is a usable compiler checkpoint, not completion of
  the CLI or compatibility gates. No Bubblehouse files have been integrated.

### Source maps

- All 29 original tests in the four core source-map suites pass upstream;
  all 89 captured calls now pass in Go. Also ported the node adapter's source-map
  comment assertion. Total captured upstream calls checked: 14,261.
- Added byte-to-UTF-16 mapping, ECMA-426 encoding, diagnostic visualization and
  full/incremental map APIs. Eight original maps match source-map-js encoding;
  twelve additional differential cases cover Unicode, CRLF/BOM, custom
  variants, property fallbacks and repeated or overlapping `@apply` tokens.
- Fixed plugin execution order, color-fallback locations and `@apply` token
  positions exposed by these tests. Mapping preserves immutable compiler
  state and passes concurrent-build checks. Unchanged incremental map builds
  use zero allocations; new map builds render once.
- Added CLI inline/file maps, imported-source tracking, write protection and
  map-only watch updates. Outputs are excluded from subsequent scans.
- Source-map implementation files have 100% statement coverage. Overall Go
  coverage is 96.4%; the project-wide 100% gate remains open. Race tests and
  vet pass. A 15-second decoder fuzz run passed 1.74 million executions.
- Quiet benchmark medians: 586 classes without maps, 1.25 ms; a complete
  imported stylesheet with retained locations, 1.86 ms without map generation
  and 2.16 ms with maps. Cached unchanged builds remain at zero allocations.
  These are checkpoint measurements, not the final performance gate.
- Minification, canonicalization, remaining internal/CLI suites, the v3
  compatibility profile and all Bubblehouse integration remain pending.

### Value normalization

- All 111 original tests in the calculation canonicalization, constant folding
  and declaration expansion suites pass. Their 135 captured calls pass in Go,
  along with 274 additional arithmetic, unit and numeric-boundary cases.
  Total captured upstream calls checked: 14,396.
- Added pure-Go Unicode 17 collation, matching the reference Node runtime's
  ICU 78.3 / CLDR 48 data. All 206,298 CLDR conformance inputs and 87,716
  additional comparison pairs match Node, including canonical decomposition,
  contraction contexts, combining marks, newer emoji and ICU index markers.
- Unicode tables are generated from the pinned ICU release's public APIs.
  Node, Rust and ICU remain development tools; the Go library has zero
  dependencies and does not execute external code.
- New normalization, declaration expansion and collation code has 100%
  statement coverage. Overall coverage is 96.6%; race tests and vet pass.
- The ASCII comparator's median time fell from 3.14 us to 164 ns with zero
  allocations. Case-sensitive comparison fell from 5.67 us to 331 ns.
  Canonical class conversion and the remaining implementation gates are open.

### Editor suggestions and previews

- All 21 original IntelliSense tests pass upstream; all 33 captured calls pass
  in Go. Total captured upstream calls checked: 14,429.
- Added class and modifier suggestions, variant arguments and selector previews,
  plus individual CSS/AST previews. Suggestion tables come from the pinned
  source registrations. Custom utilities and Go plugins extend the lists.
- Fixed plugin variant discovery and suggestion registration order. Returned
  trees are independently editable; concurrent previews and suggestions pass
  the race detector. The new editor API code has 100% statement coverage.
- Profiling the full default theme's 23,286 classes reduced suggestion time from
  25.2 ms to 12.4 ms, allocations from 145,883 to 32,746 and allocated memory from
  21.4 MB to 15.4 MB. Returned lists own their modifier storage. Numeric sorting
  now skips digit runs it has already compared.
- Overall coverage is 96.7%; tests, race tests and vet pass. Canonical class
  conversion, CLI optimization, remaining suites and integration gates remain
  open.

### Canonical class conversion

- All 1,896 original canonicalization tests pass upstream. All 3,353 captured
  calls pass in Go: 1,916 class-list conversions, 886 ordering calls and 551
  set operations. Total captured upstream calls checked: 17,782.
- Added utility and variant normalization, equivalent class lookup, deprecated
  spellings, theme-function migration and class collapsing. Replacements compare
  normalized CSS and preserve dependent theme variables. Prefixes, inline themes,
  typed arbitrary values and failing Go plugins have matching behavior.
- Added 154 further differential cases, concurrent compiler reuse, independently
  owned results, option isolation and combinations beyond 64 inputs. Canonical
  conversion files have 100% statement coverage. Whole-project coverage is 97.3%;
  tests, race tests and vet pass.
- Reused normalized values and property discovery, and cached class groups only
  while their replacement index stays unchanged. On the seven-class benchmark,
  initial full-theme time was 477 ms with 15.7 million allocations and 727 MB
  allocated. The final three-run median is 266 ms, 5.33 million allocations and
  408 MB. This includes indexing all known utilities on the first call.
- Repeating an unchanged class list takes 26 ns and one 80-byte allocation for
  the independently owned result, after the index settles. Ordinary CSS builds
  do not use this index. These remain checkpoint measurements, not the final
  performance gate.
- The CLI canonicalize command, minification, remaining suites, v3 compatibility
  and Bubblehouse/GoX integration are still pending.

### CLI canonicalization

- All twelve original CLI canonicalization tests pass upstream and in Go.
  Total captured upstream calls checked: 17,794. Go callback ports preserve
  plugin diagnostics; the Go command does not execute JavaScript fixtures.
- Added quoted class groups, stdin input, streaming, theme stylesheets and
  text/JSON/JSONL output. Class sorting is also available through `SortClasses`.
- Added cancellation, long lines, CRLF boundaries, terminal input and I/O failure
  checks. The race detector exposed an unlocked `WriteString` in the shared test
  buffer; its writes now use the same lock as byte writes and reads.
- The command implementation has 100% statement coverage. Whole-project
  coverage is 97.4%; tests, race tests, vet and a compiled-binary smoke test pass.
  Remaining CLI/internal suites, optimization, v3 compatibility and integration
  gates remain open.

### Complete core suite and CLI helpers

- Ran all 44 original core suites together: 5,188 tests pass, with one test
  already skipped upstream. All core test files now have Go counterparts.
- Added all 59 captured walks from 45 traversal tests, including replacements,
  entry/exit hooks, ancestor paths, sibling identity and mutations. Traversal is
  iterative; a 100,000-level tree also passes. New traversal code has 100%
  statement coverage. Ported the seven LRU tests directly; as upstream does,
  use that bounded cache only in the canonicalization test harness.
- All 36 original CLI argument, duration and wrapping tests pass upstream;
  their 43 captured calls pass in Go. Another 812 argument cases match the
  reference, including numeric overflow, radix values and Unicode whitespace.
- Aligned duplicate flags, alias priority, short flags, boolean/number unions,
  terminal help, diagnostic wrapping and the canonicalize command's final
  newline. CLI argument conversion has 100% statement coverage.
- Refreshed all core captures. The randomized comparison test made one fewer
  comparator call this run; its assertions and every captured comparison pass.
  Total captured original upstream calls checked: 17,895, plus direct ports,
  standard plugins and the additional differential fixtures.
- Go tests, race tests and vet pass; overall coverage is 97.5%. CSS optimization,
  CLI integration suites, the v3 compatibility profile, complete coverage and
  the final performance/integration gates remain open.

### CSS optimization and source maps

- Added readable and compact optimization, full nesting lowering, adjacent rule
  grouping and source-map composition. The CLI exposes `--optimize` and
  `--minify` / `-m`; watch builds reuse optimized output when the input CSS and
  source map have not changed. Reusing that cache allocates nothing.
- Captured all 849 distinct stylesheet outputs from the complete core suite.
  Both optimizer modes pass 1,698 independent comparisons through Tailwind's
  pinned Lightning CSS optimizer and parsed CSS trees. Comparisons ignore only
  insignificant token whitespace; they retain declaration order, fallbacks,
  selector structure and opaque future mixins.
- Added idempotence checks across those inputs, source-map composition and
  ownership checks, CLI failures and escaped-selector regressions. Preserve
  whitespace-only custom properties and the terminator of hexadecimal escapes.
  The optimizer and its CLI cache have 100% statement coverage.
- The 40 KB compiler-output benchmark takes a three-run median of 317 us in
  readable mode (4,062 allocations) and 412 us compact (7,619 allocations).
  These are initial measurements; the final allocation pass remains open.
- Go tests, race tests and vet pass; overall coverage remains 97.5%.
- This is the structural optimizer checkpoint. Browser-specific transforms,
  complete CLI integration checks, whole-project coverage, v3 compatibility and
  the final performance/integration gates remain open.

### Complete CLI integration replay

- All four original CLI integration files pass against the pinned release:
  68 tests pass; two source-map cases are already skipped upstream.
- Captured over 1,000 filesystem, process and assertion operations. The Go replay
  passes all 64 applicable cases, including 100 source-map position checks.
  Four named exceptions require JavaScript module formats, JavaScript exports,
  or execution of the third-party tailwindcss-animate plugin.
- Go ports supply the test plugins and file-backed configuration callbacks.
  `cli.Options.ResolveModule` supports those callbacks and reports dependencies
  to watch mode. The self-scanning test runs a real instrumented Go binary.
- Fixed property-layer placement after external imports, ancestor ignore
  boundaries, typography theme overrides, CLI parse-error filenames, generated
  license/source-map offsets and failed-watch completion messages.
- Actual terminal detection now uses the OS terminal API; `/dev/null` is no
  longer mistaken for a terminal. Darwin and Linux PTY tests cover terminals;
  Windows and Linux CLI binaries cross-compile.
- Go tests, race tests and vet pass. Overall statement coverage is 97.4% with
  ordinary package-local instrumentation; whole-project 100% remains open.
  Browser optimization, the v3 profile and Bubblehouse integration remain open.

### Typography configuration

- All 20 latest-Tailwind typography cases now run through both the direct Go
  options and the legacy configuration loader. The original 16 tests still
  pass against their exact Tailwind 3.2.2 dependency; the capture checks that
  version before running instead of accepting a newer caret-range resolution.
- Preserve the plugin's original theme object, including CSS arrays, so other
  theme callbacks can refer to its defaults. Theme replacement and extension
  use the same resolved configuration as the other Go plugins.
- Go tests pass, including defaults referenced from a spacing callback and
  multiple CSS objects overriding the same prose declaration.

### Additional parser and validation checks

- Replayed 257 Go-fuzzer inputs against the pinned Rust extractor and all 14
  preprocessors: 3,855 additional comparisons pass. Added 35 selector, value,
  attribute and URL cases, and 37 compiler validation cases against Tailwind.
- Those checks found a color fallback difference when a theme reference leads
  to an empty `var()`. The Go output now matches the original compiler.
- Removed an unused selector normalizer and a second implementation of custom
  property shorthand parsing that the candidate parser could never call.
- Full Go tests pass. Coverage across packages is 98.5%; this does not yet
  include the separately instrumented CLI binary. The 100% gate remains open.

### Browser optimization

- Added native property prefixes, legacy mask shorthands, image-set and stretch
  fallbacks, percentage decoration widths, media/container range lowering and
  custom-media expansion with undefined-reference and cycle errors. Transforms
  retain source locations and repeated optimization is idempotent.
- Tightened the independent check: it no longer runs Tailwind's browser
  transforms on Go output. All 849 core outputs and 268 additional browser
  inputs pass in both modes: 2,234 comparisons, including expected errors.
  Comparison preserves ordinary fallback order; it accounts for selector-list
  grouping, movable custom-property definitions and redundant legacy prefixes.
- Saved Go snapshots only after all reference comparisons passed. The Go tests
  verify their input hashes, both output modes, repeated optimization and
  source maps without invoking a reference compiler. Separate raw-output
  assertions check fallbacks that the comparison normalizer can itself supply.
- Full Go tests pass. Whole-project coverage, final performance, the v3 profile
  and Bubblehouse integration remain open.

### Configuration and error recovery

- Expanded browser checks to 287 inputs and 2,272 comparisons. The browser
  implementation now has 100% statement coverage, including recovery paths.
- Expanded additional compiler fixtures from 37 to 60. Fixed empty theme
  variables, default precedence, removed variables reappearing during function
  resolution, the obsolete `@apply ... !important` form, empty plugin variants
  and raw-content configuration diagnostics to match the pinned compiler.
- Added Go API tests for module handlers, configured variants, theme tuples,
  mutable lookup results, plugin option errors and ignored theme-cycle errors.
- `scripts/check-coverage.py` runs the complete suite, combines package and
  instrumented CLI subprocess coverage, and fails below 100%. Current combined
  coverage: 10,695/10,797 statements (99.055%). All tests pass; 102 statements
  remain uncovered. Coverage and integration gates remain open.

### Complete statement coverage

- The combined coverage gate passes: 10,786/10,786 statements (100%), including
  the separately instrumented CLI. The full test suite, race detector and vet
  pass. No coverage exclusions or Go conformance skips were added.
- Added filesystem disappearance and failed-output tests with deterministic
  I/O seams. Failed writes preserve the previous CSS and clean temporary files.
- Expanded Rust parity to 784 extraction inputs and 10,976 preprocessing cases;
  syntax edges to 44 cases, and compiler edges to 89 independent reference cases.
- Fixed container breakpoint duplicates and empty values, malformed variant
  diagnostics, configuration keyframes without a theme insertion point,
  plugin/CSS utility collisions, unused plugin errors and empty scope rules.
- Static plugin definitions are transformed once. Invalid addUtilities variants
  reject a definition; invalid @apply references fail only when that class is
  requested. Direct AddUtility remains strict during Go plugin preparation.
- Relative asset directories now follow Node's working-directory resolution.
  Removed unused arguments and parser-unreachable fallback branches.
- Coverage is complete for the current implementation. Performance, the v3
  compatibility profile, Bubblehouse migration proof and GoX hookup remain open.

### Final standalone allocation pass

- Reused small stack buffers when splitting candidate syntax; reused parsed
  node slices during browser transforms until an inserted prefix needs more
  space. No caching of account values or changes to compiler ownership.
- Three-run medians: 586 candidates take 1.27 ms and 26,630 allocations, down
  from 1.31 ms and 33,056 immediately before this pass. Allocated bytes fell
  from 1.84 MB to 1.75 MB. The initial implementation used 6.74 MB and 109,122
  allocations for the same workload.
- Readable optimization of 40 KB takes 308 us and 4,082 allocations, down from
  322 us and 4,764. Compact output takes 416 us and 7,639 allocations, down
  from 441 us and 8,321. Unchanged incremental builds still allocate nothing.
- Full tests, race tests and all 2,272 independent optimizer comparisons pass.
  The coverage gate passes: 10,803/10,803 statements (100%). Application-level
  measurements will accompany the Bubblehouse integration.

## Initial compiler performance measurements

### Tailwind 3 compatibility foundation

- Added an opt-in profile for Tailwind 3.4.17. Its 361 static utilities and 198
  functional registrations come from original source declarations, not test
  snapshots. The default theme retains references between namespaces, so an
  extended spacing or color setting reaches all dependent utilities.
- Added native old-generation color opacity, gradients, shadows, transforms,
  font tuples, container layout and selector composition. Preserve old hover
  behavior, group/peer selectors, visited-color rules and stacked variant order.
- Added preflight, component/utility layers, theme functions, @screen and @apply
  compilation. Component rules, ordinary utilities and variants retain their
  distinct positions; arbitrary values use Tailwind 3's ambiguity rules.
- Independent Tailwind 3 references pass: 11,344 default named utilities, 841
  variants, 2,759 arbitrary values, two configuration sets and seven stylesheet
  cases. The complete existing Tailwind 4 and CLI suites still pass.
- This is a compatibility checkpoint. CLI selection, remaining configuration
  controls, full new-code coverage and Bubblehouse equivalence are not complete.

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

### Tailwind 3 configuration, CLI and plugins

- Added the `--compat=3` CLI profile, prefix parsing/printing, selector and
  boolean important modes, string/regexp safelists, and named completions.
- Fixed layer-local apply/screen handling, unconditional layer rules and the
  explicit variants insertion point. Standard plugin selectors retain the
  older grouping rules; typography uses the original v3 palette.
- Added original reference builds for five configurations, fourteen stylesheets,
  three complete completion lists and 180 standard-plugin cases. The full v4
  and v3 suites pass. New v3 code still needs complete coverage and the real
  application comparison; those gates remain open.

### Tailwind 3 source scanning and stylesheet behavior

- Ported source extraction and retained the original 45-test extractor file;
  all 45 original tests and captured extraction results pass in Go.
- Added selectable forms 0.5.9 and typography 0.5.15 ports for applications
  using the versions bundled with Tailwind 3.4.17. The default plugins remain
  current. All 258 old-profile plugin comparisons pass.
- Fixed arbitrary-property validation and normalization, stacked pseudo-element
  variants, empty component rules, duplicate declaration handling and important
  behavior across apply expansions.
- A private application snapshot now matches all 2,506 selectors and all
  declarations from the original compiler with browser prefixing disabled.
  Three declaration lists differ only in independent property order. Rule
  ordering and modern browser output still need separate validation.
- Full Go tests pass. Combined coverage is 12,043/12,190 statements (98.794%);
  closing these new paths and application integration remain open.
- Browser prefix upgrades are explicitly desired and permitted output changes.

### Legacy compatibility and application comparison complete

- Added original normalization and duplicate-declaration suites, malformed-input
  regressions, complex configuration, named-template and plugin boundary tests.
  Invalid theme references return errors instead of indexing an empty result.
- Matched old class sorting, registration order, responsive variants and grouped
  component selectors. An independent application comparison finds no missing or
  extra selectors, no value changes and no conflicting cascade-order inversions.
  Three declaration lists differ only in independent longhand order.
- Modern optimization matches the independent current Tailwind optimizer across
  all 2,504 application selectors and declarations. Added transition prefixes and
  removal of obsolete paired prefixes; all 2,310 optimizer comparisons pass.
- Added a native common path for old source scanning and repeated-line reuse.
  The 16.5 MB application scan/build fell from about five seconds to three.
  All original extractor tests and 1,500 deterministic differential inputs pass.
- Full tests, race tests and vet pass. The strict combined coverage gate passes:
  12,300/12,300 statements, including the actual CLI process. TESTING.md records
  the executed suites, profile limits and JavaScript host exceptions precisely.
- Standalone implementation is ready for application integration. The disabled
  runtime-theming proof, application build switch and GoX tenant hookup remain.
