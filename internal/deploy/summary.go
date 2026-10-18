package deploy

import (
	"fmt"
	"strings"

	"vonage-cloud-runtime-cli/internal/platform"
	"vonage-cloud-runtime-cli/internal/term"
)

// summary prints the block that closes a deploy. Each piece is yellow on a colour terminal; the host addresses are
// yellow and bold wherever they go. The id, the service name and the addresses are the platform's words.
func (r *runner) summary(deployed platform.DeployResult) {
	yellow := r.c.Yellow
	line := func(pieces ...string) string { return strings.Join(pieces, " ") + "\n" }

	var b strings.Builder
	b.WriteString(line(yellow("/-------")))
	b.WriteString(line(yellow("|"), yellow("Instance has been deployed!")))
	b.WriteString(line(yellow("|"), yellow("Instance id:"), yellow(term.Sanitize(deployed.InstanceID))))
	b.WriteString(line(yellow("|"), yellow("Instance service name:"), yellow(term.Sanitize(deployed.ServiceName))))
	for _, url := range deployed.HostURLs {
		b.WriteString(line(yellow("|"), yellow("Instance host address:"), term.YellowBold(term.Sanitize(url))))
	}
	b.WriteString(line(yellow(`\-------`)))
	fmt.Fprint(r.t.Out, b.String())
}
