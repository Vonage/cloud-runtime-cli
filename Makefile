SHELL := /bin/bash
GO    ?= go

# Contract harness settings (described in test/contract/doc.go) and the pinned linter version.
BASELINE_REF ?= v2.6.1
PORTED ?= all
NO_GRAPHQL ?= 1
VCR_BIN ?=
SCENARIO ?=
# Built from source by `make lint` with the Go that go.mod selects, so pick a release that loads
# packages under that Go (a linter built with an older Go panics on a newer standard library).
GOLANGCI_LINT_VERSION ?= v2.15.0

BIN_DIR     := dist
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse HEAD 2>/dev/null || echo 0000)
BUILD_DATE  ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
API_VERSION ?= v0.5
GOVULNCHECK_VERSION ?= v1.8.0
RELEASE_URL ?= https://api.github.com/repos/Vonage/cloud-runtime-cli
TARGETS     := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

ifeq ($(strip $(API_VERSION)),)
$(error API_VERSION must not be empty)
endif

LDFLAGS = -s -w -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.buildDate=$(BUILD_DATE)' \
	-X 'main.apiVersion=$(API_VERSION)' -X 'main.releaseURL=$(RELEASE_URL)'
BUILD   = CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)"

## build: build dist/vcr for this machine with the release stamp
build:
	@mkdir -p $(BIN_DIR)
	$(BUILD) -o $(BIN_DIR)/vcr .

## build-all: cross-compile the five release binaries into dist/
build-all:
	@mkdir -p $(BIN_DIR)
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ "$$os" = windows ] && ext=".exe"; \
		echo "==> $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch $(BUILD) -o "$(BIN_DIR)/vcr_$${os}_$${arch}$$ext" . || exit 1; \
	done

## test: run every unit test with the race detector
test:
	go test -race ./...

## lint: run the pinned golangci-lint natively and for windows (one run checks one build context)
lint:
	@bin=$$(mktemp -d) && GOBIN="$$bin" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) \
		&& "$$bin"/golangci-lint --version \
		&& "$$bin"/golangci-lint run ./... \
		&& GOOS=windows CGO_ENABLED=0 "$$bin"/golangci-lint run ./...; \
	rc=$$?; rm -rf "$$bin"; exit $$rc

## vulncheck: report known vulnerabilities in what the module links
# ci.yml and vulncheck-weekly.yml run this same command (and version) directly: make reports every failure as status 2, and they need govulncheck's 3.
vulncheck:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

## fmt: format every Go file
fmt:
	$(GO) fmt ./...

## tidy: tidy go.mod and go.sum
tidy:
	$(GO) mod tidy

## docs: regenerate docs/vcr*.md from the command tree
docs:
	go run ./docs

## schema: regenerate schema/vcr.schema.json from the Vonage Application API v2 definition (SPEC=<file>)
schema:
	@test -n '$(SPEC)' || { echo "SPEC is required: make schema SPEC=<the Vonage Application API v2 definition>"; exit 1; }
	$(GO) run ./tools/schemagen -spec '$(SPEC)' -out schema/vcr.schema.json

## contract: replay the recorded transcripts (PORTED, SCENARIO and VCR_BIN narrow or redirect the run)
contract:
	VCR_CONTRACT=1 VCR_BIN='$(VCR_BIN)' PORTED='$(PORTED)' NO_GRAPHQL='$(NO_GRAPHQL)' SCENARIO='$(SCENARIO)' go test ./test/contract/ -run '^TestContract$$' -count=1 -timeout 30m

## contract-record: record the transcripts again from BASELINE_REF
contract-record:
	VCR_CONTRACT=1 BASELINE_REF='$(BASELINE_REF)' SCENARIO='$(SCENARIO)' go test ./test/contract/ -run '^TestRecord$$' -count=1 -timeout 60m

## contract-rich: replay the terminal scenarios against the rich goldens
contract-rich:
	VCR_CONTRACT=1 VCR_BIN='$(VCR_BIN)' PORTED='$(PORTED)' SCENARIO='$(SCENARIO)' go test ./test/contract/ -run '^TestContractRich$$' -count=1 -timeout 30m

## contract-rich-record: record the rich goldens again
contract-rich-record:
	VCR_CONTRACT=1 VCR_BIN='$(VCR_BIN)' SCENARIO='$(SCENARIO)' go test ./test/contract/ -run '^TestRecordRich$$' -count=1 -timeout 30m

## contract-baseline: build the BASELINE_REF binary and print its path
contract-baseline:
	@VCR_CONTRACT=1 BASELINE_REF='$(BASELINE_REF)' go test ./test/contract/ -run '^TestBuildBaseline$$' -count=1 -v | sed -n 's/^BASELINE_BIN=//p'

## contract-self: replay the transcripts against the baseline binary five times
contract-self:
	@bin=$$($(MAKE) -s contract-baseline) && test -n "$$bin" && for i in 1 2 3 4 5; do $(MAKE) contract VCR_BIN="$$bin" NO_GRAPHQL=0 PORTED=all || exit 1; done

