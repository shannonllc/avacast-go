package avacast

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Kind classifies an [Error].
type Kind string

// Error kinds. Match them with errors.Is and the Err… sentinels, or read [Error.Kind]. Branch on [Error.Code] for
// the API's error code.
const (
	// KindValidation is 400 invalid_request: a parameter is missing or invalid.
	KindValidation Kind = "validation"
	// KindAuthentication is 401 invalid_api_key: the API key is wrong or revoked, or the account is not active.
	KindAuthentication Kind = "authentication"
	// KindPaymentRequired is 402 payment_required: the monthly limit was reached. New sessions are refused.
	KindPaymentRequired Kind = "payment_required"
	// KindPermission is 403: the operation is not allowed.
	KindPermission Kind = "permission"
	// KindNotFound is 404 session_not_found, avatar_not_found or not_found.
	KindNotFound Kind = "not_found"
	// KindConflict is 409 session_ended, session_not_connected or idempotency_conflict.
	KindConflict Kind = "conflict"
	// KindRateLimit is 429 rate_limited, concurrency_limit or queue_full, returned after retries are used up.
	KindRateLimit Kind = "rate_limit"
	// KindAPI is any other error response, including 5xx (gpu_unavailable and maintenance are 503).
	KindAPI Kind = "api"
	// KindConnection means the request could not reach the API (DNS, TCP, TLS) or was cancelled.
	KindConnection Kind = "connection"
	// KindTimeout means the request did not complete within the timeout. It also matches ErrConnection.
	KindTimeout Kind = "timeout"
	// KindInvalidArgument is a usage error, such as a missing API key.
	KindInvalidArgument Kind = "invalid_argument"
)

// Sentinels for errors.Is.
var (
	ErrValidation      = errors.New("avacast: validation error")
	ErrAuthentication  = errors.New("avacast: authentication error")
	ErrPaymentRequired = errors.New("avacast: payment required")
	ErrPermission      = errors.New("avacast: permission error")
	ErrNotFound        = errors.New("avacast: not found")
	ErrConflict        = errors.New("avacast: conflict")
	ErrRateLimit       = errors.New("avacast: rate limited")
	ErrAPI             = errors.New("avacast: api error")
	ErrConnection      = errors.New("avacast: connection error")
	ErrTimeout         = errors.New("avacast: timeout")
	ErrInvalidArgument = errors.New("avacast: invalid argument")
)

var sentinels = map[Kind]error{
	KindValidation:      ErrValidation,
	KindAuthentication:  ErrAuthentication,
	KindPaymentRequired: ErrPaymentRequired,
	KindPermission:      ErrPermission,
	KindNotFound:        ErrNotFound,
	KindConflict:        ErrConflict,
	KindRateLimit:       ErrRateLimit,
	KindAPI:             ErrAPI,
	KindConnection:      ErrConnection,
	KindTimeout:         ErrTimeout,
	KindInvalidArgument: ErrInvalidArgument,
}

// Error is every error returned by the SDK. Use errors.As to read it, or errors.Is with a sentinel:
//
//	var e *avacast.Error
//	if errors.As(err, &e) && e.Code == "session_not_connected" { … }
//	if errors.Is(err, avacast.ErrPaymentRequired) { … }
type Error struct {
	Kind Kind
	// Message from the API, or a description of the failure. It may change: branch on Code.
	Message string
	// Status is the HTTP status. 0 when no response was received.
	Status int
	// Code is the API error code, such as "invalid_request", "session_not_connected" or "queue_full". "" when absent.
	Code string
	// RequestID (x-request-id) to quote when contacting support.
	RequestID string
	// RetryAfter is the wait from retry-after (on a 429, or a 503 maintenance), or nil.
	RetryAfter *time.Duration
	// Err is the underlying cause, if any.
	Err error
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Unwrap() error { return e.Err }

// Is reports whether target is the sentinel for e's kind. A timeout also matches ErrConnection.
func (e *Error) Is(target error) bool {
	if target == sentinels[e.Kind] {
		return true
	}
	return e.Kind == KindTimeout && target == ErrConnection
}

func newError(kind Kind, message string, cause error) *Error {
	return &Error{Kind: kind, Message: message, Err: cause}
}

// parseRetryAfter returns the wait from retry-after (seconds or HTTP date).
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	if strings.TrimSpace(value) == "" {
		return 0, false
	}
	if n, err := strconv.ParseFloat(value, 64); err == nil {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return time.Duration(math.Max(0, n) * float64(time.Second)), true
	}
	at, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	d := at.Sub(now)
	if d < 0 {
		d = 0
	}
	return d, true
}

func requestIDOf(h http.Header) string {
	if v := h.Get("x-request-id"); v != "" {
		return v
	}
	return h.Get("cf-ray")
}

// jsonString decodes raw when it is a JSON string (not null, a number or an object).
func jsonString(raw json.RawMessage) (string, bool) {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// readErrorBody reads {"error":{"code","message"}}; it also accepts {"error":"code","message":"…"}.
func readErrorBody(body []byte) (code, message string) {
	var top map[string]json.RawMessage
	_ = json.Unmarshal(body, &top) // HTML from a proxy, empty body, etc. leave it nil
	raw := top["error"]
	var nested map[string]json.RawMessage
	if len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &nested) == nil {
		code, _ = jsonString(nested["code"])
		message, _ = jsonString(nested["message"])
		return code, message
	}
	code, _ = jsonString(raw)
	message, _ = jsonString(top["message"])
	return code, message
}

// errorFromResponse builds the error for a non-2xx response.
func errorFromResponse(status int, body []byte, h http.Header, now time.Time) *Error {
	code, message := readErrorBody(body)
	if message == "" {
		message = fmt.Sprintf("HTTP %d", status)
	}
	e := &Error{Status: status, Code: code, Message: message, RequestID: requestIDOf(h)}
	if d, ok := parseRetryAfter(h.Get("retry-after"), now); ok {
		e.RetryAfter = &d
	}
	switch status {
	case 400, 422:
		e.Kind = KindValidation
	case 401:
		e.Kind = KindAuthentication
	case 402:
		e.Kind = KindPaymentRequired
	case 403:
		e.Kind = KindPermission
	case 404:
		e.Kind = KindNotFound
	case 409:
		e.Kind = KindConflict
	case 429:
		e.Kind = KindRateLimit
	default:
		e.Kind = KindAPI
	}
	return e
}
