// Package avacast is the Go SDK for avacast (https://avacast.jp/) realtime avatars:
// create sessions, make the avatar speak and read usage from your server.
//
// Use it on your server only: never ship the API key to a browser. Give the browser only the session's
// client_token and webrtc values, and show the avatar there with the browser SDK.
package avacast

import (
	"context"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the public API.
	DefaultBaseURL = "https://api.avacast.jp"
	// DefaultTimeout is the time allowed for each attempt.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxRetries is the number of retries after the first attempt.
	DefaultMaxRetries = 2
	// APIKeyEnv is read when the API key argument is empty.
	APIKeyEnv = "AVACAST_API_KEY"
)

// Client for the avacast API (/v1). It is safe for concurrent use.
type Client struct {
	Sessions *SessionsService
	Avatars  *AvatarsService
	Usage    *UsageService

	apiKey     string
	baseURL    string
	timeout    time.Duration
	maxRetries int
	httpClient *http.Client

	// replaced in tests
	sleep  func(ctx context.Context, d time.Duration) error
	random func() float64
	now    func() time.Time
	newKey func() string
}

// Option configures a [Client].
type Option func(*Client)

// WithBaseURL sets the API origin. Default https://api.avacast.jp.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithTimeout sets the time allowed for each attempt. Default 30 seconds.
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// WithMaxRetries sets the retries after the first attempt. Default 2. 0 disables retries.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = n } }

// WithHTTPClient sets the HTTP client. Default: a new http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// NewClient creates a client. apiKey is sk_live_… or sk_test_…; when empty, AVACAST_API_KEY is used.
func NewClient(apiKey string, opts ...Option) (*Client, error) {
	key := apiKey
	if key == "" {
		key = strings.TrimSpace(os.Getenv(APIKeyEnv))
	}
	if key == "" {
		return nil, newError(KindInvalidArgument, "Missing API key. Pass it to avacast.NewClient(\"sk_…\") or set "+APIKeyEnv+".", nil)
	}
	c := &Client{
		apiKey:     key,
		baseURL:    DefaultBaseURL,
		timeout:    DefaultTimeout,
		maxRetries: DefaultMaxRetries,
		sleep:      sleepContext,
		random:     rand.Float64,
		now:        time.Now,
		newKey:     newIdempotencyKey,
	}
	for _, o := range opts {
		o(c)
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{}
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	c.Sessions = &SessionsService{c}
	c.Avatars = &AvatarsService{c}
	c.Usage = &UsageService{c}
	return c, nil
}

// BaseURL returns the API origin without a trailing slash.
func (c *Client) BaseURL() string { return c.baseURL }

// RequestOption overrides client settings for one call.
type RequestOption func(*requestConfig)

type requestConfig struct {
	timeout        time.Duration
	maxRetries     int
	idempotencyKey string
}

// WithRequestTimeout sets the time allowed for each attempt of this call.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(r *requestConfig) { r.timeout = d }
}

// WithRequestMaxRetries sets the retries after the first attempt for this call.
func WithRequestMaxRetries(n int) RequestOption {
	return func(r *requestConfig) { r.maxRetries = n }
}

// WithIdempotencyKey sets the Idempotency-Key of Sessions.Create or Sessions.Speak (up to 255 characters).
// The same key returns the recorded response instead of running the request again. Generated when not set,
// and reused across retries. Other calls ignore it.
func WithIdempotencyKey(key string) RequestOption {
	return func(r *requestConfig) { r.idempotencyKey = key }
}

func (c *Client) config(opts []RequestOption) requestConfig {
	r := requestConfig{timeout: c.timeout, maxRetries: c.maxRetries}
	for _, o := range opts {
		o(&r)
	}
	return r
}

// Status reports whether the API is responding (GET /v1/status).
func (c *Client) Status(ctx context.Context, opts ...RequestOption) (*Status, error) {
	return get[Status](ctx, c, "/v1/status", nil, opts)
}

// Float64 returns a pointer to f, for optional fields (such as SessionVoice.Speed).
func Float64(f float64) *float64 { return &f }

// Int returns a pointer to n, for optional fields (such as SessionCreateParams.IdleTimeoutSeconds).
func Int(n int) *int { return &n }
