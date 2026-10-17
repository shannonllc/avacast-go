package avacast

import (
	"context"
	"errors"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var stateBody = map[string]any{"session_id": "ses_1", "status": "active"}

func TestClientOptions(t *testing.T) {
	t.Run("headers", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(200, stateBody))
		c, _ := testClient(t, f)
		if _, err := c.Sessions.Get(context.Background(), "ses_1"); err != nil {
			t.Fatal(err)
		}
		h := f.Last().Header
		for k, want := range map[string]string{"Authorization": "Bearer sk_test_abc", "Accept": "application/json", "User-Agent": "avacast-go/" + Version} {
			if got := h.Get(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
		}
		if h.Get("Content-Type") != "" || h.Get("Idempotency-Key") != "" || len(f.Last().Raw) != 0 {
			t.Errorf("GET sent a body")
		}
	})

	t.Run("body and content type", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(202, map[string]any{"utterance_id": "utt_1", "status": "queued", "queue_length": 1}))
		c, _ := testClient(t, f)
		r, err := c.Sessions.Speak(context.Background(), "ses_1", "ご相談ありがとうございます。")
		if err != nil || r.UtteranceID != "utt_1" || r.QueueLength != 1 {
			t.Fatal(r, err)
		}
		if f.Last().Header.Get("Content-Type") != "application/json" || string(f.Last().Raw) != `{"text":"ご相談ありがとうございます。"}` {
			t.Fatal(string(f.Last().Raw))
		}
	})

	t.Run("user agent format", func(t *testing.T) {
		if !regexp.MustCompile(`^avacast-go/\d+\.\d+\.\d+$`).MatchString(UserAgent) {
			t.Fatal(UserAgent)
		}
	})

	t.Run("api key from env", func(t *testing.T) {
		t.Setenv(APIKeyEnv, " sk_live_env ")
		f := newFakeAPI(t, jsonReply(200, stateBody))
		c, err := NewClient("", WithBaseURL(f.srv.URL))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = c.Sessions.Get(context.Background(), "ses_1")
		if got := f.Last().Header.Get("Authorization"); got != "Bearer sk_live_env" {
			t.Fatal(got)
		}
	})

	t.Run("missing api key", func(t *testing.T) {
		t.Setenv(APIKeyEnv, "  ")
		_, err := NewClient("")
		if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), APIKeyEnv) {
			t.Fatal(err)
		}
	})

	t.Run("base url and options", func(t *testing.T) {
		c, _ := NewClient("sk_test_x")
		if c.BaseURL() != "https://api.avacast.jp" {
			t.Fatal(c.BaseURL())
		}
		c, _ = NewClient("sk_test_x", WithBaseURL("https://api.example.test//"))
		if c.BaseURL() != "https://api.example.test" {
			t.Fatal(c.BaseURL())
		}
		if c.timeout != DefaultTimeout || c.maxRetries != DefaultMaxRetries || c.httpClient == nil {
			t.Fatal("defaults")
		}
		h := &http.Client{}
		c, _ = NewClient("sk_test_x", WithHTTPClient(h), WithTimeout(time.Second), WithMaxRetries(5))
		if c.httpClient != h || c.timeout != time.Second || c.maxRetries != 5 {
			t.Fatal("options")
		}
	})

	t.Run("version matches the changelog", func(t *testing.T) {
		b, err := os.ReadFile("CHANGELOG.md")
		if err != nil || !strings.Contains(string(b), "## "+Version) {
			t.Fatal("CHANGELOG.md has no entry for", Version)
		}
	})
}

