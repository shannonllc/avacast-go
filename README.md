# avacast-go

Go SDK for [avacast](https://avacast.jp/) realtime avatars: create sessions, make the avatar speak and read usage from your server.

- Standard library only.
- Go 1.21+. Every call takes a `context.Context`.
- Server-side only. Show the avatar in the browser with the browser SDK (`avacast/web` from the [Node.js SDK](https://www.npmjs.com/package/avacast), or `https://avacast.jp/sdk/v1.js`), using the session's `client_token` and `webrtc` values.

## Install

```sh
go get github.com/shannonllc/avacast-go
```

```go
import avacast "github.com/shannonllc/avacast-go"
```

## Quick start

```go
client, err := avacast.NewClient("sk_live_...") // or set AVACAST_API_KEY
if err != nil {
	log.Fatal(err)
}

avatars, err := client.Avatars.List(ctx)
if err != nil {
	log.Fatal(err)
}
session, err := client.Sessions.Create(ctx, avacast.SessionCreateParams{
	AvatarID: avatars.Data[0].ID,
	Voice:    &avacast.SessionVoice{Language: "ja", VoiceID: "female-30s-clear"},
})
if err != nil {
	log.Fatal(err)
}
// Give only session.ClientToken and session.WebRTC to the browser. Never ship the API key to a browser.
```

After the browser has connected (`session.ready`):

```go
_, err = client.Sessions.Speak(ctx, session.SessionID, "ご相談ありがとうございます。")
_, err = client.Sessions.Speak(ctx, session.SessionID, "ご用件をお聞かせください。") // no need to wait: lines are queued

_, err = client.Sessions.Interrupt(ctx, session.SessionID) // the user started talking
_, err = client.Sessions.End(ctx, session.SessionID)       // billing stops when the session closes
```

## Methods

| Method | API |
|---|---|
| `Sessions.Create(ctx, SessionCreateParams{AvatarID, Voice, IdleTimeoutSeconds, Metadata})` | `POST /v1/sessions` |
| `Sessions.Speak(ctx, sessionID, text)` | `POST /v1/sessions/:id/speak` |
| `Sessions.Interrupt(ctx, sessionID)` | `POST /v1/sessions/:id/interrupt` |
| `Sessions.Get(ctx, sessionID)` | `GET /v1/sessions/:id` |
| `Sessions.End(ctx, sessionID)` | `DELETE /v1/sessions/:id` |
| `Avatars.List(ctx)` | `GET /v1/avatars` |
| `Usage.Get(ctx)` | `GET /v1/usage` (this month) |
| `Usage.Range(ctx, UsageRangeParams{From, To, GroupBy})` | `GET /v1/usage?from=&to=&group_by=` |
| `Status(ctx)` | `GET /v1/status` |

Options for one call: `avacast.WithRequestTimeout(d)`, `avacast.WithRequestMaxRetries(n)`, and `avacast.WithIdempotencyKey(key)` for `Create` and `Speak`.

`avacast.VoiceIDs` (`female-standard`, `female-30s-clear`, … `male-60s-deep`) and `avacast.Languages` (`ja`, `en`, …) are the values for `SessionVoice`. Without `VoiceID`, the session uses the avatar's voice.

```go
client, err := avacast.NewClient(os.Getenv("AVACAST_API_KEY"),
	avacast.WithTimeout(30*time.Second), // per attempt (default)
	avacast.WithMaxRetries(2),           // retries after the first attempt (default)
)
```

## Usage

```go
u, err := client.Usage.Range(ctx, avacast.UsageRangeParams{
	From:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	To:      time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	GroupBy: "metadata.tenant", // sessions created with Metadata: map[string]any{"tenant": "…"}
})
for _, g := range u.Data {
	fmt.Println(g.Key, g.ConnectedSeconds)
}
```

You are billed for the time a session is open (`ConnectedSeconds`).

## Errors

Every error is an `*avacast.Error`. Branch on `Code` (the API's `error.code`), or use `errors.Is` with a sentinel. Messages may change.

```go
_, err := client.Sessions.Speak(ctx, id, text)
var e *avacast.Error
switch {
case errors.As(err, &e) && e.Code == "session_not_connected":
	// wait for session.ready in the browser
case errors.As(err, &e) && e.Code == "queue_full":
	// wait for speak.ended, or interrupt
case errors.Is(err, avacast.ErrPaymentRequired):
	// the monthly limit was reached
case err != nil:
	log.Fatal(err)
}
```

| Kind | Sentinel | Status | `Code` |
|---|---|---|---|
| `KindValidation` | `ErrValidation` | 400 | `invalid_request` |
| `KindAuthentication` | `ErrAuthentication` | 401 | `invalid_api_key` |
| `KindPaymentRequired` | `ErrPaymentRequired` | 402 | `payment_required` |
| `KindNotFound` | `ErrNotFound` | 404 | `session_not_found`, `avatar_not_found`, `not_found` |
| `KindConflict` | `ErrConflict` | 409 | `session_ended`, `session_not_connected`, `idempotency_conflict` |
| `KindRateLimit` | `ErrRateLimit` | 429 | `rate_limited`, `concurrency_limit`, `queue_full` |
| `KindAPI` | `ErrAPI` | 5xx and others | `gpu_unavailable` (no capacity right now), `maintenance` |
| `KindConnection` | `ErrConnection` | – | Could not reach the API, or cancelled |
| `KindTimeout` | `ErrTimeout` (also `ErrConnection`) | – | No response within the timeout |

`RequestID` is the call's ID; quote it when you contact us.

## Retries

- 429 `rate_limited` is retried after `retry-after`. `concurrency_limit` and `queue_full` are returned at once: close a session, or wait for the queue.
- 5xx, timeouts and connection errors are retried for `Get`, `End`, `Avatars.List`, `Usage` and `Status`, and for `Create` and `Speak`, which send an `Idempotency-Key` so that a retry does not open a second session or speak twice. `Interrupt` is not retried on these.
- Waits are exponential (0.5 s, 1 s, 2 s … up to 8 s, ±25%), or `retry-after` when the API sends it. A `retry-after` over 60 seconds is returned at once.

## Links

- Docs: https://avacast.jp/docs
- License: MIT
