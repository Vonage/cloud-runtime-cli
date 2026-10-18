package term

import (
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

var sgrSequence = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Block prints a titled block (Rich and Static only): "<icon> <bold title>" then two-space-indented rows with padded
// dim labels and values printed whole. Plain rows (Rich and Static) are printed uncoloured as "  <Label>: <Value>", so
// literal lines that tools grep stay contiguous.
func (t *IO) Block(icon, title string, rows []Row, plain bool) {
	if t.mode == Classic {
		return
	}
	text := plainBlock(icon, title, rows)
	if !plain {
		text = styledBlock(t.Style(), icon, title, rows)
	}
	_, _ = io.WriteString(t.Out, text)
}

// plainBlock has no escape sequence at all, not even the colour of the icon the caller passed.
func plainBlock(icon, title string, rows []Row) string {
	var b strings.Builder
	b.WriteString(blockHead(icon, title) + "\n")
	for _, row := range rows {
		b.WriteString("  " + row.Label + ": " + row.Value + "\n")
	}
	return sgrSequence.ReplaceAllString(b.String(), "")
}

func styledBlock(style Style, icon, title string, rows []Row) string {
	widest := 0
	for _, row := range rows {
		widest = max(widest, utf8.RuneCountInString(row.Label))
	}

	var b strings.Builder
	b.WriteString(blockHead(icon, style.Bold(title)) + "\n")
	for _, row := range rows {
		pad := strings.Repeat(" ", widest-utf8.RuneCountInString(row.Label))
		b.WriteString("  " + style.Gray(row.Label) + pad + "  " + row.Value + "\n")
	}
	return b.String()
}

func blockHead(icon, title string) string {
	if icon == "" {
		return title
	}
	return icon + " " + title
}
