package term

import (
	"io"
	"strings"
	"unicode/utf8"
)

// boxFrame is the width a border adds to a line: "│  " before it and "  │" after it.
const boxFrame = 6

// BoxLine is one line of a Box: a group head when Head is set, otherwise a row of cells.
type BoxLine struct {
	Head  string
	Cells []string
}

// Box prints a titled box (Rich and Static only) as wide as its widest line: "╭─ <title> ─…─╮", a blank row, the
// lines with two spaces of padding, a blank row and the bottom border, all borders dim. Cells are padded into columns
// across the whole box with two-space gutters, indented two spaces under the heads when the box has any. Text is
// measured without escape sequences and never cut: when the box does not fit the terminal, or its width is unknown,
// the title is printed and then the lines indented two spaces, without borders. Without colour no colour sequence is
// printed, not even one the caller passed.
func (t *IO) Box(title string, lines []BoxLine) {
	if t.mode == Classic {
		return
	}
	style := t.Style()
	text := layoutBox(style, title, boxRows(lines), t.columns())
	if !style.ColorEnabled() {
		text = sgrSequence.ReplaceAllString(text, "")
	}
	_, _ = io.WriteString(t.Out, text)
}

// layoutBox draws the box for a terminal of the given width (0 when unknown, which draws no border).
func layoutBox(style Style, title string, rows []string, width int) string {
	inner := visibleWidth(title)
	for _, row := range rows {
		inner = max(inner, visibleWidth(row))
	}
	var b strings.Builder
	if inner+boxFrame > width {
		b.WriteString(title + "\n")
		for _, row := range rows {
			b.WriteString("  " + row + "\n")
		}
		return b.String()
	}

	border := style.Gray("│")
	blank := border + strings.Repeat(" ", inner+4) + border + "\n"
	b.WriteString(style.Gray("╭─ ") + title + style.Gray(" "+strings.Repeat("─", inner+1-visibleWidth(title))+"╮") + "\n")
	b.WriteString(blank)
	for _, row := range rows {
		b.WriteString(border + "  " + row + strings.Repeat(" ", inner-visibleWidth(row)) + "  " + border + "\n")
	}
	b.WriteString(blank)
	b.WriteString(style.Gray("╰"+strings.Repeat("─", inner+4)+"╯") + "\n")
	return b.String()
}

// boxRows is the text of every line: heads as they are, cells padded to their column's width.
func boxRows(lines []BoxLine) []string {
	var widths []int
	indent := ""
	for _, line := range lines {
		if line.Head != "" {
			indent = "  "
		}
		for i, cell := range line.Cells {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], visibleWidth(cell))
		}
	}

	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Head != "" {
			rows = append(rows, line.Head)
			continue
		}
		var b strings.Builder
		for i, cell := range line.Cells {
			b.WriteString(cell)
			if i < len(line.Cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-visibleWidth(cell)) + "  ")
			}
		}
		rows = append(rows, indent+strings.TrimRight(b.String(), " "))
	}
	return rows
}

func visibleWidth(s string) int { return utf8.RuneCountInString(sgrSequence.ReplaceAllString(s, "")) }
