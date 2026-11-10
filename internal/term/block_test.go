package term

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var debuggerRows = []Row{{Label: "Service", Value: "my-svc"}, {Label: "Host", Value: "my-svc.example.test"}}

func TestBlockLayout(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)
	style := streams.Style()

	streams.Block(style.SuccessIcon(), "Debugger ready", debuggerRows, false)

	assert.Equal(t, "✓ Debugger ready\n  Service  my-svc\n  Host     my-svc.example.test\n", stdout.Screen())
	raw := stdout.Raw()
	assert.Contains(t, raw, style.Bold("Debugger ready"))
	assert.Contains(t, raw, style.Gray("Service"))
	assert.Contains(t, raw, style.Gray("Host"))
}

func TestBlockPlainRowsHaveNoEscapes(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)
	rows := []Row{{Label: "Instance id", Value: "abc"}, {Label: "Instance service name", Value: "s"}}

	streams.Block(streams.Style().SuccessIcon(), "Instance has been deployed!", rows, true)

	assert.Equal(t, "✓ Instance has been deployed!\n  Instance id: abc\n  Instance service name: s\n", stdout.Raw())
}

func TestBlockValuesAreNeverTruncated(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 40)
	value := "https://" + strings.Repeat("a", 82)

	streams.Block("", "Result", []Row{{Label: "Host", Value: value}}, false)

	assert.Equal(t, "Result\n  Host  "+value+"\n", stdout.Screen())
	assert.Len(t, value, 90)
}

func TestBlockWithoutIconOrRows(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)
	style := streams.Style()

	streams.Block("", "Title", []Row{{Label: "A", Value: "1"}, {Label: "B", Value: "2"}}, false)

	assert.Equal(t, "Title\n  A  1\n  B  2\n", stdout.Screen())
	assert.Contains(t, stdout.Raw(), style.Bold("Title"))

	streams, stdout, _, _ = newRich(t, Rich, 80)
	streams.Block("", "Title", nil, false)
	assert.Equal(t, "Title\n", stdout.Screen())
}

func TestBlockPadsLabelsByRuneCount(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)

	streams.Block("", "T", []Row{{Label: "Nom é", Value: "1"}, {Label: "Id", Value: "2"}}, false)

	assert.Equal(t, "T\n  Nom é  1\n  Id     2\n", stdout.Screen())
}

func TestBlockInStatic(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Static, 80)

	streams.Block(streams.Style().SuccessIcon(), "Debugger ready", debuggerRows, false)

	assert.Equal(t, "✓ Debugger ready\n  Service  my-svc\n  Host     my-svc.example.test\n", stdout.Screen())
}

func TestBlockGoesToStdoutThroughTheLockedWriter(t *testing.T) {
	d := newDrawn(t)

	d.streams.Block("", "Title", []Row{{Label: "A", Value: "1"}}, false)

	assert.Equal(t, "Title\n  A  1\n"+frame(300*time.Millisecond)+" "+uploadLabel, d.screen.Screen())
}

func TestPresentationHelpersDoNothingInClassic(t *testing.T) {
	streams, stdout, stderr, _ := newRich(t, Classic, 80)

	streams.Block("✓", "Title", debuggerRows, false)
	streams.Block("✓", "Title", debuggerRows, true)
	streams.RichTable([]string{"ID"}, [][]string{{"1"}}, nil, "item", "No items.")
	streams.RichTable([]string{"ID"}, nil, nil, "item", "No items.")
	streams.Notice("Update available")
	_, err := fmt.Fprint(streams.Gutter(), "a\n")

	assert.NoError(t, err)
	assert.Equal(t, "a\n", stdout.Raw())
	assert.Empty(t, stderr.Raw())
}
