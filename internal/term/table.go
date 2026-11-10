package term

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
)

// Table renders a box-drawing table with tablewriter v1.1.2 defaults, the way `app list` and
// `instance list` do in v2.6.1. Callers sanitise cells with Sanitize first.
func Table(w io.Writer, headers []string, rows [][]string) error {
	table := tablewriter.NewWriter(w)

	header := make([]any, len(headers))
	for i, h := range headers {
		header[i] = h
	}
	table.Header(header...)

	for _, row := range rows {
		if err := table.Append(row); err != nil {
			return fmt.Errorf("failed to append row to table: %w", err)
		}
	}
	return table.Render()
}
