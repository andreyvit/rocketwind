# Original Tailwind CLI integration tests

Captured from all four `integrations/cli/*.test.ts` files in Tailwind CSS
v4.3.4, commit `2626ae6ceddf36069ab6d55c53a5857c1e2e360a`. Source hashes are
in `../manifest.json`; the MIT license is in `../LICENSE`.

The original Vitest assertions pass: 68 tests pass, and two source-map tests
are already skipped upstream. `scripts/capture-cli-integration.mjs` runs the
release CLI and standalone binary with the pinned source checkout's CSS.
The npm package reformats that CSS, so using its stylesheets would change an
original snapshot's quote style. The capture does not change assertions.

The gzip files retain file writes, deletions, symlinks, commands, watch events,
CSS checks and source-map position checks. Temporary paths and durations are
normalized; unused package-install logs are omitted. `cli/integration_test.go`
replays these operations against Go. Its self-scanning case builds and runs
the real Rocketwind binary inside the scanned directory.

64 cases run in Go. The four JavaScript exceptions are named in the replay:

* CLI and standalone module-resolution tests for CJS, ESM, CTS and MTS.
* JavaScript plugin API exports and `typeof` checks.
* Execution of the third-party `tailwindcss-animate` JavaScript plugin.

Other test plugins and configuration callbacks have explicit Go ports.
The TypeScript compiler step is excluded, but that test's plugin behavior and
CSS assertions run in Go. None of Rocketwind's runtime code executes JavaScript.
