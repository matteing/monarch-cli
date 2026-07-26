package monarch

import "context"

type categoriesData struct {
	Categories responseField[[]Category] `json:"categories"`
}

// ListCategories returns the household's transaction categories.
func (c *Client) ListCategories(ctx context.Context) (CategoriesResult, error) {
	const op = "list categories"
	data, err := execute[categoriesData](ctx, c, op, categoriesQuery, nil)
	if err != nil {
		return CategoriesResult{}, err
	}
	if !data.Categories.Present || data.Categories.Null {
		return CategoriesResult{}, unexpectedResponse(op, "GraphQL data.categories is missing or null")
	}
	if err := validateCategories(data.Categories.Value); err != nil {
		return CategoriesResult{}, unexpectedResponse(op, err.Error())
	}
	return CategoriesResult{Categories: data.Categories.Value}, nil
}
