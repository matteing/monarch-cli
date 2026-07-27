package monarch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/httpx"
)

type graphQLRequest struct {
	Query     string `json:"query"`
	Variables any    `json:"variables,omitempty"`
}

type graphQLError struct {
	Message    string `json:"message"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

type graphQLResponse[T any] struct {
	Data   *T             `json:"data"`
	Errors []graphQLError `json:"errors"`
}

// responseField distinguishes an absent field, an explicit null, and a value.
// That distinction is essential at the private API boundary: a missing root is
// schema drift, while a nullable lookup root can legitimately mean not found.
type responseField[T any] struct {
	Value   T
	Present bool
	Null    bool
}

func (f *responseField[T]) UnmarshalJSON(data []byte) error {
	f.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		f.Null = true
		var zero T
		f.Value = zero
		return nil
	}
	f.Null = false
	return json.Unmarshal(data, &f.Value)
}

func execute[T any](ctx context.Context, client *Client, operation, query string, variables any) (T, error) {
	return executeWithAttempts[T](ctx, client, operation, query, variables, maxAttempts)
}

// executeMutation does not retry because a transport failure can happen after
// Monarch accepted the mutation, making its outcome ambiguous to the client.
func executeMutation[T any](ctx context.Context, client *Client, operation, query string, variables any) (T, error) {
	return executeWithAttempts[T](ctx, client, operation, query, variables, 1)
}

func executeWithAttempts[T any](ctx context.Context, client *Client, operation, query string, variables any, attempts int) (T, error) {
	var zero T
	payload, err := json.Marshal(graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return zero, apperr.New(apperr.KindInternal, operation, "could not encode Monarch request", err)
	}
	select {
	case client.requests <- struct{}{}:
		defer func() { <-client.requests }()
	case <-ctx.Done():
		return zero, ctx.Err()
	}

	for attempt := 0; attempt < attempts; attempt++ {
		body, status, header, requestErr := client.do(ctx, payload)
		if requestErr != nil {
			if ctx.Err() != nil {
				return zero, ctx.Err()
			}
			if errors.Is(requestErr, errResponseTooLarge) {
				return zero, apperr.New(apperr.KindUnavailable, operation, "Monarch response exceeded the safety limit", requestErr)
			}
			if attempt+1 < attempts {
				if err := client.retryWait(ctx, attempt, 0); err != nil {
					return zero, err
				}
				continue
			}
			return zero, &apperr.Error{
				Kind: apperr.KindUnavailable, Op: operation, Message: "Monarch is unavailable",
				Retryable: true, Err: requestErr,
			}
		}

		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return zero, &apperr.Error{
				Kind: apperr.KindAuth, Op: operation,
				Message: "Monarch session is expired; run `monarch auth login`", StatusCode: status,
			}
		}
		if retryableStatus(status) {
			retryAfter := httpx.RetryAfter(header.Get("Retry-After"), client.now())
			statusErr := retryableHTTPError(operation, status, retryAfter)
			if attempt+1 < attempts {
				// Never turn a server-requested delay into a shorter retry. If the
				// delay exceeds this CLI's bounded wait, surface it to the caller.
				if retryAfter > maxRetryWait {
					return zero, statusErr
				}
				if err := client.retryWait(ctx, attempt, retryAfter); err != nil {
					return zero, err
				}
				continue
			}
			return zero, statusErr
		}
		if status < 200 || status >= 300 {
			return zero, &apperr.Error{
				Kind: apperr.KindUnavailable, Op: operation,
				Message: fmt.Sprintf("Monarch returned HTTP %d", status), StatusCode: status,
			}
		}

		var envelope graphQLResponse[T]
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		if err := decoder.Decode(&envelope); err != nil {
			return zero, decodeResponseError(operation, err)
		}
		if err := requireJSONEOF(decoder); err != nil {
			return zero, decodeResponseError(operation, err)
		}
		if len(envelope.Errors) > 0 {
			appErr := classifyGraphQLErrors(operation, envelope.Errors)
			if appErr.Retryable {
				appErr.RetryAfter = httpx.RetryAfter(header.Get("Retry-After"), client.now())
			}
			if appErr.Retryable && attempt+1 < attempts {
				if appErr.RetryAfter > maxRetryWait {
					return zero, appErr
				}
				if err := client.retryWait(ctx, attempt, appErr.RetryAfter); err != nil {
					return zero, err
				}
				continue
			}
			return zero, appErr
		}
		if envelope.Data == nil {
			return zero, unexpectedResponse(operation, "GraphQL response omitted data")
		}
		return *envelope.Data, nil
	}
	return zero, apperr.New(apperr.KindInternal, operation, "request retry loop ended unexpectedly", nil)
}

func decodeResponseError(operation string, err error) error {
	message := "Monarch returned an unexpected response shape"
	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		message = "Monarch returned malformed JSON"
	}
	return apperr.New(apperr.KindUnavailable, operation, message, err)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("response contains more than one JSON value")
	}
	return err
}

func unexpectedResponse(operation, detail string) error {
	return apperr.New(
		apperr.KindUnavailable,
		operation,
		"Monarch returned an unexpected response shape",
		errors.New(detail),
	)
}

func classifyGraphQLErrors(operation string, graphQLErrors []graphQLError) *apperr.Error {
	errorCodes := make([]string, 0, len(graphQLErrors))
	for _, graphErr := range graphQLErrors {
		errorCodes = append(errorCodes, graphErr.Extensions.Code)
	}
	return classifyErrorCodes(operation, errorCodes)
}

func classifyErrorCodes(operation string, errorCodes []string) *apperr.Error {
	codes := make([]string, 0, len(errorCodes))
	auth, rateLimited, notFound, invalidInput, retryable := false, false, false, false, false
	for _, errorCode := range errorCodes {
		code := strings.ToUpper(strings.TrimSpace(errorCode))
		if code == "" {
			continue
		}
		codes = append(codes, code)
		switch {
		case strings.Contains(code, "UNAUTH") || strings.Contains(code, "AUTHENTICATION") ||
			strings.Contains(code, "AUTHORIZATION") || strings.Contains(code, "FORBIDDEN") ||
			strings.Contains(code, "PERMISSION_DENIED") || code == "AUTH_ERROR":
			auth = true
		case strings.Contains(code, "RATE_LIMIT") || strings.Contains(code, "TOO_MANY_REQUESTS"):
			rateLimited, retryable = true, true
		case strings.Contains(code, "NOT_FOUND"):
			notFound = true
		case code == "BAD_USER_INPUT" || code == "INVALID_INPUT":
			invalidInput = true
		case strings.Contains(code, "INTERNAL") || strings.Contains(code, "UNAVAILABLE") || strings.Contains(code, "TIMEOUT") || strings.Contains(code, "TEMPORARY"):
			retryable = true
		}
	}
	sort.Strings(codes)
	cause := errors.New("monarch response contained errors")
	if len(codes) > 0 {
		cause = fmt.Errorf("monarch error codes: %s", strings.Join(codes, ","))
	}
	switch {
	case auth:
		return &apperr.Error{Kind: apperr.KindAuth, Op: operation, Message: "Monarch session is expired; run `monarch auth login`", Err: cause}
	case rateLimited:
		return &apperr.Error{Kind: apperr.KindRateLimited, Op: operation, Message: "Monarch rate limit exceeded", Retryable: true, Err: cause}
	case notFound:
		return &apperr.Error{Kind: apperr.KindNotFound, Op: operation, Message: "Monarch resource was not found", Err: cause}
	case invalidInput:
		return &apperr.Error{Kind: apperr.KindInvalidInput, Op: operation, Message: "Monarch rejected the request input", Err: cause}
	default:
		return &apperr.Error{Kind: apperr.KindUnavailable, Op: operation, Message: "Monarch could not complete the request", Retryable: retryable, Err: cause}
	}
}
