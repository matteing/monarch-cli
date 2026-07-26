package monarch

import (
	"context"
	"net/http"
	"time"

	"github.com/matteing/monarch-cli/internal/apperr"
)

const (
	maxAttempts  = 3
	maxRetryWait = 10 * time.Second
)

type retryWaitFunc func(context.Context, int, time.Duration) error

func retryableHTTPError(operation string, status int, retryAfter time.Duration) *apperr.Error {
	kind := apperr.KindUnavailable
	message := "Monarch is temporarily unavailable"
	if status == http.StatusTooManyRequests {
		kind, message = apperr.KindRateLimited, "Monarch rate limit exceeded"
	}
	return &apperr.Error{
		Kind: kind, Op: operation, Message: message, StatusCode: status,
		Retryable: true, RetryAfter: retryAfter,
	}
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func waitForRetry(ctx context.Context, attempt int, retryAfter time.Duration) error {
	delay := retryAfter
	if delay <= 0 {
		delay = time.Duration(1<<attempt)*200*time.Millisecond + time.Duration(attempt+1)*37*time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
