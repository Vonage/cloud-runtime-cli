package term

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderTable(t *testing.T, headers []string, rows [][]string) string {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, Table(&out, headers, rows))
	return out.String()
}

func TestTableMatchesBaselineLayout(t *testing.T) {
	got := renderTable(t, []string{"ID", "Name"}, [][]string{
		{"abc-1", "My app"},
		{"abc-22", "Zed"},
	})

	want := "" +
		"┌────────┬────────┐\n" +
		"│   ID   │  NAME  │\n" +
		"├────────┼────────┤\n" +
		"│ abc-1  │ My app │\n" +
		"│ abc-22 │ Zed    │\n" +
		"└────────┴────────┘\n"
	assert.Equal(t, want, got)
}

func TestTableHeaderOnly(t *testing.T) {
	got := renderTable(t, []string{"Instance ID", "Name"}, nil)

	want := "" +
		"┌─────────────┬──────┐\n" +
		"│ INSTANCE ID │ NAME │\n" +
		"└─────────────┴──────┘\n"
	assert.Equal(t, want, got)
}

func TestTableWideText(t *testing.T) {
	got := renderTable(t, []string{"ID", "Name"}, [][]string{
		{"a-1", "日本語アプリ"},
		{"a-2", "café é"},
	})

	want := "" +
		"┌─────┬──────────────┐\n" +
		"│ ID  │     NAME     │\n" +
		"├─────┼──────────────┤\n" +
		"│ a-1 │ 日本語アプリ │\n" +
		"│ a-2 │ café é       │\n" +
		"└─────┴──────────────┘\n"
	assert.Equal(t, want, got)
}

func TestTableEmptyAndMultilineCells(t *testing.T) {
	got := renderTable(t, []string{"ID", "Name"}, [][]string{
		{"a-1", ""},
		{"", "x\ny"},
	})

	want := "" +
		"┌─────┬──────┐\n" +
		"│ ID  │ NAME │\n" +
		"├─────┼──────┤\n" +
		"│ a-1 │      │\n" +
		"│     │ x    │\n" +
		"│     │ y    │\n" +
		"└─────┴──────┘\n"
	assert.Equal(t, want, got)
}
