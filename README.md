# Rocketwind

A programmable CSS compiler for Go, targeting Tailwind CSS 4.3.4.

Under development. Compatibility and coverage are measured gates, not claims
about the current implementation. See [WORK.md](WORK.md) for progress.

The compiler will support utility stylesheets, named CSS classes, typed values
resolved by the caller, programmatic themes, and exclusion of utilities supplied
by existing stylesheets. The Go library and CLI do not execute JavaScript.

Upstream tests and source-derived data retain Tailwind Labs' MIT license.
