package monarch

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	cursorVersion    = 2
	transactionOrder = "date"
)

type cursorPayload struct {
	Version     int    `json:"v"`
	Offset      int    `json:"o"`
	Fingerprint string `json:"f"`
}

type cursorScope struct {
	OrderBy string             `json:"order_by"`
	Filters transactionFilters `json:"filters"`
}

func transactionCursorFingerprint(filters transactionFilters) (string, error) {
	raw, err := json.Marshal(cursorScope{OrderBy: transactionOrder, Filters: filters})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(digest[:16]), nil
}

func encodeCursor(offset int, fingerprint string) string {
	raw, err := json.Marshal(cursorPayload{Version: cursorVersion, Offset: offset, Fingerprint: fingerprint})
	if err != nil {
		panic(fmt.Sprintf("encode cursor invariant failed: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(cursor, expectedFingerprint string) (int, error) {
	value, err := decodeTransactionCursor(cursor)
	if err != nil {
		return 0, err
	}
	if cursor == "" {
		return 0, nil
	}
	if expectedFingerprint == "" || subtle.ConstantTimeCompare([]byte(value.Fingerprint), []byte(expectedFingerprint)) != 1 {
		return 0, errors.New("cursor filter fingerprint does not match")
	}
	return value.Offset, nil
}

func decodeTransactionCursor(cursor string) (cursorPayload, error) {
	if cursor == "" {
		return cursorPayload{}, nil
	}
	if len(cursor) > MaxTransactionCursorLength {
		return cursorPayload{}, errors.New("cursor is too long")
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return cursorPayload{}, err
	}
	var value cursorPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return cursorPayload{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return cursorPayload{}, errors.New("cursor contains more than one JSON value")
		}
		return cursorPayload{}, err
	}
	if value.Version != cursorVersion || value.Offset < 0 || value.Offset > maxTransactionOffset || !validCursorFingerprint(value.Fingerprint) {
		return cursorPayload{}, errors.New("unsupported cursor")
	}
	return value, nil
}

func validCursorFingerprint(value string) bool {
	if len(value) != 22 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == 16 && base64.RawURLEncoding.EncodeToString(raw) == value
}

func normalizedIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	result := append([]string(nil), ids...)
	sort.Strings(result)
	write := 0
	for _, id := range result {
		if write > 0 && result[write-1] == id {
			continue
		}
		result[write] = id
		write++
	}
	return result[:write]
}

func validateIDs(groups ...[]string) error {
	for _, ids := range groups {
		if len(ids) > MaxTransactionFilterIDs {
			return fmt.Errorf("each ID filter is limited to %d values", MaxTransactionFilterIDs)
		}
		if err := validateUniqueOpaqueIDs(ids, "filter IDs"); err != nil {
			return err
		}
	}
	return nil
}

func validateUniqueOpaqueIDs(ids []string, label string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !validOpaqueID(id) {
			return fmt.Errorf("%s must be 1-%d printable characters", label, MaxOpaqueIDLength)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%s must be unique", label)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validOpaqueID(id string) bool {
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > MaxOpaqueIDLength || strings.TrimSpace(id) != id {
		return false
	}
	for _, r := range id {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func validateDateRange(start, end string, allowEmpty bool) error {
	if allowEmpty && start == "" && end == "" {
		return nil
	}
	if start == "" || end == "" {
		return errors.New("start and end dates must be provided together")
	}
	startTime, err := parseDate(start)
	if err != nil {
		return errors.New("start date must use YYYY-MM-DD")
	}
	endTime, err := parseDate(end)
	if err != nil {
		return errors.New("end date must use YYYY-MM-DD")
	}
	if endTime.Before(startTime) {
		return errors.New("end date must not precede start date")
	}
	if endTime.After(addCalendarYears(startTime, 10)) {
		return errors.New("date range cannot exceed ten years")
	}
	return nil
}

func addCalendarYears(value time.Time, years int) time.Time {
	year, month, day := value.Year()+years, value.Month(), value.Day()
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, value.Location()).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func normalizeMonthRange(months MonthRange, now time.Time) (string, string, error) {
	start, end := months.StartMonth, months.EndMonth
	if start == "" && end == "" {
		start = currentMonth(now)
		end = start
	}
	if start == "" || end == "" {
		return "", "", errors.New("start and end months must be provided together")
	}
	startTime, err := parseMonth(start)
	if err != nil {
		return "", "", errors.New("start month must use YYYY-MM")
	}
	endTime, err := parseMonth(end)
	if err != nil {
		return "", "", errors.New("end month must use YYYY-MM")
	}
	if endTime.Before(startTime) {
		return "", "", errors.New("end month must not precede start month")
	}
	monthDifference := (endTime.Year()-startTime.Year())*12 + int(endTime.Month()-startTime.Month())
	if monthDifference >= 120 {
		return "", "", errors.New("month range cannot exceed ten years")
	}
	return start, end, nil
}

func parseDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, errors.New("invalid ISO date")
	}
	return parsed, nil
}

func parseMonth(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01", value)
	if err != nil || parsed.Format("2006-01") != value {
		return time.Time{}, errors.New("invalid ISO month")
	}
	return parsed, nil
}

func currentMonth(now time.Time) string { return now.Format("2006-01") }

func currentMonthDates(now time.Time) (string, string) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 1, -1)
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}
