package monarch

import (
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

func TestEmbeddedOperationsAreNamedAndExpectedType(t *testing.T) {
	documents := map[string]string{
		"accounts": accountsQuery, "transactions": transactionsQuery,
		"transaction": transactionQuery, "categories": categoriesQuery,
		"budgets": budgetsQuery, "cashflow": cashflowQuery,
		"refresh_accounts": refreshAccountsMutation,
	}
	for name, source := range documents {
		t.Run(name, func(t *testing.T) {
			document, err := parser.ParseQuery(&ast.Source{Name: name + ".graphql", Input: source})
			if err != nil {
				t.Fatal(err)
			}
			if len(document.Operations) != 1 {
				t.Fatalf("operation count = %d, want 1", len(document.Operations))
			}
			operation := document.Operations[0]
			wantType := ast.Query
			if name == "refresh_accounts" {
				wantType = ast.Mutation
			}
			if operation.Operation != wantType {
				t.Fatalf("operation type = %q, want %q", operation.Operation, wantType)
			}
			if operation.Name == "" {
				t.Fatal("operation must be named")
			}
		})
	}
}
