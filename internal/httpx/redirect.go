// Package httpx contains the shared HTTP safety policy used by Monarch clients.
package httpx

import "net/http"

// RejectRedirects prevents credential-bearing requests from being replayed to
// an unexpected origin or across an HTTPS downgrade.
func RejectRedirects(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}
