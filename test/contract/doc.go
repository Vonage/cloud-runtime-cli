// Package contract checks that a build of vcr behaves like the baseline
// release, by replaying recorded command runs against a fake platform.
//
// Each scenario runs one vcr command in a sandbox of its own (home directory,
// working directory and environment) against test/fakeplatform. The recorder
// runs every scenario with a binary built from the baseline and writes what it
// saw to a transcript: stdout, stderr, the exit code, the files the command
// wrote and the requests it sent. The replayer runs the same scenarios with
// the binary under test and compares each of those fields with the
// transcript. A field may differ only where allowed-diffs.txt says so.
//
// The replay and record tests run on Unix only; this file has no build
// constraint so that the package is never empty.
//
// # Layout
//
//   - scenarios/<id>.yaml: one scenario per file. Its group is its group: key,
//     or else the part of the id before the first "-". The groups are version,
//     help, completion, root, config, bootstrap, app, secret, instance,
//     deploy, init, debug, configure and upgrade.
//   - testdata/transcripts/<id>.txt: the transcript recorded for each scenario.
//   - testdata/contract-tree.json: every command of the baseline with its use
//     line, aliases, help texts, flags and subcommands.
//   - testdata/golden-help/: the --help output of every command, one file per
//     command path with spaces replaced by "_" (vcr_app_create.txt).
//   - testdata/golden-misc.txt: reference recordings of version, help and
//     error output from an earlier release; the version line is compared by
//     format only.
//   - testdata/corpora/: input files (archives, manifests, config files,
//     ignore files) that scenarios copy into their sandbox.
//   - coverage.txt: the areas of the CLI contract and the scenarios that cover
//     each, one "<area>: <glob> <glob> ..." line per area; a test fails when a
//     glob matches no scenario or a scenario is in no row.
//   - allowed-diffs.txt: the deliberate differences from the baseline, one
//     "<scenario-glob> <field> <reason>" line each.
//   - scraped.yaml: the output lines that other tools parse, checked by name.
//   - testapp/: a small program that stands in for the application that
//     vcr debug runs.
//
// # Terminal scenarios
//
// A scenario with tty: true runs on a pseudo-terminal of 100 columns by 40
// rows, with stdin, stdout and stderr all on it. Its steps are expect (wait
// until the screen text holds some text), expect-any (wait until it holds any
// one of several texts, for a prompt that differs between builds) and send
// (type some keys); a send follows an expect or an expect-any, except Ctrl-C
// and Ctrl-D, which end the run or the typed input. vt.go keeps the screen a
// person would see, so spinner frames, redraws and colours leave no trace, and
// answers the cursor position queries of the prompt library. The transcript's
// stdout is the final screen text.
//
// The binaries the harness builds go to .cache/contract/ at the repository
// root, which git ignores.
//
// # Environment
//
//   - VCR_CONTRACT=1 opts in. Without it the replay and record tests skip, so
//     a plain go test ./... never runs them.
//   - VCR_BIN is the binary to replay. When it is empty, the harness builds
//     the repository root.
//   - VCR_FAKE_PORT is the one port the fake platform listens on during a
//     run. When it is empty, the harness uses the port in the <binary>.port
//     file it writes next to each binary it builds, or else a free port.
//   - PORTED selects the groups to replay: all, or a comma- or
//     space-separated list of groups. Empty means all.
//   - NO_GRAPHQL=1 says the binary under test must not use GraphQL: any
//     /graphql request fails the scenario, and the read requests it sends are
//     checked against the scenario's expect-reads list. Set it to 0 to replay
//     a baseline build.
//   - SCENARIO is a glob over scenario ids. Empty means every scenario.
//   - BASELINE_REF is the git tag or commit that transcripts are recorded
//     from. It defaults to v2.6.1.
//
// # Make targets
//
// make contract replays the transcripts and make contract-record records them
// again. make contract-self replays a baseline build against its own
// transcripts five times, make contract-mutate checks that deliberately
// altered scenarios and transcripts turn the replay red, and
// make contract-baseline-check checks that every transcript was recorded from
// one and the same release tag.
package contract
