package term

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestRichTableLayout(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)
	style := streams.Style()
	rows := [][]string{{"abc-1", "My app"}, {"abc-22", "Zed"}}

	streams.RichTable([]string{"ID", "Name"}, rows, []int{0}, "application", "No applications found.")

	assert.Equal(t, "ID      Name\nabc-1   My app\nabc-22  Zed\n\n2 applications\n", stdout.Screen())
	raw := stdout.Raw()
	assert.Contains(t, raw, style.Bold("ID"))
	assert.Contains(t, raw, style.Bold("Name"))
	assert.Contains(t, raw, style.Gray("2 applications"))
	for _, line := range strings.Split(stdout.Screen(), "\n") {
		assert.Equal(t, strings.TrimRight(line, " "), line, "no trailing spaces")
	}
}

func TestRichTableFooterPlural(t *testing.T) {
	for rows, footer := range map[int]string{1: "1 instance", 3: "3 instances"} {
		streams, stdout, _, _ := newRich(t, Rich, 80)
		data := make([][]string, rows)
		for i := range data {
			data[i] = []string{"i"}
		}

		streams.RichTable([]string{"ID"}, data, nil, "instance", "No instances found.")

		assert.True(t, strings.HasSuffix(stdout.Screen(), "\n\n"+footer+"\n"), stdout.Screen())
	}
}

func TestRichTableEmpty(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)

	streams.RichTable([]string{"ID", "Name"}, nil, []int{0}, "instance", "No instances found.")

	assert.Equal(t, "No instances found.\n", stdout.Screen())
	assert.Equal(t, "No instances found.\n", stdout.Raw())
}

func TestRichTableHasNoTrailingSpaces(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)

	streams.RichTable([]string{"ID", "Name"}, [][]string{{"abc", ""}, {"d", "x"}}, []int{0}, "item", "None.")

	assert.Equal(t, "ID   Name\nabc\nd    x\n\n2 items\n", stdout.Screen())
}

func TestRichTableKeepsHeadersAsGiven(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 80)

	streams.RichTable([]string{"Instance ID", "Service name"}, [][]string{{"i-1", "svc"}}, nil, "instance", "None.")

	assert.True(t, strings.HasPrefix(stdout.Screen(), "Instance ID  Service name\n"), stdout.Screen())
}

func TestRichTableTruncatesNonKeepColumns(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 40)
	row := []string{"id-01", strings.Repeat("N", 30), strings.Repeat("R", 22)}

	streams.RichTable([]string{"ID", "Name", "Region"}, [][]string{row}, []int{0}, "instance", "No instances found.")

	want := "ID" + strings.Repeat(" ", 5) + "Name" + strings.Repeat(" ", 13) + "Region\n" +
		"id-01" + "  " + strings.Repeat("N", 14) + "…" + "  " + strings.Repeat("R", 15) + "…\n" +
		"\n1 instance\n"
	assert.Equal(t, want, stdout.Screen())
	for _, line := range strings.Split(stdout.Screen(), "\n") {
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 40)
	}
}

func TestRichTableNeverCutsAKeepColumn(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 40)
	long := strings.Repeat("K", 60)

	streams.RichTable([]string{"ID", "Name"}, [][]string{{long, "short"}}, []int{0}, "item", "None.")

	assert.Contains(t, stdout.Screen(), long)
}

func TestRichTableCutsOnRuneBoundaries(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 14)
	rows := [][]string{{"id-1", "日本語アプリケーション"}, {"id-2", "café é long name"}}

	streams.RichTable([]string{"ID", "Name"}, rows, []int{0}, "item", "None.")

	screen := stdout.Screen()
	assert.True(t, utf8.ValidString(screen))
	assert.Contains(t, screen, "id-1  日本語アプリケ…\n")
	assert.Contains(t, screen, "id-2  café é …\n")
	for _, line := range strings.Split(screen, "\n") {
		assert.LessOrEqual(t, utf8.RuneCountInString(line), 14, line)
	}
}

func TestRichTableColumnFloor(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 10)

	streams.RichTable([]string{"ID", "Name"}, [][]string{{"id-01", "Longer name"}}, []int{0}, "item", "None.")

	assert.Equal(t, "ID     Name\nid-01  Lon…\n\n1 item\n", stdout.Screen())
}

func TestRichTableFloorIsTheHeaderWidthWhenLonger(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Rich, 10)

	streams.RichTable([]string{"ID", "Service name"}, [][]string{{"id-01", "Longer name than that"}}, []int{0}, "item", "None.")

	assert.Equal(t, "ID     Service name\nid-01  Longer name…\n\n1 item\n", stdout.Screen())
}

func TestRichTableUnknownWidthDoesNotCut(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Static, 0)
	name := strings.Repeat("N", 120)

	streams.RichTable([]string{"ID", "Name"}, [][]string{{"id-01", name}}, []int{0}, "item", "None.")

	assert.Contains(t, stdout.Screen(), "id-01  "+name+"\n")
}

func TestRichTableInStatic(t *testing.T) {
	streams, stdout, _, _ := newRich(t, Static, 80)

	streams.RichTable([]string{"ID"}, [][]string{{"a"}, {"b"}}, nil, "item", "None.")

	assert.Equal(t, "ID\na\nb\n\n2 items\n", stdout.Screen())
}
