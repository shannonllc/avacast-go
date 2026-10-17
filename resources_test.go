package avacast

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"
	"time"
)

func TestSessions(t *testing.T) {
	ctx := context.Background()

	t.Run("create sends the params and an Idempotency-Key", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(201, map[string]any{
			"session_id": "ses_1", "client_token": "a.b.c", "expires_at": "2026-10-17T12:30:00.000Z",
			"webrtc": map[string]any{"signaling_url": "https://n/offer", "events_url": "https://n/events", "ice_servers": []any{map[string]any{"urls": "turn:t", "username": "u", "credential": "c"}}},
		}))
		c, _ := testClient(t, f)
		s, err := c.Sessions.Create(ctx, SessionCreateParams{
			AvatarID:           "avt_1",
			Voice:              &SessionVoice{Language: "ja", VoiceID: "female-30s-clear", Speed: Float64(0.9)},
			IdleTimeoutSeconds: Int(300),
			Metadata:           map[string]any{"tenant": "clinic-a"},
		})
		if err != nil || s.ClientToken != "a.b.c" || s.WebRTC.SignalingURL != "https://n/offer" || s.WebRTC.IceServers[0].Username != "u" || s.ExpiresAt == "" {
			t.Fatal(s, err)
		}
		l := f.Last()
		if l.Method != "POST" || l.Path != "/v1/sessions" || string(l.Raw) != `{"avatar_id":"avt_1","voice":{"language":"ja","voice_id":"female-30s-clear","speed":0.9},"idle_timeout_seconds":300,"metadata":{"tenant":"clinic-a"}}` {
			t.Fatal(l.Method, l.Path, string(l.Raw))
		}
		if !regexp.MustCompile(`^avacast-go-`).MatchString(l.Header.Get("Idempotency-Key")) {
			t.Fatal(l.Header)
		}
	})

	t.Run("minimal create, and a given Idempotency-Key", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(201, map[string]any{"session_id": "ses_1"}))
		c, _ := testClient(t, f)
		if _, err := c.Sessions.Create(ctx, SessionCreateParams{AvatarID: "avt_1"}, WithIdempotencyKey("order-1")); err != nil {
			t.Fatal(err)
		}
		if string(f.Last().Raw) != `{"avatar_id":"avt_1"}` || f.Last().Header.Get("Idempotency-Key") != "order-1" {
			t.Fatal(string(f.Last().Raw), f.Last().Header)
		}
		if _, err := c.Sessions.Speak(ctx, "ses_1", "x", WithIdempotencyKey("line-1")); err != nil || f.Last().Header.Get("Idempotency-Key") != "line-1" {
			t.Fatal(err)
		}
	})

	t.Run("get, end, speak and interrupt", func(t *testing.T) {
		f := newFakeAPI(t,
			jsonReply(200, map[string]any{"session_id": "ses/1", "status": "ended", "end_reason": "gpu_failure", "connected_seconds": 312, "ended_at": "2026-10-17T12:00:00Z"}),
			jsonReply(200, map[string]any{"session_id": "ses/1", "status": "ended", "connected_seconds": 3, "total_speak_ms": 10}),
			jsonReply(202, map[string]any{"utterance_id": "utt_1", "status": "queued", "queue_length": 1}),
			jsonReply(200, map[string]any{"interrupted_utterance_id": "utt_1", "discarded_count": 2}),
		)
		c, _ := testClient(t, f)
		st, err := c.Sessions.Get(ctx, "ses/1")
		if err != nil || *st.EndReason != "gpu_failure" || st.ConnectedSeconds != 312 || *st.EndedAt == "" {
			t.Fatal(st, err)
		}
		e, err := c.Sessions.End(ctx, "ses/1")
		if err != nil || e.ConnectedSeconds != 3 || e.TotalSpeakMs != 10 {
			t.Fatal(e, err)
		}
		u, err := c.Sessions.Speak(ctx, "ses/1", "hi")
		if err != nil || u.Status != "queued" {
			t.Fatal(u, err)
		}
		i, err := c.Sessions.Interrupt(ctx, "ses/1")
		if err != nil || *i.InterruptedUtteranceID != "utt_1" || i.DiscardedCount != 2 {
			t.Fatal(i, err)
		}
		var got []string
		for _, call := range f.Calls() {
			got = append(got, call.Method+" "+call.Path)
		}
		want := []string{"GET /v1/sessions/ses%2F1", "DELETE /v1/sessions/ses%2F1", "POST /v1/sessions/ses%2F1/speak", "POST /v1/sessions/ses%2F1/interrupt"}
		if !reflect.DeepEqual(got, want) {
			t.Fatal(got)
		}
		if f.Calls()[3].Header.Get("Idempotency-Key") != "" || len(f.Calls()[3].Raw) != 0 {
			t.Fatal("interrupt sent a key or a body")
		}
	})

	t.Run("errors are returned as nil and the error", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(404, apiError("session_not_found", "x")))
		c, _ := testClient(t, f)
		if s, err := c.Sessions.End(ctx, "a"); s != nil || !errors.Is(err, ErrNotFound) {
			t.Fatal(s, err)
		}
		if s, err := c.Sessions.Get(ctx, "a"); s != nil || !errors.Is(err, ErrNotFound) {
			t.Fatal(s, err)
		}
	})
}

func TestAvatarsUsageStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("avatars", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(200, map[string]any{"data": []any{map[string]any{"id": "avt_1", "name": "案内役A", "thumbnail_url": nil, "languages": []any{"ja"}, "is_preset": true}}}))
		c, _ := testClient(t, f)
		l, err := c.Avatars.List(ctx)
		if err != nil || len(l.Data) != 1 || l.Data[0].Name != "案内役A" || !l.Data[0].IsPreset || l.Data[0].ThumbnailURL != nil || f.Last().Path != "/v1/avatars" {
			t.Fatal(l, err)
		}
	})

	t.Run("usage this month", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(200, map[string]any{"connected_seconds": 10, "limits": map[string]any{"concurrency": 10, "monthly_connected_seconds": nil}}))
		c, _ := testClient(t, f)
		u, err := c.Usage.Get(ctx)
		if err != nil || u.ConnectedSeconds != 10 || u.Limits.Concurrency != 10 || u.Limits.MonthlyConnectedSeconds != nil || f.Last().Query != "" {
			t.Fatal(u, err)
		}
	})

	t.Run("usage range", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(200, map[string]any{"connected_seconds": 5, "group_by": "metadata.tenant", "data": []any{map[string]any{"key": nil, "connected_seconds": 5, "session_count": 1}}}))
		c, _ := testClient(t, f)
		jst := time.FixedZone("JST", 9*3600)
		u, err := c.Usage.Range(ctx, UsageRangeParams{From: time.Date(2026, 10, 1, 9, 0, 0, 0, jst), To: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), GroupBy: "metadata.tenant"})
		if err != nil || u.GroupBy != "metadata.tenant" || u.Data[0].Key != nil {
			t.Fatal(u, err)
		}
		if q := f.Last().Query; q != "from=2026-10-01T00%3A00%3A00Z&group_by=metadata.tenant&to=2026-11-01T00%3A00%3A00Z" {
			t.Fatal(q)
		}
		if _, err := c.Usage.Range(ctx, UsageRangeParams{GroupBy: "metadata.x"}); err != nil || f.Last().Query != "group_by=metadata.x" {
			t.Fatal(err, f.Last().Query)
		}
		n := len(f.Calls())
		if u, err := c.Usage.Range(ctx, UsageRangeParams{}); u != nil || !errors.Is(err, ErrInvalidArgument) || len(f.Calls()) != n {
			t.Fatal(u, err)
		}
	})

	t.Run("status", func(t *testing.T) {
		f := newFakeAPI(t, jsonReply(200, map[string]any{"ok": true}))
		c, _ := testClient(t, f)
		s, err := c.Status(ctx)
		if err != nil || !s.OK || f.Last().Path != "/v1/status" {
			t.Fatal(s, err)
		}
	})

	t.Run("voice IDs and languages", func(t *testing.T) {
		re := regexp.MustCompile(`^(female|male)-(standard|[2-6]0s-[a-z]+)$`)
		for _, v := range VoiceIDs {
			if !re.MatchString(v) {
				t.Fatal(v)
			}
		}
		if len(VoiceIDs) != 14 || Languages[0] != "ja" {
			t.Fatal(VoiceIDs, Languages)
		}
	})
}
