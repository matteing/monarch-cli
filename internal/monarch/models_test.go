package monarch

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAmountPreservesLexicalValue(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{`1234567890.123456`, "1234567890.123456"},
		{`"-42.50"`, "-42.50"},
		{`1e3`, "1e3"},
		{`null`, ""},
	} {
		var amount Amount
		if err := json.Unmarshal([]byte(test.input), &amount); err != nil {
			t.Fatalf("Unmarshal(%s): %v", test.input, err)
		}
		if string(amount) != test.want {
			t.Errorf("Unmarshal(%s) = %q, want %q", test.input, amount, test.want)
		}
	}
}

func TestAmountRejectsInvalidDecimals(t *testing.T) {
	for _, input := range []string{
		`""`, `"NaN"`, `"Inf"`, `"1/2"`, `" 1"`, `"0x10"`, `"1_000"`,
		`"+1"`, `"01"`, `"1."`, `"1e1001"`, `"1e-1001"`,
		`"` + strings.Repeat("9", maxAmountTextLength+1) + `"`, `true`, `{}`, `[]`,
	} {
		var amount Amount
		if err := json.Unmarshal([]byte(input), &amount); err == nil {
			t.Errorf("Unmarshal(%s) accepted an invalid decimal", input)
		}
	}
}

func TestNetWorthUsesExactDecimalArithmeticAndAccountSign(t *testing.T) {
	accounts := []Account{
		{DisplayBalance: "0.10", IsAsset: true, IncludeInNetWorth: true},
		{DisplayBalance: "0.20", IsAsset: true, IncludeBalanceInNetWorth: true},
		{DisplayBalance: "0.05", IsAsset: false, IncludeInNetWorth: true},
		{DisplayBalance: "100", IsAsset: true},
	}
	got, err := netWorth(accounts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.25" {
		t.Fatalf("netWorth() = %q, want 0.25", got)
	}
}

func TestNetWorthPreservesSubCentPrecision(t *testing.T) {
	accounts := []Account{
		{DisplayBalance: "0.001", IsAsset: true, IncludeInNetWorth: true},
		{DisplayBalance: "1e-4", IsAsset: true, IncludeInNetWorth: true},
	}
	got, err := netWorth(accounts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.0011" {
		t.Fatalf("netWorth() = %q, want 0.0011", got)
	}
}

func TestNetWorthRejectsMissingOrInvalidIncludedBalance(t *testing.T) {
	for _, account := range []Account{
		{ID: "missing", IsAsset: true, IncludeInNetWorth: true},
		{ID: "invalid", DisplayBalance: "not-money", IsAsset: true, IncludeInNetWorth: true},
		{ID: "extreme", DisplayBalance: "1e-1001", IsAsset: true, IncludeInNetWorth: true},
	} {
		if _, err := netWorth([]Account{account}); err == nil {
			t.Fatalf("netWorth(%+v) accepted an unusable balance", account)
		}
	}
}

