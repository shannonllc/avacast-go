package avacast

import (
	"context"
	"net/url"
	"time"
)

func esc(id string) string { return url.PathEscape(id) }

func get[T any](ctx context.Context, c *Client, path string, q url.Values, opts []RequestOption) (*T, error) {
	var out T
	if err := c.do(ctx, request{method: "GET", path: path, query: q, idempotent: true}, &out, c.config(opts)); err != nil {
		return nil, err
	}
	return &out, nil
}

func send[T any](ctx context.Context, c *Client, req request, opts []RequestOption) (*T, error) {
	var out T
	if err := c.do(ctx, req, &out, c.config(opts)); err != nil {
		return nil, err
	}
	return &out, nil
}

// idempotencyKey is the key given with WithIdempotencyKey, or a new one.
func (c *Client) idempotencyKey(opts []RequestOption) string {
	if k := c.config(opts).idempotencyKey; k != "" {
		return k
	}
	return c.newKey()
}

// SessionsService opens an avatar, makes it speak and closes it (/v1/sessions).
type SessionsService struct{ c *Client }

func sessionPath(id, rest string) string { return "/v1/sessions/" + esc(id) + rest }

// Create opens a session. Pass only ClientToken and WebRTC to the browser, never the API key.
// It sends an Idempotency-Key (generated unless WithIdempotencyKey is given), so it is retried safely on a 5xx or a
// connection error. Fails with KindPaymentRequired at the monthly limit, and KindRateLimit (concurrency_limit) when
// too many sessions are open.
func (s *SessionsService) Create(ctx context.Context, params SessionCreateParams, opts ...RequestOption) (*Session, error) {
	return send[Session](ctx, s.c, request{method: "POST", path: "/v1/sessions", body: params, idempotencyKey: s.c.idempotencyKey(opts), idempotent: true}, opts)
}

// Get returns the state, connected time (what you are billed for) and total speaking time.
func (s *SessionsService) Get(ctx context.Context, sessionID string, opts ...RequestOption) (*SessionState, error) {
	return get[SessionState](ctx, s.c, sessionPath(sessionID, ""), nil, opts)
}

// End ends the session. Ending one that has already ended is not an error.
func (s *SessionsService) End(ctx context.Context, sessionID string, opts ...RequestOption) (*EndedSession, error) {
	return send[EndedSession](ctx, s.c, request{method: "DELETE", path: sessionPath(sessionID, ""), idempotent: true}, opts)
}

type speakBody struct {
	Text string `json:"text"`
}

// Speak queues a line (up to 1,000 characters). No need to wait for the previous line.
// Fails with KindConflict (session_not_connected) before the browser has connected, and KindRateLimit (queue_full)
// when 20 lines are waiting. It sends an Idempotency-Key, so it is retried without speaking twice.
func (s *SessionsService) Speak(ctx context.Context, sessionID, text string, opts ...RequestOption) (*Utterance, error) {
	return send[Utterance](ctx, s.c, request{method: "POST", path: sessionPath(sessionID, "/speak"), body: speakBody{text}, idempotencyKey: s.c.idempotencyKey(opts), idempotent: true}, opts)
}

// Interrupt stops the current line and discards the queue. It is not retried on a 5xx or a connection error (it
// could cut a newer line).
func (s *SessionsService) Interrupt(ctx context.Context, sessionID string, opts ...RequestOption) (*InterruptResult, error) {
	return send[InterruptResult](ctx, s.c, request{method: "POST", path: sessionPath(sessionID, "/interrupt"), idempotent: false}, opts)
}

// AvatarsService lists the avatars you can use (/v1/avatars).
type AvatarsService struct{ c *Client }

// List lists the preset avatars and the ones you made, all at once.
func (s *AvatarsService) List(ctx context.Context, opts ...RequestOption) (*List[Avatar], error) {
	return get[List[Avatar]](ctx, s.c, "/v1/avatars", nil, opts)
}

// UsageService reads connected time and sessions (/v1/usage).
type UsageService struct{ c *Client }

// Get returns this month (UTC) with your limits.
func (s *UsageService) Get(ctx context.Context, opts ...RequestOption) (*MonthlyUsage, error) {
	return get[MonthlyUsage](ctx, s.c, "/v1/usage", nil, opts)
}

// UsageRangeParams selects the period of UsageService.Range. Set at least one field.
type UsageRangeParams struct {
	// From is the start of the period. Zero: the start of this month (UTC).
	From time.Time
	// To is the end of the period (not included). Zero: now. Up to 366 days after From.
	To time.Time
	// GroupBy is "metadata.<key>", such as "metadata.tenant".
	GroupBy string
}

// Range returns a period of your choice, optionally split by a metadata key.
func (s *UsageService) Range(ctx context.Context, params UsageRangeParams, opts ...RequestOption) (*RangeUsage, error) {
	q := url.Values{}
	if !params.From.IsZero() {
		q.Set("from", params.From.UTC().Format(time.RFC3339Nano))
	}
	if !params.To.IsZero() {
		q.Set("to", params.To.UTC().Format(time.RFC3339Nano))
	}
	if params.GroupBy != "" {
		q.Set("group_by", params.GroupBy)
	}
	if len(q) == 0 {
		return nil, newError(KindInvalidArgument, "Usage.Range needs From, To or GroupBy. Use Usage.Get for this month.", nil)
	}
	return get[RangeUsage](ctx, s.c, "/v1/usage", q, opts)
}
