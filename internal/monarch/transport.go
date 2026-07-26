package monarch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/matteing/monarch-cli/internal/buildinfo"
)

const (
	maxResponseSize  = 10 << 20
	webClientVersion = "2025.05"
)

var errResponseTooLarge = errors.New("response exceeds 10 MiB limit")

func (c *Client) do(ctx context.Context, payload []byte) ([]byte, int, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	req.Header.Set("Client-Platform", "web")
	if c.authorization != "" {
		req.Header.Set("Authorization", c.authorization)
	}
	if c.cookieMode {
		req.Header.Set("Cookie", c.cookieHeader)
		req.Header.Set("Origin", "https://app.monarch.com")
		req.Header.Set("Referer", "https://app.monarch.com/")
		req.Header.Set("X-Csrftoken", c.csrfToken)
		req.Header.Set("monarch-client", "web")
		req.Header.Set("monarch-client-version", webClientVersion)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, resp.StatusCode, resp.Header, err
	}
	if len(body) > maxResponseSize {
		return nil, resp.StatusCode, resp.Header, errResponseTooLarge
	}
	return body, resp.StatusCode, resp.Header, nil
}
