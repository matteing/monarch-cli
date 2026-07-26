package httpx

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		value string
		want  time.Duration
	}{
		"seconds":  {value: " 12 ", want: 12 * time.Second},
		"date":     {value: now.Add(3 * time.Second).Format(http.TimeFormat), want: 3 * time.Second},
		"elapsed":  {value: now.Add(-time.Second).Format(http.TimeFormat), want: 0},
		"invalid":  {value: "tomorrow", want: 0},
		"overflow": {value: strings.Repeat("9", 64), want: time.Duration(1<<63 - 1)},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := RetryAfter(test.value, now); got != test.want {
				t.Fatalf("RetryAfter(%q) = %s, want %s", test.value, got, test.want)
			}
		})
	}
}
