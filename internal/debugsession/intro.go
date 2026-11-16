package debugsession

import (
	"fmt"

	"github.com/Vonage/cloud-runtime-cli/internal/platform"
	"github.com/Vonage/cloud-runtime-cli/internal/term"
)

// introBlock is what the session shows once the proxy is connected, in the layout the old client used. The host is
// always yellow and bold, also when stdout is not a terminal, as before. Each applied webhook address gets a line after
// the host.
func introBlock(style term.Style, serviceName, hostText string, webhooks ...platform.WebhookAddress) string {
	yellow := style.Yellow
	b := "\n" +
		yellow("/-------") + "\n" +
		yellow("| \U0001F41E Debugger proxy connection established - Have a play around!") + "\n" +
		yellow("| Application Name:") + " " + yellow(serviceName) + "\n" +
		yellow("| Application Host:") + " " + term.YellowBold(hostText) + "\n"
	for _, a := range webhooks {
		b += yellow("| "+a.Label()+":") + " " + term.YellowBold(term.Sanitize(a.Address)) + "\n"
	}
	return b + yellow(`\-------`) + "\n\n"
}

// printIntro shows where the debugger can be reached.
func (s *session) printIntro() {
	s.warnNotApplied()
	if s.tio.Mode() != term.Classic {
		s.printIntroRich()
		return
	}
	s.con.stdout(introBlock(s.con.style, term.Sanitize(s.service.ServiceName), term.Sanitize(s.addrs.Host),
		platform.WebhookAddresses(s.service.ApplicationCapabilities)...))
}

// warnNotApplied says on stderr that a block that was sent has no counterpart in the response.
func (s *session) warnNotApplied() {
	if len(s.appCaps) == 0 || platform.Applied(s.service.ApplicationCapabilities) {
		return
	}
	s.con.stderr(fmt.Sprintf("%s %s\n", s.con.style.WarningIcon(), platform.NotAppliedNotice))
}
