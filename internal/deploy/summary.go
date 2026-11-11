package deploy

import (
	"fmt"
	"strings"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// summary prints the block that closes a deploy. Each piece is yellow on a colour terminal; the host addresses are
// yellow and bold wherever they go. The id, the service name and the addresses are the platform's words.
func (r *runner) summary(deployed platform.DeployResult) {
	if r.t.Mode() != term.Classic {
		r.summaryRich(deployed)
		return
	}
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
	for _, a := range platform.WebhookAddresses(deployed.ApplicationCapabilities) {
		b.WriteString(line(yellow("|"), yellow(a.Label()+":"), term.YellowBold(term.Sanitize(a.Address))))
	}
	b.WriteString(line(yellow(`\-------`)))
	fmt.Fprint(r.t.Out, b.String())
}

// warnNotApplied says on stderr that a block that was sent has no counterpart in the response.
func (r *runner) warnNotApplied(deployed platform.DeployResult) {
	if len(r.p.Manifest.ApplicationCapabilities) == 0 || platform.Applied(deployed.ApplicationCapabilities) {
		return
	}
	fmt.Fprintf(r.t.ErrOut, "%s %s\n", r.c.WarningIcon(), platform.NotAppliedNotice)
}
