package mcpserver

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/matteing/monarch-cli/internal/monarch"
)

const maxRangeDescription = " Range cannot exceed ten years."

func emptySchema() *jsonschema.Schema {
	return objectSchema(nil, nil, nil)
}

func accountsSchema() *jsonschema.Schema {
	falseDefault := json.RawMessage("false")
	return objectSchema(map[string]*jsonschema.Schema{
		"include_hidden": {
			Type: "boolean", Default: falseDefault,
			Description: "Include accounts hidden in Monarch; defaults to false.",
		},
		"include_deactivated": {
			Type: "boolean", Default: falseDefault,
			Description: "Include deactivated accounts; defaults to false.",
		},
	}, nil, nil)
}

func transactionsSchema() *jsonschema.Schema {
	return objectSchema(map[string]*jsonschema.Schema{
		"start_date": dateProperty("Inclusive start date; provide with end_date."),
		"end_date":   dateProperty("Inclusive end date; provide with start_date."),
		"search": {
			Type: "string", MaxLength: jsonschema.Ptr(monarch.MaxTransactionSearchLength),
			Description: "Optional merchant or description search.",
		},
		"account_ids":  idArrayProperty("Optional account IDs."),
		"category_ids": idArrayProperty("Optional category IDs."),
		"tag_ids":      idArrayProperty("Optional tag IDs."),
		"limit": {
			Type: "integer", Minimum: jsonschema.Ptr(1.0), Maximum: jsonschema.Ptr(float64(monarch.MaxTransactionPageSize)),
			Default:     json.RawMessage(strconv.Itoa(monarch.DefaultTransactionPageSize)),
			Description: fmt.Sprintf("Page size; defaults to %d.", monarch.DefaultTransactionPageSize),
		},
		"cursor": {
			Type: "string", MaxLength: jsonschema.Ptr(monarch.MaxTransactionCursorLength),
			Description: "Opaque next_cursor returned by an earlier call.",
		},
	}, nil, map[string][]string{"start_date": {"end_date"}, "end_date": {"start_date"}})
}

func transactionSchema() *jsonschema.Schema {
	return objectSchema(map[string]*jsonschema.Schema{
		"id": {
			Type: "string", MinLength: jsonschema.Ptr(1), MaxLength: jsonschema.Ptr(monarch.MaxOpaqueIDLength),
			Description: "Monarch transaction ID.",
		},
	}, []string{"id"}, nil)
}

func budgetsSchema() *jsonschema.Schema {
	month := func(description string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "string", Pattern: `^[0-9]{4}-(0[1-9]|1[0-2])$`, Description: description + maxRangeDescription}
	}
	return objectSchema(map[string]*jsonschema.Schema{
		"start_month": month("Inclusive start month; provide with end_month or omit both for the current month."),
		"end_month":   month("Inclusive end month; provide with start_month or omit both for the current month."),
	}, nil, map[string][]string{"start_month": {"end_month"}, "end_month": {"start_month"}})
}

func dateRangeSchema() *jsonschema.Schema {
	return objectSchema(map[string]*jsonschema.Schema{
		"start_date": dateProperty("Inclusive start date; provide with end_date or omit both for the current month."),
		"end_date":   dateProperty("Inclusive end date; provide with start_date or omit both for the current month."),
	}, nil, map[string][]string{"start_date": {"end_date"}, "end_date": {"start_date"}})
}

func dateProperty(description string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string", Pattern: `^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`,
		Description: description + maxRangeDescription,
	}
}

func idArrayProperty(description string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "array", MaxItems: jsonschema.Ptr(monarch.MaxTransactionFilterIDs), UniqueItems: true,
		Items:       &jsonschema.Schema{Type: "string", MinLength: jsonschema.Ptr(1), MaxLength: jsonschema.Ptr(monarch.MaxOpaqueIDLength)},
		Description: description,
	}
}

func objectSchema(properties map[string]*jsonschema.Schema, required []string, dependent map[string][]string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		DependentRequired:    dependent,
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}