## contract-mutate: check that deliberate mutations turn the replay red
contract-mutate:
	@bin=$$($(MAKE) -s contract-baseline) && test -n "$$bin" && VCR_CONTRACT=1 VCR_BIN="$$bin" go test ./test/contract/ -run '^TestMutationsTurnContractRed$$' -count=1 -timeout 30m

## contract-baseline-check: check that the transcripts came from the tagged baseline
contract-baseline-check:
	VCR_CONTRACT=1 go test ./test/contract/ -run '^TestTranscriptsRecordedFromTaggedBaseline$$' -count=1

# `make hooks` runs the hooks from this clone's own hooks directory. It is found through the common git
# directory, as a physical path, so every worktree of the clone computes the same string. When
# core.hooksPath points every repository at a shared directory, `make hooks` overrides that for this
# clone and writes a forwarding shim for each executable shared hook whose name lefthook does not use,
# so those keep running. A shared hook that has a name lefthook installs itself (pre-commit,
# commit-msg, pre-push, prepare-commit-msg) is not forwarded: a warning names it. Every shim carries
# SHIM_MARK, which is how both targets find exactly their own files. A shim does nothing when it is
# entered again from the hook it forwards to, and nothing when that hook is gone, so it can neither
# loop nor block. The shims are written even when lefthook fails.
SHIM_MARK  := shared-hook shim written by make hooks
HOOKS_DIR   = $$(cd "$$(git rev-parse --git-common-dir)" && pwd -P)/hooks
LOCAL_PATH  = $$(p=$$(git config --local --get core.hooksPath || true); if [ -d "$$p" ]; then (cd "$$p" && pwd -P) || printf '%s' "$$p"; else printf '%s' "$$p"; fi)
RM_SHIMS    = for t in "$$hooks"/*; do if [ -f "$$t" ] && grep -qF "$(SHIM_MARK)" "$$t"; then rm -f "$$t"; fi; done

## hooks: install the git hooks, once per clone
hooks:
	@command -v lefthook >/dev/null || { echo "install lefthook: go install github.com/evilmartians/lefthook@latest"; exit 1; }
	@command -v npm >/dev/null || { echo "install Node.js 20 (the commit-message hook runs npx commitlint)"; exit 1; }
	@set -eu; \
	hooks="$(HOOKS_DIR)"; \
	current="$(LOCAL_PATH)"; \
	shared=$$(git config --global --includes --type=path --get core.hooksPath || git config --system --includes --type=path --get core.hooksPath || true); \
	forward=; \
	if [ -n "$$shared" ] && [ "$$shared" != "$$hooks" ] && { [ -z "$$current" ] || [ "$$current" = "$$hooks" ]; }; then \
		echo "core.hooksPath is $$shared (shared by every repository here); using $$hooks for this clone and forwarding the shared hooks that lefthook does not replace"; \
		git config --local core.hooksPath "$$hooks"; \
		$(RM_SHIMS); \
		forward=1; \
	fi; \
	rc=0; lefthook install || rc=$$?; \
	if [ -n "$$forward" ]; then \
		mkdir -p "$$hooks"; \
		for f in "$$shared"/*; do \
			[ -f "$$f" ] && [ -x "$$f" ] || continue; \
			t="$$hooks/$${f##*/}"; \
			if [ -e "$$t" ]; then \
				echo "warning: $$t exists, so the shared $${f##*/} hook does not run in this clone" >&2; \
				continue; \
			fi; \
			printf '#!/bin/sh\n# %s\n[ -z "$${SHARED_HOOK_SHIM:-}" ] || exit 0\n[ -x "%s" ] || exit 0\nSHARED_HOOK_SHIM=1 exec "%s" "$$@"\n' "$(SHIM_MARK)" "$$f" "$$f" >"$$t"; \
			chmod +x "$$t"; \
		done; \
	fi; \
	[ "$$rc" -eq 0 ] || exit "$$rc"; \
	npm ci --ignore-scripts

## hooks-uninstall: remove the git hooks from this clone
hooks-uninstall:
	@command -v lefthook >/dev/null || { echo "lefthook is not installed, so its hooks cannot be removed"; exit 1; }
	@set -eu; \
	hooks="$(HOOKS_DIR)"; \
	current="$(LOCAL_PATH)"; \
	$(RM_SHIMS); \
	lefthook uninstall; \
	if [ "$$current" = "$$hooks" ]; then git config --local --unset core.hooksPath; fi

## clean: remove build output
clean:
	rm -rf $(BIN_DIR)

## help: list the targets
help:
	@grep -hE '^## [a-z-]+:' $(MAKEFILE_LIST) | sed 's/^## //' | awk -F': ' '{printf "  %-24s %s\n", $$1, $$2}'

.PHONY: build build-all test lint vulncheck fmt tidy docs schema contract contract-record contract-rich contract-rich-record contract-baseline contract-self contract-mutate contract-baseline-check hooks hooks-uninstall clean help
