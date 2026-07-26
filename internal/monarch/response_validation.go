package monarch

import (
	"errors"
	"fmt"
)

func requireIdentity(path, id string) error {
	if id == "" {
		return fmt.Errorf("%s is missing, null, or has no id", path)
	}
	return nil
}

func requireAccountType(path string, accountType AccountType) error {
	if accountType.Name == "" && accountType.Display == "" {
		return fmt.Errorf("%s is missing, null, or empty", path)
	}
	return nil
}

func validateAccountResponses(responses []accountResponse) ([]Account, error) {
	accounts := make([]Account, 0, len(responses))
	for index, response := range responses {
		path := fmt.Sprintf("account %d", index)
		fields := []struct {
			name    string
			present bool
			null    bool
		}{
			{"deactivated_at", response.DeactivatedAt.Present, response.DeactivatedAt.Null},
			{"is_hidden", response.IsHidden.Present, response.IsHidden.Null},
			{"is_asset", response.IsAsset.Present, response.IsAsset.Null},
		}
		for _, field := range fields {
			if !field.present || (field.name != "deactivated_at" && field.null) {
				return nil, fmt.Errorf("%s %s is missing or null", path, field.name)
			}
		}
		if !response.IncludeInNetWorth.Present || !response.IncludeBalanceInNetWorth.Present {
			return nil, fmt.Errorf("%s net-worth inclusion fields are missing", path)
		}
		// Monarch has used both inclusion fields over time. A legacy record may
		// expose one as null, but at least one must carry an authoritative value.
		if response.IncludeInNetWorth.Null && response.IncludeBalanceInNetWorth.Null {
			return nil, fmt.Errorf("%s net-worth inclusion fields are both null", path)
		}
		account := response.Account
		account.DeactivatedAt = response.DeactivatedAt.Value
		account.IsHidden = response.IsHidden.Value
		account.IsAsset = response.IsAsset.Value
		account.IncludeInNetWorth = response.IncludeInNetWorth.Value
		account.IncludeBalanceInNetWorth = response.IncludeBalanceInNetWorth.Value
		accounts = append(accounts, account)
	}
	if err := validateAccounts(accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func validateAccounts(accounts []Account) error {
	for index, account := range accounts {
		path := fmt.Sprintf("account %d", index)
		if err := requireIdentity(path, account.ID); err != nil {
			return err
		}
		if err := requireAccountType(path+" type", account.Type); err != nil {
			return err
		}
		if err := requireAccountType(path+" subtype", account.Subtype); err != nil {
			return err
		}
		if account.DisplayBalance == "" {
			return fmt.Errorf("%s display_balance is missing or null", path)
		}
		if account.CurrentBalance == "" {
			return fmt.Errorf("%s current_balance is missing or null", path)
		}
	}
	return nil
}

func validateCategories(categories []Category) error {
	for index, category := range categories {
		if err := validateCategory(fmt.Sprintf("category %d", index), category); err != nil {
			return err
		}
	}
	return nil
}

func validateCategory(path string, category Category) error {
	if err := requireIdentity(path, category.ID); err != nil {
		return err
	}
	return requireIdentity(path+" group", category.Group.ID)
}

func validateTransactions(transactions []Transaction) error {
	for index, transaction := range transactions {
		path := fmt.Sprintf("transaction %d", index)
		if err := requireIdentity(path, transaction.ID); err != nil {
			return err
		}
		if transaction.Amount == "" {
			return fmt.Errorf("%s amount is missing or null", path)
		}
		if transaction.Date == "" {
			return fmt.Errorf("%s date is missing or null", path)
		}
		if err := requireIdentity(path+" account", transaction.Account.ID); err != nil {
			return err
		}
		if transaction.Category != nil {
			if err := validateCategory(path+" category", *transaction.Category); err != nil {
				return err
			}
		}
		if transaction.Merchant != nil {
			if err := requireIdentity(path+" merchant", transaction.Merchant.ID); err != nil {
				return err
			}
		}
		if transaction.Goal != nil {
			if err := requireIdentity(path+" goal", transaction.Goal.ID); err != nil {
				return err
			}
		}
		for splitIndex, split := range transaction.SplitTransactions {
			splitPath := fmt.Sprintf("%s split %d", path, splitIndex)
			if err := requireIdentity(splitPath, split.ID); err != nil {
				return err
			}
			if split.Amount == "" {
				return fmt.Errorf("%s amount is missing or null", splitPath)
			}
			if split.Category != nil {
				if err := validateCategory(splitPath+" category", *split.Category); err != nil {
					return err
				}
			}
			if split.Merchant != nil {
				if err := requireIdentity(splitPath+" merchant", split.Merchant.ID); err != nil {
					return err
				}
			}
		}
		for tagIndex, tag := range transaction.Tags {
			if err := requireIdentity(fmt.Sprintf("%s tag %d", path, tagIndex), tag.ID); err != nil {
				return err
			}
		}
		for attachmentIndex, attachment := range transaction.Attachments {
			if err := requireIdentity(fmt.Sprintf("%s attachment %d", path, attachmentIndex), attachment.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateBudgets(categories []CategoryBudget, groups []GroupBudget) error {
	validate := func(owner string, ownerIndex int, amounts []BudgetAmount) error {
		for amountIndex, amount := range amounts {
			if amount.Month == "" {
				return fmt.Errorf("%s %d monthly amount %d month is missing or null", owner, ownerIndex, amountIndex)
			}
			if amount.PlannedCashFlowAmount == "" || amount.ActualAmount == "" || amount.RemainingAmount == "" {
				return fmt.Errorf("%s %d monthly amount %d has a missing or null required amount", owner, ownerIndex, amountIndex)
			}
		}
		return nil
	}
	for index, category := range categories {
		if err := requireIdentity(fmt.Sprintf("category budget %d category", index), category.Category.ID); err != nil {
			return err
		}
		if category.MonthlyAmounts == nil {
			return fmt.Errorf("category budget %d monthly_amounts is missing or null", index)
		}
		if err := validate("category budget", index, category.MonthlyAmounts); err != nil {
			return err
		}
	}
	for index, group := range groups {
		if err := requireIdentity(fmt.Sprintf("group budget %d category_group", index), group.CategoryGroup.ID); err != nil {
			return err
		}
		if group.MonthlyAmounts == nil {
			return fmt.Errorf("group budget %d monthly_amounts is missing or null", index)
		}
		if err := validate("group budget", index, group.MonthlyAmounts); err != nil {
			return err
		}
	}
	return nil
}

func validateCashflowAmounts(summary CashflowSummary) error {
	if summary.SumIncome == "" || summary.SumExpense == "" || summary.Savings == "" || summary.SavingsRate == "" {
		return errors.New("cashflow summary has a missing or null required amount")
	}
	return nil
}

func validateCashflowAggregates(aggregates []cashflowAggregate) error {
	for index, aggregate := range aggregates {
		if !aggregate.Summary.Present || aggregate.Summary.Null {
			return fmt.Errorf("cashflow aggregate %d summary is missing or null", index)
		}
		if err := validateCashflowAmounts(aggregate.Summary.Value); err != nil {
			return fmt.Errorf("cashflow aggregate %d: %w", index, err)
		}
	}
	return nil
}
