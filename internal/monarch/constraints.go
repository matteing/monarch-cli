package monarch

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// Public input bounds are shared by the client, CLI, and MCP schemas.
const (
	DefaultTransactionPageSize = 25
	MaxTransactionPageSize     = 100
	MaxTransactionSearchLength = 200
	MaxTransactionFilterIDs    = 50
	MaxOpaqueIDLength          = 200
	MaxTransactionCursorLength = 256
	maxTransactionOffset       = 10_000_000
	// FinancialOverviewTransactionLimit bounds the overview's recent activity.
	FinancialOverviewTransactionLimit = 10
)

// ValidateTransactionSearch validates a transaction search string.
func ValidateTransactionSearch(search string) error {
	if !utf8.ValidString(search) {
		return errors.New("search must be valid UTF-8")
	}
	if utf8.RuneCountInString(search) > MaxTransactionSearchLength {
		return fmt.Errorf("search is limited to %d characters", MaxTransactionSearchLength)
	}
	return nil
}

// ValidateTransactionFilterIDs validates one or more transaction ID filters.
func ValidateTransactionFilterIDs(groups ...[]string) error { return validateIDs(groups...) }

// ValidateTransactionPageSize validates an explicit, non-default page size.
func ValidateTransactionPageSize(limit int) error {
	if limit < 1 || limit > MaxTransactionPageSize {
		return fmt.Errorf("limit must be between 1 and %d", MaxTransactionPageSize)
	}
	return nil
}

// ValidateTransactionID validates one opaque Monarch transaction ID.
func ValidateTransactionID(id string) error {
	if !validOpaqueID(id) {
		return fmt.Errorf("transaction ID must be 1-%d printable characters", MaxOpaqueIDLength)
	}
	return nil
}

// ValidateDateRange validates an optional inclusive date range. Both dates may
// be empty to request an operation's documented default range.
func ValidateDateRange(dates DateRange) error {
	return validateDateRange(dates.StartDate, dates.EndDate, true)
}
