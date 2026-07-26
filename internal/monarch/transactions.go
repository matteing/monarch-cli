package monarch

import (
	"context"
	"fmt"

	"github.com/matteing/monarch-cli/internal/apperr"
)

type transactionConnection struct {
	TotalCount responseField[int]           `json:"totalCount"`
	Results    responseField[[]Transaction] `json:"results"`
}

type transactionsData struct {
	AllTransactions responseField[transactionConnection] `json:"allTransactions"`
}

type transactionFilters struct {
	StartDate  string   `json:"startDate,omitempty"`
	EndDate    string   `json:"endDate,omitempty"`
	Search     string   `json:"search,omitempty"`
	Accounts   []string `json:"accounts,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

type transactionsVariables struct {
	Offset  int                `json:"offset"`
	Limit   int                `json:"limit"`
	OrderBy string             `json:"orderBy"`
	Filters transactionFilters `json:"filters"`
}

// ListTransactions returns one bounded page in Monarch's date order. Monarch
// does not expose a stable secondary key, so equal-date ordering is unspecified.
// Cursor is bound to the normalized filters that produced it. Pagination is
// offset-based, so inserts and deletes can still shift unvisited pages.
func (c *Client) ListTransactions(ctx context.Context, params ListTransactionsParams) (TransactionPage, error) {
	const op = "list transactions"
	if err := ValidateDateRange(DateRange{StartDate: params.StartDate, EndDate: params.EndDate}); err != nil {
		return TransactionPage{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	if err := ValidateTransactionSearch(params.Search); err != nil {
		return TransactionPage{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	if err := ValidateTransactionFilterIDs(params.AccountIDs, params.CategoryIDs, params.TagIDs); err != nil {
		return TransactionPage{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	limit := params.Limit
	if limit == 0 {
		limit = DefaultTransactionPageSize
	}
	if err := ValidateTransactionPageSize(limit); err != nil {
		return TransactionPage{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}

	filters := transactionFilters{
		StartDate:  params.StartDate,
		EndDate:    params.EndDate,
		Search:     params.Search,
		Accounts:   normalizedIDs(params.AccountIDs),
		Categories: normalizedIDs(params.CategoryIDs),
		Tags:       normalizedIDs(params.TagIDs),
	}
	fingerprint, err := transactionCursorFingerprint(filters)
	if err != nil {
		return TransactionPage{}, apperr.New(apperr.KindInternal, op, "could not prepare transaction pagination", err)
	}
	offset, err := decodeCursor(params.Cursor, fingerprint)
	if err != nil {
		return TransactionPage{}, apperr.New(apperr.KindInvalidInput, op, "cursor is invalid or belongs to different filters", err)
	}

	variables := transactionsVariables{
		Offset: offset, Limit: limit, OrderBy: transactionOrder, Filters: filters,
	}
	data, err := execute[transactionsData](ctx, c, op, transactionsQuery, variables)
	if err != nil {
		return TransactionPage{}, err
	}
	if !data.AllTransactions.Present || data.AllTransactions.Null {
		return TransactionPage{}, unexpectedResponse(op, "GraphQL data.allTransactions is missing or null")
	}
	connection := data.AllTransactions.Value
	if !connection.TotalCount.Present || connection.TotalCount.Null || connection.TotalCount.Value < 0 {
		return TransactionPage{}, unexpectedResponse(op, "GraphQL allTransactions.totalCount is missing, null, or negative")
	}
	if !connection.Results.Present || connection.Results.Null {
		return TransactionPage{}, unexpectedResponse(op, "GraphQL allTransactions.results is missing or null")
	}
	if err := validateTransactionPage(offset, limit, connection.TotalCount.Value, len(connection.Results.Value)); err != nil {
		return TransactionPage{}, unexpectedResponse(op, err.Error())
	}
	if err := validateTransactions(connection.Results.Value); err != nil {
		return TransactionPage{}, unexpectedResponse(op, err.Error())
	}
	transactions := connection.Results.Value
	result := TransactionPage{Transactions: transactions, TotalCount: connection.TotalCount.Value}
	nextOffset := offset + len(transactions)
	if nextOffset < result.TotalCount && len(transactions) > 0 {
		if nextOffset > maxTransactionOffset {
			return TransactionPage{}, unexpectedResponse(op, "GraphQL transaction results exceed the supported pagination range")
		}
		result.NextCursor = encodeCursor(nextOffset, fingerprint)
	}
	return result, nil
}

func validateTransactionPage(offset, limit, total, count int) error {
	if count > limit {
		return fmt.Errorf("GraphQL transaction page returned %d results for limit %d", count, limit)
	}
	if offset > total {
		if count != 0 {
			return fmt.Errorf("GraphQL transaction page returned results beyond total count %d", total)
		}
		// Deletions between offset-based requests can legitimately move the end
		// of the collection before a previously issued cursor.
		return nil
	}
	if offset+count > total {
		return fmt.Errorf("GraphQL transaction page exceeds total count %d", total)
	}
	if offset < total && count == 0 {
		return fmt.Errorf("GraphQL transaction page is empty before total count %d", total)
	}
	if count < limit && offset+count < total {
		return fmt.Errorf("GraphQL transaction page ended before total count %d", total)
	}
	return nil
}

type transactionData struct {
	Transaction responseField[Transaction] `json:"getTransaction"`
}

type transactionVariables struct {
	ID string `json:"id"`
}

// GetTransaction retrieves one transaction by its opaque Monarch ID.
func (c *Client) GetTransaction(ctx context.Context, id string) (TransactionResult, error) {
	const op = "get transaction"
	if err := ValidateTransactionID(id); err != nil {
		return TransactionResult{}, apperr.New(apperr.KindInvalidInput, op, err.Error(), err)
	}
	data, err := execute[transactionData](ctx, c, op, transactionQuery, transactionVariables{ID: id})
	if err != nil {
		return TransactionResult{}, err
	}
	if !data.Transaction.Present {
		return TransactionResult{}, unexpectedResponse(op, "GraphQL data.getTransaction is missing")
	}
	if data.Transaction.Null || data.Transaction.Value.ID == "" {
		return TransactionResult{}, apperr.New(apperr.KindNotFound, op, "transaction not found", nil)
	}
	if err := validateTransactions([]Transaction{data.Transaction.Value}); err != nil {
		return TransactionResult{}, unexpectedResponse(op, err.Error())
	}
	return TransactionResult{Transaction: data.Transaction.Value}, nil
}
