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
  imports: 27; compiler lifetime: 1; legacy utilities: 13. All match exactly.
  Total compiler calls: 2,144. Declarative JS test plugins replay through the Go
  API; functional test callbacks have explicit Go ports. No Go conformance skips.
- Original upstream runs: 400 utility tests, 104 variant tests, and 376 tests in
  the eight additional source files pass. One upstream sorting test is already
  skipped upstream. Sorting uses the design-system API and still needs capture coverage.
- Core Go suite, race detector and vet pass. Statement coverage: 94.9%.
- Candidate syntax/normalization: all 2,672 captured calls from 1,388 upstream
  tests pass. CSS generation now uses the same parser exposed by Compiler.Parse.
- Nine more syntax/helper suites: 2,573 calls from 169 upstream tests pass;
  covers values, selectors, attributes, braces, escaping, segmenting, comparison,
  theme paths and shadow colors. Total captured calls checked: 7,677.
- CSS AST/nesting: all 604 calls from 208 upstream tests pass. Total captured
  calls checked: 8,281.
- Typed templates, scoped class-list references, named selectors and opt-in aliases
  implemented. Templates and aliases have 100% statement coverage.
- Remaining: full internal and compatibility test suites,
  standard plugins, scanning/CLI, existing-CSS exclusion, coverage/performance
  and integration gates.
- Implementation and conformance gates remain open.