func TestErrors(t *testing.T) {
	cases := []struct {
		status   int
		code     string
		kind     Kind
		sentinel error
	}{
		{400, "invalid_request", KindValidation, ErrValidation},
		{401, "invalid_api_key", KindAuthentication, ErrAuthentication},
		{402, "payment_required", KindPaymentRequired, ErrPaymentRequired},
		{403, "forbidden", KindPermission, ErrPermission},
		{404, "session_not_found", KindNotFound, ErrNotFound},
		{409, "session_not_connected", KindConflict, ErrConflict},
		{418, "teapot", KindAPI, ErrAPI},
		{422, "invalid_request", KindValidation, ErrValidation},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			f := newFakeAPI(t, jsonReply(tc.status, apiError(tc.code, "m"), "x-request-id", "req_1"))
			c, _ := testClient(t, f)
			_, err := c.Sessions.Get(context.Background(), "ses_1")
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("%T", err)
			}
			if e.Kind != tc.kind || e.Status != tc.status || e.Code != tc.code || e.Message != "m" || e.RequestID != "req_1" {
				t.Fatalf("%+v", e)
			}
			if !errors.Is(err, tc.sentinel) || errors.Is(err, ErrConnection) || err.Error() != "m" {
				t.Fatal("sentinel")
			}
			if len(f.Calls()) != 1 {
				t.Fatal("retried")
			}
		})
	}

	t.Run("request id falls back to cf-ray", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(404, apiError("not_found", "x"), "cf-ray", "r"))
		c, _ := testClient(t, f)
		_, err := c.Sessions.Get(context.Background(), "a")
		var e *Error
		errors.As(err, &e)
		if e.RequestID != "r" {
			t.Fatalf("%+v", e)
		}
	})

	t.Run("older body shape", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(404, map[string]any{"error": "not_found", "message": "gone"}))
		c, _ := testClient(t, f)
		_, err := c.Sessions.Get(context.Background(), "a")
		var e *Error
		errors.As(err, &e)
		if e.Code != "not_found" || e.Message != "gone" {
			t.Fatalf("%+v", e)
		}
	})

	t.Run("non-json error body and fields of the wrong type", func(t *testing.T) {
		f := newFakeAPI(t, rawReply(502, "<html>bad gateway</html>"), rawReply(400, "null"), rawReply(500, `"oops"`),
			rawReply(404, `{"error":1,"message":null}`), rawReply(409, `{"error":{"code":1,"message":2}}`), rawReply(409, `{"error":{oops}`))
		c, _ := testClient(t, f, WithMaxRetries(0))
		for _, want := range []struct {
			status int
			kind   Kind
		}{{502, KindAPI}, {400, KindValidation}, {500, KindAPI}, {404, KindNotFound}, {409, KindConflict}, {409, KindConflict}} {
			_, err := c.Sessions.Get(context.Background(), "a")
			var e *Error
			errors.As(err, &e)
			if e.Status != want.status || e.Kind != want.kind || e.Code != "" || e.Message != "HTTP "+strconv.Itoa(want.status) || e.RequestID != "" {
				t.Fatalf("%+v", e)
			}
		}
	})

	t.Run("invalid json in 2xx is not retried", func(t *testing.T) {
		f := newFakeAPI(t, rawReply(200, "{oops", "x-request-id", "r2"))
		c, _ := testClient(t, f)
		_, err := c.Sessions.Get(context.Background(), "a")
		var e *Error
		errors.As(err, &e)
		if e.Kind != KindAPI || e.Status != 200 || e.RequestID != "r2" || !strings.Contains(e.Message, "Invalid JSON") || len(f.Calls()) != 1 {
			t.Fatalf("%+v", e)
		}
	})

	t.Run("json of the wrong shape in 2xx", func(t *testing.T) {
		f := newFakeAPI(t, rawReply(200, `{"session_id": 1}`))
		c, _ := testClient(t, f)
		_, err := c.Sessions.Get(context.Background(), "a")
		var e *Error
		errors.As(err, &e)
		if e.Kind != KindAPI || !strings.Contains(e.Message, "Unexpected response") || e.Unwrap() == nil {
			t.Fatalf("%+v", e)
		}
	})

	t.Run("empty 2xx body", func(t *testing.T) {
		f := newFakeAPI(t, rawReply(204, ""))
		c, _ := testClient(t, f)
		r, err := c.Sessions.End(context.Background(), "ses_1")
		if err != nil || r.SessionID != "" {
			t.Fatal(r, err)
		}
	})

	t.Run("connection error keeps the cause", func(t *testing.T) {
		f := newFakeAPI(t, dropReply)
		c, _ := testClient(t, f, WithMaxRetries(0))
		_, err := c.Sessions.Get(context.Background(), "a")
		var e *Error
		errors.As(err, &e)
		if e.Kind != KindConnection || e.Status != 0 || e.Err == nil || !strings.HasPrefix(e.Message, "Connection error: ") {
			t.Fatalf("%+v", e)
		}
		if !errors.Is(err, ErrConnection) || errors.Is(err, ErrTimeout) {
			t.Fatal("sentinel")
		}
	})

	t.Run("unencodable body", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(200, map[string]any{}))
		c, _ := testClient(t, f)
		_, err := c.Sessions.Create(context.Background(), SessionCreateParams{AvatarID: "a", Metadata: map[string]any{"x": make(chan int)}})
		if !errors.Is(err, ErrInvalidArgument) || len(f.Calls()) != 0 {
			t.Fatal(err)
		}
	})

	t.Run("invalid base url", func(t *testing.T) {
		c, _ := NewClient("sk_test_x", WithBaseURL("http://bad host"))
		_, err := c.Sessions.Get(context.Background(), "a")
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatal(err)
		}
	})
}

