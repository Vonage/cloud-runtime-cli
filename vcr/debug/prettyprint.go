package debug

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cli/cli/v2/pkg/iostreams"

	"vonage-cloud-runtime-cli/pkg/cmdutil"
)

var (
	c      = iostreams.System().ColorScheme()
	yellow = c.ColorFromString("yellow")

	white     = c.ColorFromString("white")
	whiteBold = c.ColorFromString("bold")

	red     = c.ColorFromString("red")
	verbose = false
)

func logIntroMessage(appName, host2 string) {
	fmt.Println()
	fmt.Println(yellow(`/-------`))
	fmt.Println(yellow("| 🐞 Debugger proxy connection established - Have a play around!"))
	fmt.Println(yellow("| Application Name:"), yellow(appName))
	fmt.Println(yellow("| Application Host:"), cmdutil.YellowBold(host2))
	fmt.Println(yellow(`\-------`))
	fmt.Println()
}

func logErrorMessage(err error) {
	if verbose {
		fmt.Println()
		fmt.Println(red(fmt.Sprintf(`%s  Error from CLI`, c.FailureIcon())))
		logStringField("ERROR", err.Error())
		fmt.Println()
	}
}

func logInboundRequest(req websocketRequestMessage) {
	if verbose {
		header := flattenMapForPrinting(req.Headers)
		query := flattenMapForPrinting(req.Query)
		fmt.Println()
		fmt.Println(c.GreenBold(`➡️  Inbound Request to client app`))
		logStringField("ID", req.ID)
		logStringField("Method", req.Method)
		logStringField("Path", req.Route)
		logStringField("Query", query)
		logStringField("Headers", header)
		logPayload("Payload", req.Payload)
		fmt.Println()
	}
}

func logOutboundResponse(resp websocketResponseMessage) {
	if verbose {
		header := flattenMapForPrinting(resp.Headers)
		fmt.Println()
		fmt.Println(c.GreenBold(`⬅️️️  Outbound Response from client app`))
		logStringField("ID", resp.ID)
		logStringField("Status", fmt.Sprint(resp.Status))
		logStringField("Header", header)
		logPayload("Payload", resp.Payload)
		fmt.Println()
	}

}

func logOutboundRequest(req websocketRemoteRequestMessage) {
	if verbose {
		header := marshalIndent(req.Headers)
		query := flattenMapForPrinting(req.Query)
		fmt.Println()
		fmt.Println(c.CyanBold(`⬅️️️  Outbound Request to Provider`))
		logStringField("ID", req.ID)
		logStringField("Provider", strings.TrimSuffix(req.Request.FAASFunction, ".neru"))
		logStringField("Headers", header)
		logStringField("Query", query)
		logPayload("Payload", req.Request.Payload)
		fmt.Println()
	}
}

func logInboundResponse(resp websocketResponseMessage) {
	if verbose {
		header := flattenMapForPrinting(resp.Headers)
		fmt.Println()
		fmt.Println(c.CyanBold(`➡️  Inbound Response from provider`))
		logStringField("ID", resp.ID)
		logStringField("Status", fmt.Sprint(resp.Status))
		logStringField("Headers", header)
		logPayload("Payload", resp.Payload)
		fmt.Println()
	}
}

func logStringField(name, value string) {
	fmt.Println("   ", white(name+":"), whiteBold(value))
}

func logPayload(name string, payload []byte) {
	if len(payload) == 0 {
		return
	}
	if len(payload) > 4*1024 {
		fmt.Println("   ", white(name+":"), whiteBold(`[Too big to display]`))
	}
	if !utf8.ValidString(string(payload)) {
		fmt.Println("   ", white(name+":"), whiteBold(`[Binary payload]`))
	}
	var v interface{}
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		fmt.Println("   ", white(name+":"), whiteBold(string(payload)))
		return
	}
	fmt.Println("   ", white(name+":"), whiteBold(marshalIndent(v)))
}

func flattenMapForPrinting(m map[string][]string) string {
	fm := make(map[string]interface{})
	for k, v := range m {
		if len(v) == 1 {
			fm[k] = v[0]
		} else {
			fm[k] = v
		}
	}
	return marshalIndent(fm)
}

func marshalIndent(v interface{}) string {
	//nolint
	buf, _ := json.MarshalIndent(v, "      ", "  ")
	return string(buf)
}
