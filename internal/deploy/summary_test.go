package deploy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// finalBlock returns what Run printed from the first line of the block on.
func finalBlock(stdout string) string {
	_, block, _ := strings.Cut(stdout, "\n/-------\n")
	return "/-------\n" + block
}

func TestFinalBlockWithoutHostURLs(t *testing.T) {
	h := newHarness(t)
	h.api.deployed.HostURLs = nil

	err := h.run()

	require.NoError(t, err)
	assert.Equal(t, "/-------\n"+
		"| Instance has been deployed!\n"+
		"| Instance id: i-1\n"+
		"| Instance service name: my-project-dev\n"+
		`\-------`+"\n", finalBlock(h.stdout.String()))
}

func TestFinalBlockSanitisesServerText(t *testing.T) {
	h := newHarness(t)
	h.api.deployed.InstanceID = "i-\x1b[2J1"
	h.api.deployed.ServiceName = "my-\x1b]0;title\aproject-dev"
	h.api.deployed.HostURLs = []string{"https://one.\x1b[31mexample.test"}

	err := h.run()

	require.NoError(t, err)
	assert.Equal(t, "/-------\n"+
		"| Instance has been deployed!\n"+
		"| Instance id: i-1\n"+
		"| Instance service name: my-project-dev\n"+
		hostLine("https://one.example.test")+
		`\-------`+"\n", finalBlock(h.stdout.String()))
}
