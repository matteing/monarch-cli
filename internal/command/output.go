package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/matteing/monarch-cli/internal/textsafe"
	"github.com/matteing/monarch-cli/internal/tui"
)

func writeJSON(output io.Writer, value any) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return err
	}
	_, err := io.Copy(output, strings.NewReader(textsafe.EscapeJSON(encoded.String())))
	return err
}

func (a *application) writeResult(value any, headers []string, rows [][]string) error {
	if a.config.Output == "json" {
		return writeJSON(a.out, value)
	}
	return a.writeTable(headers, rows)
}

func (a *application) writeTable(headers []string, rows [][]string) error {
	if tui.IsTerminal(a.out) {
		width, _ := tui.TerminalSize(a.out)
		_, err := fmt.Fprintln(a.out, tui.RenderTable(headers, rows, width))
		return err
	}
	cleanRows := make([][]string, len(rows))
	for rowIndex, row := range rows {
		cleanRows[rowIndex] = make([]string, len(row))
		for columnIndex, value := range row {
			cleanRows[rowIndex][columnIndex] = textsafe.Terminal(value)
		}
	}
	return writePlainTable(a.out, headers, cleanRows)
}

func writePlainTable(output io.Writer, headers []string, rows [][]string) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	for index, header := range headers {
		if index > 0 {
			fmt.Fprint(writer, "\t")
		}
		fmt.Fprint(writer, header)
	}
	fmt.Fprintln(writer)
	for _, row := range rows {
		for index, value := range row {
			if index > 0 {
				fmt.Fprint(writer, "\t")
			}
			fmt.Fprint(writer, value)
		}
		fmt.Fprintln(writer)
	}
	return writer.Flush()
}
