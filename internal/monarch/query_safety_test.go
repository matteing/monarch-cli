package monarch

import (
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

func TestEmbeddedOperationsAreNamedQueriesOnly(t *testing.T) {
	documents := map[string]string{
		"accounts": accountsQuery, "transactions": transactionsQuery,
		"transaction": transactionQuery, "categories": categoriesQuery,
		"budgets": budgetsQuery, "cashflow": cashflowQuery,
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
			if operation.Operation != ast.Query {
				t.Fatalf("operation type = %q, want query", operation.Operation)
			}
			if operation.Name == "" {
				t.Fatal("operation must be named")
			}
		})
	}
}