func TestCursorRoundTripAndFilterBinding(t *testing.T) {
	filters := transactionFilters{Search: "coffee", Accounts: []string{"a", "b"}}
	fingerprint, err := transactionCursorFingerprint(filters)
	if err != nil {
		t.Fatal(err)
	}
	cursor := encodeCursor(125, fingerprint)
	offset, err := decodeCursor(cursor, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 125 {
		t.Fatalf("offset = %d, want 125", offset)
	}

	otherFingerprint, err := transactionCursorFingerprint(transactionFilters{Search: "tea"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCursor(cursor, otherFingerprint); err == nil {
		t.Fatal("cursor was accepted with different filters")
	}
	if _, err := decodeCursor("not-a-cursor", fingerprint); err == nil {
		t.Fatal("invalid cursor was accepted")
	}
}

func TestCursorRejectsTrailingValuesAndUnknownFields(t *testing.T) {
	fingerprint, err := transactionCursorFingerprint(transactionFilters{})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"v":2,"o":1,"f":"` + fingerprint + `"}{}`,
		`{"v":2,"o":1,"f":"` + fingerprint + `","extra":true}`,
	} {
		cursor := base64.RawURLEncoding.EncodeToString([]byte(raw))
		if _, err := decodeCursor(cursor, fingerprint); err == nil {
			t.Errorf("decodeCursor accepted %s", raw)
		}
	}
}

func TestDateRangeUsesExactCalendarYearBoundary(t *testing.T) {
	if err := validateDateRange("2016-02-29", "2026-02-28", false); err != nil {
		t.Fatalf("exact leap-year boundary rejected: %v", err)
	}
	if err := validateDateRange("2016-02-29", "2026-03-01", false); err == nil {
		t.Fatal("date beyond the ten-calendar-year boundary was accepted")
	}
	if err := validateDateRange("2016-01-01", "2026-01-01", false); err != nil {
		t.Fatalf("exact ten-year boundary rejected: %v", err)
	}
	if err := validateDateRange("2016-01-01", "2026-01-02", false); err == nil {
		t.Fatal("date beyond the ten-year boundary was accepted")
	}
}

func TestMonthRangeAllowsExactly120InclusiveMonths(t *testing.T) {
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	start, end, err := normalizeMonthRange(MonthRange{StartMonth: "2016-01", EndMonth: "2025-12"}, now)
	if err != nil || start != "2016-01" || end != "2025-12" {
		t.Fatalf("120-month range = %q..%q, %v", start, end, err)
	}
	if _, _, err := normalizeMonthRange(MonthRange{StartMonth: "2016-01", EndMonth: "2026-01"}, now); err == nil {
		t.Fatal("121-month range was accepted")
	}
	start, end, err = normalizeMonthRange(MonthRange{}, now)
	if err != nil || start != "2026-07" || end != "2026-07" {
		t.Fatalf("default month range = %q..%q, %v", start, end, err)
	}
}

func TestOpaqueIDCountsCharactersAndAllowsPunctuation(t *testing.T) {
	if !validOpaqueID("txn:space/with_punctuation-1") {
		t.Fatal("printable opaque ID was rejected")
	}
	if !validOpaqueID(strings.Repeat("界", 200)) {
		t.Fatal("200-rune opaque ID was rejected")
	}
	for _, id := range []string{"", " leading", "trailing ", "has\ncontrol", strings.Repeat("界", 201)} {
		if validOpaqueID(id) {
			t.Errorf("invalid opaque ID %q was accepted", id)
		}
	}
}

func TestSharedTransactionConstraintsUseSemanticLengths(t *testing.T) {
	if err := ValidateTransactionSearch(strings.Repeat("界", MaxTransactionSearchLength)); err != nil {
		t.Fatalf("max-length Unicode search rejected: %v", err)
	}
	if err := ValidateTransactionSearch(strings.Repeat("界", MaxTransactionSearchLength+1)); err == nil {
		t.Fatal("overlong Unicode search was accepted")
	}
	duplicates := make([]string, MaxTransactionFilterIDs+1)
	for index := range duplicates {
		duplicates[index] = "same"
	}
	if err := ValidateTransactionFilterIDs(duplicates); err == nil {
		t.Fatal("duplicate filter IDs were accepted")
	}
	unique := make([]string, MaxTransactionFilterIDs+1)
	for index := range unique {
		unique[index] = fmt.Sprintf("id-%d", index)
	}
	if err := ValidateTransactionFilterIDs(unique); err == nil {
		t.Fatal("too many unique filter IDs were accepted")
	}
}

func FuzzAmountUnmarshalNeverPanics(f *testing.F) {
	for _, seed := range []string{`0`, `"1.23"`, `null`, `"1/2"`, `{}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		var amount Amount
		_ = json.Unmarshal([]byte(input), &amount)
	})
}

func FuzzDecodeCursorNeverPanics(f *testing.F) {
	f.Add("")
	f.Add("not-a-cursor")
	f.Add(base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"o":1,"f":"seed"}`)))
	f.Fuzz(func(t *testing.T, cursor string) {
		_, _ = decodeCursor(cursor, "seed")
	})
}
