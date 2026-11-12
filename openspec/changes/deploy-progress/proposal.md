# Proposal

Branch: `feature/application-capabilities`, after the `application-capabilities` and `capabilities-generator` changes
in the same pull request.

## Why

On a terminal, `vcr deploy` streams every line of the image build: dozens of Kaniko lines about manifests, snapshots
and lookups, with the output of the user's build script lost among them. Nothing says which part of the build is
running, and the closing summary is a plain list of `Label: value` lines that is hard to scan, more so now that it
lists webhook addresses.

## What Changes

- On a terminal (the rich presentation), the build step's live line shows the build phase and a bar that fills one
  segment per phase: waiting to start, preparing source, pulling base image, mounting filesystem, copying source,
  running build script, saving image, pushing image. The CLI tells the phases apart by the fixed opening words of
  the Kaniko lines.
- Only the build script's output is shown, as a rolling window of its last 8 lines under the live line. When the
  build succeeds the window is erased and only `✓ Package built` remains.
- When the build fails or times out, the CLI prints the whole build log it held back, Kaniko lines included.
- The deploy summary becomes two boxes: the deployed instance (URL, instance id, service name) under the title
  `<project> is live`, and the applied webhooks grouped by capability with their HTTP method.
- With `VCR_UI=static`, the filtered build script lines are printed as they arrive, with no bar and no erasing; the
  summary boxes are the same as in the rich presentation.
- Classic output (CI, pipes, `VCR_UI=classic`) does not change at all.

## Capabilities

### New Capabilities

- `deploy-progress`: how `vcr deploy` shows the build and the deployed instance on a terminal.

### Modified Capabilities

- `deploy-build-outcome`: the build log stream requirement says frames are shown in full only in classic output;
  the terminal presentations follow `deploy-progress`.
- `application-capabilities`: the webhook address lines stay as they are in classic output; on a terminal the
  addresses appear in the webhooks box.

## Impact

- `internal/deploy` (build step and summary in the rich presentation), `internal/term` (a live line with a phase bar
  and a rolling window, a boxed block), `internal/platform/appcaps.go` (the HTTP method next to each address).
- Contract rich goldens for deploy change; new rich scenarios for a build with Kaniko lines, a failed build and a
  deploy with webhooks. Classic transcripts do not change.
- No new dependency.

## Rollout

Ships with the release that carries the `application-capabilities` change. `VCR_UI=classic` restores the old output
on a terminal.

## Out of scope

- Structured build frames from the platform (a phase or source field per frame): needs a server change, and older
  platforms would never send it.
- A real percentage for the bar: the platform reports no build progress.
- `vcr debug`'s summary: it has its own presentation, and this change is about deploy.
- A flag to show the full build log on success: the classic presentation (`VCR_UI=classic`) already does.
