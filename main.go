package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Vonage/cloud-runtime-cli/internal/buildinfo"
	"github.com/Vonage/cloud-runtime-cli/internal/cli"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// Set at build time with -ldflags "-X main.<name>=<value>"; each build stamps these names.
var (
	version        = "dev"
	commit         = "0000"
	buildDate      = "2021-09-01T00:00:00Z"
	apiVersion     = "v0.5"
	releaseURL     = "https://api.github.com/repos/Vonage/cloud-runtime-cli"
	upgradeTestKey = ""
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], term.System(), buildInfo())
	stop()
	os.Exit(code)
}

func buildInfo() buildinfo.Info {
	return buildinfo.Info{
		Version:        version,
		Commit:         commit,
		Date:           buildDate,
		APIVersion:     apiVersion,
		ReleaseURL:     releaseURL,
		UpgradeTestKey: upgradeTestKey,
	}
}
