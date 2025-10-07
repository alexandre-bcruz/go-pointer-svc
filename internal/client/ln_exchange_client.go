package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"pointersvc/internal/domain"
)

const (
	defaultBaseURL      = "http://localhost:3007"
	defaultUserAgent    = "pointer-svc/ln-exchange-client"
	maxErrorBodyPreview = 4096
)

// Interface for tests/mocks.
type LNExchange interface {
	ApplyTakeProfit(ctx context.Context, profitCfg domain.ProfitCfg, accountID string) error
	GetTradesByAccount(ctx context.Context, accountID string, status string) (domain.LNOrders, error)
}

// Concrete client.
type LNExchangeClient struct {
	base *url.URL
	http *http.Client
	ua   string
}

// Option pattern.
type Option func(*LNExchangeClient)

func WithHTTPClient(hc *http.Client) Option {
	return func(c *LNExchangeClient) {
		if hc != nil {
			c.http = hc
		}
	}
}

func WithUserAgent(ua string) Option {
	return func(c *LNExchangeClient) {
		if ua != "" {
			c.ua = ua
		}
	}
}

func WithBaseURL(raw string) Option {
	return func(c *LNExchangeClient) {
		if raw == "" {
			return
		}
		u, err := url.Parse(raw)
		if err == nil && u.Scheme != "" && u.Host != "" {
			c.base = u
		}
	}
}

func NewLNExchangeClient(opts ...Option) *LNExchangeClient {
	raw := os.Getenv("LN_EXCHANGE_SERVICE_URL")
	if raw == "" {
		raw = defaultBaseURL
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// Fallback to default if env is malformed.
		u, _ = url.Parse(defaultBaseURL)
	}

	c := &LNExchangeClient{
		base: u,
		http: &http.Client{Timeout: 10 * time.Second},
		ua:   defaultUserAgent,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}


// ApplyTakeProfit sends ProfitCfg to /ln-exchange/accounts/{accountID}/strategies/takeprofit:apply
func (c *LNExchangeClient) ApplyTakeProfit(ctx context.Context, takeProfitReq domain.TakeProfitRequest) error {
	profitCfg := takeProfitReq.ProfitCfg

	endpoint, err := url.JoinPath(
		c.base.String(),
		"ln-exchange", "strategies", "takeprofit:apply",
	)
	if err != nil {
		return fmt.Errorf("join path: %w", err)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(profitCfg); err != nil {
		return fmt.Errorf("encode body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.ua)

	return c.doJSON(req, nil)
}

// GetTradesByAccount calls /ln-exchange/orders/account/{accountID}?status={status}
func (c *LNExchangeClient) GetTradesByAccount(ctx context.Context, accountID string, status string) (domain.LNOrders, error) {
	if accountID == "" {
		return domain.LNOrders{}, errors.New("accountID is required")
	}

	endpoint, err := url.JoinPath(c.base.String(), "ln-exchange", "orders", "account", accountID)
	if err != nil {
		return domain.LNOrders{}, fmt.Errorf("join path: %w", err)
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return domain.LNOrders{}, fmt.Errorf("parse endpoint: %w", err)
	}
	if status != "" {
		q := u.Query()
		q.Set("status", status)
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return domain.LNOrders{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.ua)

	var out domain.LNOrders
	if err := c.doJSON(req, &out); err != nil {
		return domain.LNOrders{}, err
	}
	return out, nil
}

// doJSON executes the request, checks for 2xx, and optionally decodes JSON into v.
func (c *LNExchangeClient) doJSON(req *http.Request, v any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyPreview))
		// Try to parse JSON error body to enrich message (best effort).
		var parsed any
		if len(preview) > 0 {
			_ = json.Unmarshal(preview, &parsed)
		}
		if parsed != nil {
			return fmt.Errorf("ln-exchange-service %d: %v", resp.StatusCode, parsed)
		}
		return fmt.Errorf("ln-exchange-service %d: %s", resp.StatusCode, string(preview))
	}

	if v == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}