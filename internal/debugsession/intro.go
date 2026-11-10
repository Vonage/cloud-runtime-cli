package debugsession

import "github.com/Vonage/cloud-runtime-cli/internal/term"

// introBlock is what the session shows once the proxy is connected, in the layout the old client used. The host is
// always yellow and bold, also when stdout is not a terminal, as before.
func introBlock(style term.Style, serviceName, hostText string) string {
	yellow := style.Yellow
	return "\n" +
		yellow("/-------") + "\n" +
		yellow("| \U0001F41E Debugger proxy connection established - Have a play around!") + "\n" +
		yellow("| Application Name:") + " " + yellow(serviceName) + "\n" +
		yellow("| Application Host:") + " " + term.YellowBold(hostText) + "\n" +
		yellow(`\-------`) + "\n\n"
}

// printIntro shows where the debugger can be reached.
func (s *session) printIntro() {
	if s.tio.Mode() != term.Classic {
		s.printIntroRich()
		return
	}
	s.con.stdout(introBlock(s.con.style, term.Sanitize(s.service.ServiceName), term.Sanitize(s.addrs.Host)))
}
