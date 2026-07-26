package httpx

import (
	"errors"
	"net/http"
	"testing"
)

func TestRejectRedirects(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "http://api.monarch.com/downgrade", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := RejectRedirects(request, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("RejectRedirects() = %v, want http.ErrUseLastResponse", err)
	}
}
