package term

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"
)

// minColumn is the narrowest a column is cut to, unless its header is longer.
const minColumn = 4

// RichTable prints a borderless table (Rich and Static only): bold headers in the caller's wording, two-space gutters,
// non-id columns shortened with "…" (by rune count) when the terminal is too narrow, and a dim count footer such as
// "2 instances" built from noun; empty, the line emptyText.
func (t *IO) RichTable(headers []string, rows [][]string, keep []int, noun, emptyText string) {
	if t.mode == Classic {
		return
	}
	text := emptyText + "\n"
	if len(rows) > 0 {
		text = layoutTable(t.Style(), headers, rows, keep, noun, t.columns())
	}
	_, _ = io.WriteString(t.Out, text)
}

// layoutTable lays the table out for a terminal of the given width (0 when unknown, which cuts nothing).
func layoutTable(style Style, headers []string, rows [][]string, keep []int, noun string, width int) string {
	widths, floors := columnWidths(headers, rows)
	if width > 0 {
		shrinkColumns(widths, floors, keep, width)
	}

	var b strings.Builder
	b.WriteString(tableLine(headers, widths, style.Bold))
	for _, row := range rows {
		b.WriteString(tableLine(row, widths, func(cell string) string { return cell }))
	}
	b.WriteString("\n" + style.Gray(countFooter(len(rows), noun)) + "\n")
	return b.String()
}

// columnWidths is the natural width of every column, and the floor each may be cut down to.
func columnWidths(headers []string, rows [][]string) (widths, floors []int) {
	widths = make([]int, len(headers))
	floors = make([]int, len(headers))
	for i, header := range headers {
		widths[i] = utf8.RuneCountInString(header)
		floors[i] = max(minColumn, widths[i])
	}
	for _, row := range rows {
		for i := 0; i < len(row) && i < len(widths); i++ {
			widths[i] = max(widths[i], utf8.RuneCountInString(row[i]))
		}
	}
	return widths, floors
}

// shrinkColumns cuts the widest column that may be cut, one rune at a time, until the table fits in limit columns or
// nothing is left to cut. A table that still does not fit is printed as it is.
func shrinkColumns(widths, floors, keep []int, limit int) {
	for tableWidth(widths) > limit {
		col := widestCuttable(widths, floors, keep)
		if col < 0 {
			return
		}
		widths[col]--
	}
}

func tableWidth(widths []int) int {
	total := 2 * max(len(widths)-1, 0)
	for _, w := range widths {
		total += w
	}
	return total
}

// widestCuttable is the widest column that is not kept and is above its floor; on a tie, the lowest index.
func widestCuttable(widths, floors, keep []int) int {
	best := -1
	for i, w := range widths {
		if slices.Contains(keep, i) || w <= floors[i] {
			continue
		}
		if best < 0 || w > widths[best] {
			best = i
		}
	}
	return best
}

// tableLine is one row: every column but the last padded to its width, two spaces between columns and no trailing
// spaces. wrap styles a cell and adds no width.
func tableLine(cells []string, widths []int, wrap func(string) string) string {
	var b strings.Builder
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cutCell(cells[i], width)
		}
		b.WriteString(wrap(cell))
		if i < len(widths)-1 {
			b.WriteString(strings.Repeat(" ", width-utf8.RuneCountInString(cell)) + "  ")
		}
	}
	return strings.TrimRight(b.String(), " ") + "\n"
}

// cutCell keeps the first width-1 runes of a cell that is wider than width and ends it in "…".
func cutCell(cell string, width int) string {
	if utf8.RuneCountInString(cell) <= width {
		return cell
	}
	return string([]rune(cell)[:max(width-1, 0)]) + ellipsis
}

func countFooter(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
