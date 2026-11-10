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
//   - testdata/transcripts/<id>.txt: the transcript recorded for each scenario
//     that has a classic leg.
//   - testdata/rich/<id>.txt: the rich golden of each scenario that has a rich
//     leg: the exit and the final screen of the interactive presentation,
//     recorded from the binary under test.
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
// # Presentations
//
// A process whose stdout and stderr are terminals shows the interactive
// presentation unless VCR_UI contains classic, CI is set or TERM is dumb. A
// terminal scenario therefore runs in the presentations its presentation: key
// names:
//
//   - both (the default for tty: true) runs the scenario twice. The classic
//     leg adds VCR_UI=classic to the environment and is compared with the
//     transcript like any other scenario. The rich leg adds nothing, so the
//     process shows what a person at a terminal sees, and is compared with the
//     rich golden. Both legs drive the same steps, so every expect must hold
//     in both; a scenario that waits for a line only one presentation prints
//     is written twice, as a classic and a rich scenario. A both scenario
//     cannot set VCR_UI, CI, CLICOLOR_FORCE or TERM in its env.
//   - classic (the default for a scenario without a terminal) runs once, in
//     make contract, with the environment exactly as the scenario states. The
//     harness adds nothing, not even VCR_UI.
//   - rich runs once, in make contract-rich, with the environment as the
//     scenario states. It has no transcript, so only the exit and the screen
//     (against the golden), the reads and the raw bytes are checked.
//
// The rich leg is checked in sub-tests of its own:
//
//   - rich/stdout: the final screen, passed through the normaliser and then
//     stripped of the duration that a resolved line shows from two seconds on,
//     equals the golden. No allowed diff excuses it.
//   - rich/exit: the exit equals the golden's and the transcript's.
//   - rich/files and rich/requests: the files and the requests equal those of
//     the transcript, with the same allowed diffs.
//   - rich/reads: the reads equal the scenario's expect-reads, and the binary
//     sends no GraphQL request.
//   - rich/raw: the bytes the terminal delivered leave the cursor shown, use
//     no alternate screen or mouse mode, and hold no colour or style sequence
//     when NO_COLOR is set (a scenario that clears it must show colour).
//
// A rich scenario has no transcript, so rich/files and rich/requests are
// skipped for it. A scenario that needs a fix which its transcript predates
// skips rich/exit, rich/files and rich/requests too, as the classic leg skips
// the whole scenario. rich/reads and rich/raw need no transcript and always
// run.
//
// A rich golden is a regression pin recorded from the binary under test, not
// an oracle of the baseline: a person reviews it whenever it changes.
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
// make contract replays the transcripts (the classic leg of every scenario)
// and make contract-record records them again. make contract-rich replays the
// rich leg of the terminal scenarios and make contract-rich-record records
// their goldens, twice per scenario, failing when the two screens differ; it
// never touches the transcripts. make contract-self replays a baseline build
// against its own transcripts five times, make contract-mutate checks that
// deliberately altered scenarios and transcripts turn the replay red, and
// make contract-baseline-check checks that every transcript was recorded from
// one and the same release tag.
package contract
