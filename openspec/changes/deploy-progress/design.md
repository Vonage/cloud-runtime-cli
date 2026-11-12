# Design

## Context

- Build frames reach the CLI as plain strings over the build-watch websocket (`platform.Client.WatchBuild`). The
  platform reduces Kaniko's logrus lines to their message and passes the build script's output through raw, then
  prefixes every frame with a magenta `ℹ `. Nothing in a frame says which container or phase it came from.
- `internal/deploy/rich.go` `watchBuildRich` writes every frame to `term.IO.Gutter()`. The live line is a single row
  drawn on stderr by `internal/term/surface.go`, which erases it (`\r\x1b[2K`) before any write and draws it again
  after.
- `summaryRich` prints a plain `term.Block` so that the `Instance …:` lines stay greppable. The contract scrape
  checks run on classic transcripts only, so the rich summary is free to change.
- `TestStaticDeployMatchesRichScreen` holds the static screen equal to the rich one.

## Goals / Non-Goals

**Goals:** phase tracking and filtering that work against every platform version today; a live region of more than
one row that still leaves the screen clean; a summary that fits any terminal width without cutting a URL.

**Non-Goals:** any change to classic output, to the build outcome logic in `build.go`, or to `vcr debug`.

## Decisions

### A build-log reader in `internal/deploy`

A new `buildLog` type takes each frame (after `term.SanitizeStream` and the leading `ℹ ` removed) and returns
what to do with it: its phase, whether it is script output, and it records every frame in arrival order for the
failure dump. Prefix matching follows the spec table; the phase index only increases. It is pure, so the spec
scenarios become a table test.

- Rejected: regular expressions per Kaniko line: prefixes are enough and easier to read.
- Rejected: structured frames from the platform: needs a server change and old platforms would never send them.

### The live line grows a phase bar and a tail

`term.Step` gains two methods used only by the build step: `SetProgress(done, total int, label string)` and
`Tail(lines []string)`. The surface draws, for the top step, the live row and then up to 8 tail rows, and erases
the whole region by moving the cursor up (`\x1b[<n>A`) and clearing to the end of the screen (`\x1b[J`) before any
write. Tail rows and the phase-bar row are cut to width−1 terminal cells, measured with `go-runewidth` (already in
the module graph) so that a wide rune such as CJK text or an emoji takes 2; no row wraps and the row count stays
exact. The bar is 7
segments of 2 cells (`━━` filled, `──` dim for the rest) and is dropped under 60 columns. Static ignores both methods; the build step
writes script lines to the gutter itself there.

- Rejected: printing script lines to stdout and erasing them later: lines that scrolled off cannot be erased.
- Rejected: a TUI library (bubbletea): a new dependency and a second rendering model next to the surface.

### The failure dump

On a `failed` status or a timeout, `watchBuildRich` stops the step (erasing the region) and writes the recorded
frames to the gutter before returning the error. In static it writes only the frames it did not print. On
`context.Canceled` it prints nothing. Memory is bounded by keeping the last 2,000 frames, with a dim
`… <n> earlier lines` first when more arrived.

- Rejected: printing only the Kaniko lines on failure: the order between script and Kaniko lines matters to read
  an error.

### Boxes in `internal/term`

A new `term.Box(title string, sections [][]Row)`-style helper renders a rounded box sized to its widest line, or
the borderless fallback when that line does not fit `columns()`. Row values are measured without escape sequences.
The deploy summary builds two boxes; the webhooks box groups `platform.WebhookAddresses` by the first path
segment, which now also carries the sibling `http_method` (`WebhookAddress.Method`). Capability bullets take a
colour from a fixed list in capability-name order (voice magenta, messages blue, rtc yellow, video cyan, verify
green, others bold), which needs a new `Style.Magenta`.

- Rejected: reusing `term.Block` with a flag: its plain mode exists for greppable lines, which this drops.
- Rejected: cutting long URLs to fit: people copy them.

### Static matches rich except for the build lines

The static screen keeps the script lines the rich screen erased, so `TestStaticDeployMatchesRichScreen` compares
the screens with the static gutter lines removed.

## Risks / Trade-offs

- [A Kaniko upgrade rewords its lines] → The phase label lags, or a Kaniko line shows in the script window; the
  outcome is never affected, and the matching prefixes sit in one table in one file.
- [A multi-row live region misbehaves on an odd terminal] → `VCR_UI=static` and `VCR_UI=classic` already exist as
  escape hatches; the region is erased before every write, as the single row is today.
- [The terminal is resized while the tail is drawn] → Rows are re-cut to the new width on every redraw; a shrink
  between draws can leave a wrapped row behind once, the same as the live line today.