func TestRetries(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		replies   []reply
		opts      []Option
		op        func(c *Client) error
		calls     int
		sleeps    []time.Duration
		wantError error
	}{
		{
			name:    "5xx with backoff, then success",
			replies: []reply{jsonReply(500, apiError("internal_error", "x")), jsonReply(503, apiError("gpu_unavailable", "busy")), jsonReply(200, stateBody)},
			op:      func(c *Client) error { _, err := c.Sessions.Get(ctx, "ses_1"); return err },
			calls:   3, sleeps: []time.Duration{500 * time.Millisecond, time.Second},
		},
		{
			name:    "gives up after max retries",
			replies: []reply{jsonReply(503, apiError("gpu_unavailable", "busy"))},
			opts:    []Option{WithMaxRetries(3)},
			op:      func(c *Client) error { _, err := c.Sessions.Get(ctx, "a"); return err },
			calls:   4, sleeps: []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second}, wantError: ErrAPI,
		},
		{
			name:    "503 maintenance waits retry-after",
			replies: []reply{jsonReply(503, apiError("maintenance", "x"), "retry-after", "5"), jsonReply(200, stateBody)},
			op:      func(c *Client) error { _, err := c.Sessions.Get(ctx, "a"); return err },
			calls:   2, sleeps: []time.Duration{5 * time.Second},
		},
		{
			name:    "503 asking for more than a minute returned at once",
			replies: []reply{jsonReply(503, apiError("maintenance", "x"), "retry-after", "600")},
			op:      func(c *Client) error { _, err := c.Sessions.Get(ctx, "a"); return err },
			calls:   1, wantError: ErrAPI,
		},
		{
			name:    "max retries per request",
			replies: []reply{jsonReply(500, map[string]any{})},
			opts:    []Option{WithMaxRetries(5)},
			op:      func(c *Client) error { _, err := c.Sessions.Get(ctx, "a", WithRequestMaxRetries(0)); return err },
			calls:   1, wantError: ErrAPI,
		},
		{
			name:    "connection errors retried for GET and DELETE",
			replies: []reply{dropReply, jsonReply(200, map[string]any{"data": []any{}}), dropReply, jsonReply(200, map[string]any{"status": "ended"})},
			op: func(c *Client) error {
				if _, err := c.Avatars.List(ctx); err != nil {
					return err
				}
				_, err := c.Sessions.End(ctx, "ses_1")
				return err
			},
			calls: 4, sleeps: []time.Duration{500 * time.Millisecond, 500 * time.Millisecond},
		},
		{
			name:    "create is retried with an Idempotency-Key",
			replies: []reply{jsonReply(502, map[string]any{}), jsonReply(201, map[string]any{"session_id": "ses_1"})},
			op: func(c *Client) error {
				_, err := c.Sessions.Create(ctx, SessionCreateParams{AvatarID: "avt_1"})
				return err
			},
			calls: 2, sleeps: []time.Duration{500 * time.Millisecond},
		},
		{
			name:    "speak is retried with an Idempotency-Key",
			replies: []reply{dropReply, jsonReply(202, map[string]any{"utterance_id": "utt_1"})},
			op: func(c *Client) error {
				_, err := c.Sessions.Speak(ctx, "ses_1", "hi")
				return err
			},
			calls: 2, sleeps: []time.Duration{500 * time.Millisecond},
		},
		{
			name:    "4xx not retried",
			replies: []reply{jsonReply(409, apiError("session_ended", "x"))},
			op:      func(c *Client) error { _, err := c.Sessions.Speak(ctx, "a", "x"); return err },
			calls:   1, wantError: ErrConflict,
		},
		{
			name:    "429 rate_limited waits retry-after, for any request",
			replies: []reply{jsonReply(429, apiError("rate_limited", "slow"), "retry-after", "7"), jsonReply(200, map[string]any{"discarded_count": 0})},
			op:      func(c *Client) error { _, err := c.Sessions.Interrupt(ctx, "ses_1"); return err },
			calls:   2, sleeps: []time.Duration{7 * time.Second},
		},
		{
			name:    "429 without retry-after uses backoff",
			replies: []reply{jsonReply(429, apiError("rate_limited", "x")), jsonReply(200, stateBody)},
			op:      func(c *Client) error { _, err := c.Sessions.Get(ctx, "a"); return err },
			calls:   2, sleeps: []time.Duration{500 * time.Millisecond},
		},
		{
			name:    "429 at exactly one minute is retried",
			replies: []reply{jsonReply(429, map[string]any{}, "retry-after", "60"), jsonReply(200, stateBody)},
			op:      func(c *Client) error { _, err := c.Sessions.Get(ctx, "a"); return err },
			calls:   2, sleeps: []time.Duration{time.Minute},
		},
		{
			name:    "429 longer than a minute returned at once",
			replies: []reply{jsonReply(429, apiError("rate_limited", "x"), "retry-after", "61")},
			op:      func(c *Client) error { _, err := c.Sessions.Get(ctx, "a"); return err },
			calls:   1, wantError: ErrRateLimit,
		},
		{
			name:    "429 concurrency_limit returned at once",
			replies: []reply{jsonReply(429, apiError("concurrency_limit", "x"))},
			op: func(c *Client) error {
				_, err := c.Sessions.Create(ctx, SessionCreateParams{AvatarID: "a"})
				return err
			},
			calls: 1, wantError: ErrRateLimit,
		},
		{
			name:    "429 queue_full returned at once",
			replies: []reply{jsonReply(429, apiError("queue_full", "x"))},
			op:      func(c *Client) error { _, err := c.Sessions.Speak(ctx, "a", "x"); return err },
			calls:   1, wantError: ErrRateLimit,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, tc.replies...)
			c, s := testClient(t, f, tc.opts...)
			err := tc.op(c)
			if tc.wantError == nil && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != nil && !errors.Is(err, tc.wantError) {
				t.Fatalf("got %v, want %v", err, tc.wantError)
			}
			if n := len(f.Calls()); n != tc.calls {
				t.Fatalf("calls = %d, want %d", n, tc.calls)
			}
			if got := s.All(); !reflect.DeepEqual(got, tc.sleeps) && !(len(got) == 0 && len(tc.sleeps) == 0) {
				t.Fatalf("sleeps = %v, want %v", got, tc.sleeps)
			}
		})
	}

	t.Run("retries keep the same Idempotency-Key", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(500, map[string]any{}), jsonReply(201, map[string]any{"session_id": "ses_1"}))
		c, _ := testClient(t, f)
		if _, err := c.Sessions.Create(ctx, SessionCreateParams{AvatarID: "a"}); err != nil {
			t.Fatal(err)
		}
		k := f.Calls()[0].Header.Get("Idempotency-Key")
		if !regexp.MustCompile(`^avacast-go-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(k) || f.Calls()[1].Header.Get("Idempotency-Key") != k {
			t.Fatal(k)
		}
	})

	t.Run("429 returned with RetryAfter", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(429, apiError("rate_limited", "x"), "retry-after", "1"))
		c, _ := testClient(t, f, WithMaxRetries(1))
		_, err := c.Sessions.Get(ctx, "a")
		var e *Error
		errors.As(err, &e)
		if e.Kind != KindRateLimit || e.Code != "rate_limited" || e.RetryAfter == nil || *e.RetryAfter != time.Second || len(f.Calls()) != 2 {
			t.Fatalf("%+v", e)
		}
	})

	t.Run("interrupt is not retried on 5xx, connection errors or timeouts", func(t *testing.T) {
		for _, rep := range []reply{jsonReply(500, map[string]any{}), dropReply, hangReply} {
			f := newFakeAPI(t, rep)
			c, _ := testClient(t, f, WithTimeout(50*time.Millisecond))
			_, err := c.Sessions.Interrupt(ctx, "ses_1")
			var e *Error
			if !errors.As(err, &e) || len(f.Calls()) != 1 {
				t.Fatal(err, len(f.Calls()))
			}
		}
	})
}

func TestTimeoutAndCancel(t *testing.T) {
	t.Run("each attempt times out; safe requests are retried", func(t *testing.T) {
		f := newFakeAPI(t, hangReply)
		c, _ := testClient(t, f, WithTimeout(50*time.Millisecond), WithMaxRetries(1))
		_, err := c.Sessions.Get(context.Background(), "a")
		var e *Error
		errors.As(err, &e)
		if e.Kind != KindTimeout || !errors.Is(err, ErrTimeout) || !errors.Is(err, ErrConnection) || e.Message != "Request timed out after 50ms" {
			t.Fatalf("%+v", e)
		}
		if len(f.Calls()) != 2 {
			t.Fatal(len(f.Calls()))
		}
	})

	t.Run("timeout per request", func(t *testing.T) {
		f := newFakeAPI(t, hangReply)
		c, _ := testClient(t, f, WithTimeout(time.Minute))
		_, err := c.Sessions.Interrupt(context.Background(), "a", WithRequestTimeout(20*time.Millisecond))
		if !errors.Is(err, ErrTimeout) || len(f.Calls()) != 1 {
			t.Fatal(err)
		}
	})

	t.Run("http.Client timeout counts as a timeout", func(t *testing.T) {
		f := newFakeAPI(t, hangReply)
		c, _ := testClient(t, f, WithHTTPClient(&http.Client{Timeout: 20 * time.Millisecond}), WithMaxRetries(0))
		_, err := c.Sessions.Get(context.Background(), "a")
		if !errors.Is(err, ErrTimeout) {
			t.Fatal(err)
		}
	})

	t.Run("a cancelled context stops the request and is not retried", func(t *testing.T) {
		f := newFakeAPI(t, hangReply)
		c, _ := testClient(t, f)
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(30*time.Millisecond, cancel)
		_, err := c.Sessions.Get(ctx, "a")
		var e *Error
		errors.As(err, &e)
		if e.Kind != KindConnection || e.Message != "Request was aborted" || len(f.Calls()) != 1 {
			t.Fatalf("%+v", e)
		}
	})

	t.Run("a context cancelled before the call sends nothing", func(t *testing.T) {
		f := newFakeAPI(t, hangReply)
		c, _ := testClient(t, f)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := c.Sessions.Get(ctx, "a")
		if !errors.Is(err, ErrConnection) || !errors.Is(err, context.Canceled) || len(f.Calls()) != 0 {
			t.Fatal(err)
		}
	})

	t.Run("cancelling during the wait between retries", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(503, map[string]any{}))
		c, _ := testClient(t, f)
		ctx, cancel := context.WithCancel(context.Background())
		c.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
		_, err := c.Sessions.Get(ctx, "a")
		if !errors.Is(err, ErrConnection) || !errors.Is(err, context.Canceled) || len(f.Calls()) != 1 {
			t.Fatal(err)
		}
	})

	t.Run("real sleep honours the context", func(t *testing.T) {
		if err := sleepContext(context.Background(), time.Millisecond); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := sleepContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}

func TestHelpers(t *testing.T) {
	half := func() float64 { return 0.5 }
	var got []time.Duration
	for _, n := range []int{0, 1, 2, 3, 4, 5, 10} {
		got = append(got, backoff(n, half))
	}
	ms := time.Millisecond
	if !reflect.DeepEqual(got, []time.Duration{500 * ms, 1000 * ms, 2000 * ms, 4000 * ms, 8000 * ms, 8000 * ms, 8000 * ms}) {
		t.Fatal(got)
	}
	if backoff(0, func() float64 { return 0 }) != 375*ms || backoff(0, func() float64 { return 1 }) != 625*ms {
		t.Fatal("jitter")
	}

	at := func(sec float64) time.Time { return time.Unix(0, int64(sec*1e9)) }
	cases := []struct {
		v    string
		now  float64
		want time.Duration
		ok   bool
	}{
		{"2", 0, 2 * time.Second, true},
		{"0.4", 0, 400 * ms, true},
		{"Thu, 01 Jan 1970 00:00:05 GMT", 1, 4 * time.Second, true},
		{"Thu, 01 Jan 1970 00:00:05 GMT", 9, 0, true},
		{"soon", 0, 0, false},
		{"-3", 0, 0, true},
		{"NaN", 0, 0, false},
		{" ", 0, 0, false},
		{"", 0, 0, false},
	}
	for i, tc := range cases {
		d, ok := parseRetryAfter(tc.v, at(tc.now))
		if d != tc.want || ok != tc.ok {
			t.Errorf("case %d: got %v %v, want %v %v", i, d, ok, tc.want, tc.ok)
		}
	}
	if MaxRetryAfter != time.Minute {
		t.Fatal(MaxRetryAfter)
	}
	if *Float64(0.9) != 0.9 || *Int(3) != 3 {
		t.Fatal("pointer helpers")
	}
	e := &Error{Kind: KindRateLimit, Message: "slow"}
	if e.Error() != "slow" || e.Unwrap() != nil || e.RetryAfter != nil {
		t.Fatal(e)
	}
	if a, b := newIdempotencyKey(), newIdempotencyKey(); a == b || !strings.HasPrefix(a, "avacast-go-") {
		t.Fatal(a, b)
	}
}
