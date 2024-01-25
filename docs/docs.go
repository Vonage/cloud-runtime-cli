package main

import (
	"log"

	"github.com/spf13/cobra/doc"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
	"vonage-cloud-runtime-cli/vcr/root"
)

func main() {
	f := cmdutil.NewDefaultFactory("v0.3", "https://api.github.com/repos/Vonage/vonage-cloud-runtime-cli")
	updateMessageChan := make(chan string)
	rootCmd := root.NewCmdRoot(f, "dev", "2021-09-01T00:00:00Z", "0000", updateMessageChan)
	err := doc.GenMarkdownTree(rootCmd, "./docs")
	if err != nil {
		close(updateMessageChan)
		log.Fatal(err)
	}
	close(updateMessageChan)
}
