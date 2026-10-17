# Changelog

## 0.1.0 (2026-10-17)

- First release. `avacast.NewClient(apiKey)` calls the REST API (`/v1`) with your API key (`sk_live_…` / `sk_test_…`, or `AVACAST_API_KEY`).
- `Sessions.Create`, `Sessions.Get`, `Sessions.End`, `Sessions.Speak`, `Sessions.Interrupt`, `Avatars.List`, `Usage.Get` (this month), `Usage.Range` (`From` / `To` / `GroupBy: "metadata.<key>"`) and `Client.Status`.
- Retries 429 `rate_limited`, and 5xx and connection errors for requests that are safe to repeat. `Sessions.Create` and `Sessions.Speak` send an `Idempotency-Key` (generated, or `WithIdempotencyKey`), so they are retried without opening two sessions or speaking twice. `Sessions.Interrupt` is not retried on a 5xx or a connection error.
- `*avacast.Error` with `Kind`, `Code` (the API's `error.code`), `Status`, `RequestID` and `RetryAfter`, and sentinels for `errors.Is`.
- `VoiceIDs` and `Languages`: the values for `SessionVoice.VoiceID` and `SessionVoice.Language`.
- Standard library only. Go 1.21+.
