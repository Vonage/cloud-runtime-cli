GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=vcr

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
	$(GOTEST) -v ./...

test-unit:
	$(GOTEST) -v ./...

test-integration: test-integration-build
	cd tests/integration && docker compose up --build --exit-code-from cli-tool --abort-on-container-exit

test-integration-build:
	@mkdir -p tests/integration/bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "-X main.releaseURL=http://mockserver:80" -o tests/integration/bin/vcr-cli .
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o tests/integration/bin/mockserver ./tests/integration/mocks/main.go

test-integration-clean:
	cd tests/integration && docker compose down -v --remove-orphans
	rm -rf tests/integration/bin/vcr-cli tests/integration/bin/mockserver

test-all: test-unit test-integration

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)*

run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

deps:
	$(GOGET) -u

.PHONY: build build-linux build-windows build-macos test test-unit test-integration test-integration-build test-integration-clean test-all clean run deps install-macos

install-macos:
	go build -o $(GOPATH)/bin/vcr main.go
	sudo cp $(GOPATH)/bin/vcr /usr/local/bin/vcr