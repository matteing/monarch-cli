package httpx

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryAfter parses the HTTP Retry-After grammar without overflowing
// time.Duration. Invalid or elapsed values return zero; unrepresentable delays
// saturate so callers never accidentally replace them with a shorter retry.
func RetryAfter(value string, now time.Time) time.Duration {
	const maxDuration = time.Duration(1<<63 - 1)
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds > uint64(maxDuration/time.Second) {
			return maxDuration
		}
		return time.Duration(seconds) * time.Second
	} else if errors.Is(err, strconv.ErrRange) {
		return maxDuration
	}
	if when, err := http.ParseTime(value); err == nil {
		if duration := when.Sub(now); duration > 0 {
			return duration
		}
	}
	return 0
}
