// Package tui contains the human-facing terminal UI used by monarch. JSON and
// MCP data output bypass it; interactive login renders to stderr in every mode.
package tui

import (
	"fmt"
	"io"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"golang.org/x/term"

	"github.com/matteing/monarch-cli/internal/textsafe"
)

var (
	accentColor = lipgloss.Color("#F2A65A")
	mutedColor  = lipgloss.Color("#7C8492")
	borderColor = lipgloss.Color("#454B59")

	groupStyle   = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	mutedStyle   = lipgloss.NewStyle().Foreground(mutedColor)
	borderStyle  = lipgloss.NewStyle().Foreground(borderColor)
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#73DACA"))
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
)

// IsTerminal reports whether value is backed by a terminal file descriptor.
// It intentionally returns false for buffers and pipes so automation receives
// stable, undecorated output.
func IsTerminal(value any) bool {
	file, ok := value.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(file.Fd()))
}

// TerminalSize returns the current terminal dimensions. Zero values indicate
// that value is not a terminal or its size could not be read.
func TerminalSize(value any) (int, int) {
	file, ok := value.(interface{ Fd() uintptr })
	if !ok {
		return 0, 0
	}
	width, height, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return 0, 0
	}
	return width, height
}

// RenderTable renders a compact, lightly styled table without a page heading.
// Callers are responsible for using a plain renderer for pipes.
func RenderTable(headers []string, rows [][]string, terminalWidth ...int) string {
	width := 0
	if len(terminalWidth) > 0 {
		width = terminalWidth[0]
	}
	return renderTable(headers, rows, width)
}

func renderTable(headers []string, rows [][]string, terminalWidth int) string {
	return renderTableWithColumnWidths(headers, rows, terminalWidth, nil)
}

func renderTableWithColumnWidths(headers []string, rows [][]string, terminalWidth int, columnWidths []int) string {
	headers, rows = safeTableData(headers, rows)
	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		BorderRow(false).
		StyleFunc(func(row, column int) lipgloss.Style {
			style := lipgloss.NewStyle().Padding(0, 1)
			if column < len(columnWidths) && columnWidths[column] > 0 {
				style = style.Width(columnWidths[column])
			}
			if row == table.HeaderRow {
				return style.Bold(true).Foreground(accentColor)
			}
			return style
		})
	if terminalWidth > 0 {
		width := min(terminalWidth, 140)
		t.Width(max(width, 1)).Wrap(false)
	}
	return t.Render()
}

// WriteSuccess writes a concise, styled success message.
func WriteSuccess(output io.Writer, message string) error {
	_, err := fmt.Fprintln(output, successStyle.Render("✓ "+textsafe.Terminal(message)))
	return err
}

type frameLayout struct {
	contentWidth int
	padding      int
	frameWidth   int
}

func responsiveFrame(width, maximumContentWidth int) frameLayout {
	width = max(width, 1)
	padding := 2
	if width < 4 {
		padding = 0
	} else if width < 40 {
		padding = 1
	}
	contentWidth := min(max(width-(padding*2), 1), maximumContentWidth)
	return frameLayout{
		contentWidth: contentWidth,
		padding:      padding,
		frameWidth:   contentWidth + (padding * 2),
	}
}

func safeTableData(headers []string, rows [][]string) ([]string, [][]string) {
	safeHeaders := make([]string, len(headers))
	for index, header := range headers {
		safeHeaders[index] = textsafe.Terminal(header)
	}
	safeRows := make([][]string, len(rows))
	for rowIndex, row := range rows {
		safeRows[rowIndex] = make([]string, len(row))
		for columnIndex, value := range row {
			safeRows[rowIndex][columnIndex] = textsafe.Terminal(value)
		}
	}
	return safeHeaders, safeRows
}
