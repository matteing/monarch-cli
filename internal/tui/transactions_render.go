package tui

import (
	"strings"
	"time"

	"github.com/matteing/monarch-cli/internal/monarch"
	"github.com/matteing/monarch-cli/internal/textsafe"
)

func renderTransactions(transactions []monarch.Transaction, groupByMonth bool, width int) string {
	if len(transactions) == 0 {
		return mutedStyle.Render("No transactions.")
	}
	if !groupByMonth {
		return renderTransactionTable(transactions, width)
	}

	var body strings.Builder
	for start := 0; start < len(transactions); {
		month := transactionMonth(transactions[start].Date)
		end := start + 1
		for end < len(transactions) && transactionMonth(transactions[end].Date) == month {
			end++
		}
		if start > 0 {
			body.WriteString("\n\n")
		}
		body.WriteString(groupStyle.Render(textsafe.Terminal(month)))
		body.WriteString("\n")
		body.WriteString(renderTransactionTable(transactions[start:end], width))
		start = end
	}
	return body.String()
}

func renderTransactionTable(transactions []monarch.Transaction, width int) string {
	headers, rows := transactionRows(transactions, width)
	return renderTableWithColumnWidths(headers, rows, width, transactionColumnWidths(headers, width))
}

// transactionColumnWidths depends only on the viewport and visible columns,
// never on page contents. That keeps each column anchored while paging.
func transactionColumnWidths(headers []string, tableWidth int) []int {
	columnBudget := tableWidth - len(headers) - 1 // outer borders and separators
	if columnBudget < len(headers) {
		return nil
	}

	minimums := make([]int, len(headers))
	var growthOrder []int
	minimumTotal := 0
	for index, header := range headers {
		minimum, weight := 10, 1
		switch header {
		case "DATE":
			minimum, weight = 12, 0
		case "MERCHANT":
			minimum, weight = 10, 3
		case "CATEGORY", "ACCOUNT":
			minimum, weight = 10, 2
		case "AMOUNT":
			minimum, weight = 14, 0
		case "ID":
			minimum, weight = 16, 2
		}
		minimums[index] = minimum
		minimumTotal += minimum
		for range weight {
			growthOrder = append(growthOrder, index)
		}
	}

	widths := make([]int, len(headers))
	if minimumTotal > columnBudget {
		for index := range widths {
			widths[index] = 1
		}
		for remaining, index := columnBudget-len(widths), 0; remaining > 0; index = (index + 1) % len(widths) {
			if widths[index] >= minimums[index] {
				continue
			}
			widths[index]++
			remaining--
		}
		return widths
	}

	copy(widths, minimums)
	for extra, index := columnBudget-minimumTotal, 0; extra > 0; index = (index + 1) % len(growthOrder) {
		widths[growthOrder[index]]++
		extra--
	}
	return widths
}

func transactionRows(transactions []monarch.Transaction, width int) ([]string, [][]string) {
	headers := []string{"DATE", "MERCHANT", "AMOUNT"}
	includeCategory := width >= 64
	includeAccount := width >= 88
	includeID := width >= 130
	if includeCategory {
		headers = []string{"DATE", "MERCHANT", "CATEGORY", "AMOUNT"}
	}
	if includeAccount {
		headers = []string{"DATE", "MERCHANT", "CATEGORY", "ACCOUNT", "AMOUNT"}
	}
	if includeID {
		headers = append(headers, "ID")
	}

	rows := make([][]string, 0, len(transactions))
	for _, transaction := range transactions {
		row := []string{transaction.Date, transactionMerchant(transaction)}
		if includeCategory {
			row = append(row, transactionCategory(transaction))
		}
		if includeAccount {
			row = append(row, transaction.Account.DisplayName)
		}
		row = append(row, string(transaction.Amount))
		if includeID {
			row = append(row, transaction.ID)
		}
		rows = append(rows, row)
	}
	return headers, rows
}

func transactionMerchant(transaction monarch.Transaction) string {
	if transaction.Merchant == nil {
		return ""
	}
	return transaction.Merchant.Name
}

func transactionCategory(transaction monarch.Transaction) string {
	if transaction.Category == nil {
		return ""
	}
	return transaction.Category.Name
}

func transactionMonth(date string) string {
	parsed, err := time.Parse("2006-01-02", date)
	if err == nil {
		return parsed.Format("January 2006")
	}
	return "Unknown month"
}
