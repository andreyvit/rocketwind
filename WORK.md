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
- Core Go suite and race detector pass. Statement coverage: 91.6% at this stage.
- Remaining: complete variants, stylesheet features, dynamic settings, plugins,
  scanning/CLI, existing-CSS exclusion, coverage/performance and integration gates.
- Implementation and conformance gates remain open.
