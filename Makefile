GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=vcr

# Contract harness settings (described in test/contract/doc.go) and the pinned linter version.
BASELINE_REF ?= v2.6.1
PORTED ?= all
NO_GRAPHQL ?= 1
VCR_BIN ?=
SCENARIO ?=
GOLANGCI_LINT_VERSION ?= v2.6.2

build:
	$(GOBUILD) -o $(BINARY_NAME) -v

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)_linux_amd64 -v
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(BINARY_NAME)_linux_arm64 -v

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)_windows_amd64.exe -v

build-macos:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)_macos_amd64 -v
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BINARY_NAME)_macos_arm64 -v

test:
	go test -race ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

## docs: regenerate docs/vcr*.md from the command tree
docs:
	go run ./docs

contract:
	VCR_CONTRACT=1 VCR_BIN='$(VCR_BIN)' PORTED='$(PORTED)' NO_GRAPHQL='$(NO_GRAPHQL)' SCENARIO='$(SCENARIO)' go test ./test/contract/ -run '^TestContract$$' -count=1 -timeout 30m

contract-record:
	VCR_CONTRACT=1 BASELINE_REF='$(BASELINE_REF)' SCENARIO='$(SCENARIO)' go test ./test/contract/ -run '^TestRecord$$' -count=1 -timeout 60m

contract-rich:
	VCR_CONTRACT=1 VCR_BIN='$(VCR_BIN)' PORTED='$(PORTED)' SCENARIO='$(SCENARIO)' go test ./test/contract/ -run '^TestContractRich$$' -count=1 -timeout 30m

contract-rich-record:
	VCR_CONTRACT=1 VCR_BIN='$(VCR_BIN)' SCENARIO='$(SCENARIO)' go test ./test/contract/ -run '^TestRecordRich$$' -count=1 -timeout 30m

contract-baseline:
	@VCR_CONTRACT=1 BASELINE_REF='$(BASELINE_REF)' go test ./test/contract/ -run '^TestBuildBaseline$$' -count=1 -v | sed -n 's/^BASELINE_BIN=//p'

contract-self:
	@bin=$$($(MAKE) -s contract-baseline) && test -n "$$bin" && for i in 1 2 3 4 5; do $(MAKE) contract VCR_BIN="$$bin" NO_GRAPHQL=0 PORTED=all || exit 1; done

contract-mutate:
	@bin=$$($(MAKE) -s contract-baseline) && test -n "$$bin" && VCR_CONTRACT=1 VCR_BIN="$$bin" go test ./test/contract/ -run '^TestMutationsTurnContractRed$$' -count=1 -timeout 30m

contract-baseline-check:
	VCR_CONTRACT=1 go test ./test/contract/ -run '^TestTranscriptsRecordedFromTaggedBaseline$$' -count=1

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)*

run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

deps:
	$(GOGET) -u

.PHONY: build build-linux build-windows build-macos test clean run deps install-macos lint docs contract contract-record contract-rich contract-rich-record contract-baseline contract-self contract-mutate contract-baseline-check

install-macos:
	go build -o $(GOPATH)/bin/vcr main.go
	sudo cp $(GOPATH)/bin/vcr /usr/local/bin/vcr
