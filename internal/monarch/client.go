package monarch

import (
	_ "embed"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/matteing/monarch-cli/internal/apperr"
	"github.com/matteing/monarch-cli/internal/httpx"
	"github.com/matteing/monarch-cli/internal/session"
)

const (
	graphqlURL   = "https://api.monarch.com/graphql"
	requestLimit = 4
)

var (
	//go:embed query/accounts.graphql
	accountsQuery string
	//go:embed query/transactions.graphql
	transactionsQuery string
	//go:embed query/transaction.graphql
	transactionQuery string
	//go:embed query/categories.graphql
	categoriesQuery string
	//go:embed query/budgets.graphql
	budgetsQuery string
	//go:embed query/cashflow.graphql
	cashflowQuery string
)

// Client executes the package's fixed set of read-only GraphQL documents.
// It is safe for concurrent use. Authentication state is copied at creation
// time and cannot be changed through the source Session afterward.
type Client struct {
	httpClient    *http.Client
	endpoint      string
	authorization string
	cookieHeader  string
	csrfToken     string
	cookieMode    bool
	requests      chan struct{}
	now           func() time.Time
	retryWait     retryWaitFunc
}

// NewClient constructs a bounded, concurrency-safe Monarch reader.
func NewClient(value session.Session, timeout time.Duration) (*Client, error) {
	if err := value.Validate(); err != nil {
		return nil, apperr.New(apperr.KindAuth, "create client", "saved Monarch session is invalid", err)
	}
	if timeout <= 0 {
		return nil, apperr.New(apperr.KindInvalidInput, "create client", "request timeout must be positive", nil)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 16
	transport.MaxIdleConnsPerHost = 8
	transport.MaxConnsPerHost = 8
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = min(timeout, 15*time.Second)
	return newClient(value, &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: httpx.RejectRedirects,
	}, graphqlURL)
}

func newClient(value session.Session, httpClient *http.Client, endpoint string) (*Client, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, errors.New("HTTP client cannot be nil")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("invalid GraphQL endpoint")
	}

	client := &Client{
		httpClient: httpClient,
		endpoint:   endpoint,
		requests:   make(chan struct{}, requestLimit),
		now:        time.Now,
		retryWait:  waitForRetry,
	}
	switch value.Mode {
	case session.ModeToken:
		client.authorization = "Token " + value.Token()
	case session.ModeCookie:
		cookies := value.Cookies()
		names := make([]string, 0, len(cookies))
		for name := range cookies {
			names = append(names, name)
		}
		sort.Strings(names)
		pairs := make([]string, 0, len(names))
		for _, name := range names {
			pairs = append(pairs, name+"="+cookies[name])
		}
		client.cookieHeader = strings.Join(pairs, "; ")
		client.csrfToken = value.Cookie("csrftoken")
		client.cookieMode = true
	}
	return client, nil
}
