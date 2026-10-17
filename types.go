package avacast

// Shapes of /v1 requests and responses (https://avacast.jp/docs). Times are ISO 8601 (UTC) strings.

// VoiceIDs are the values for SessionVoice.VoiceID: <gender>-standard or <gender>-<decade>s-<tone>.
var VoiceIDs = []string{
	"female-standard",
	"female-20s-bright",
	"female-30s-clear",
	"female-30s-soft",
	"female-40s-warm",
	"female-50s-calm",
	"female-60s-gentle",
	"male-standard",
	"male-20s-bright",
	"male-30s-clear",
	"male-30s-friendly",
	"male-40s-calm",
	"male-50s-calm",
	"male-60s-deep",
}

// Languages are the values for SessionVoice.Language. Other values are read as "ja".
var Languages = []string{"ja", "en", "zh", "th", "es", "vi", "pt", "it", "sk"}

// SessionVoice is the voice of a session. Every field is optional.
type SessionVoice struct {
	// Language of speech. Default "ja".
	Language string `json:"language,omitempty"`
	// VoiceID such as "female-30s-clear" (see VoiceIDs). Default: the avatar's voice.
	VoiceID string `json:"voice_id,omitempty"`
	// Speed is the speaking rate, 0.8 to 1.2. Default 1.0. Values out of range are clamped.
	Speed *float64 `json:"speed,omitempty"`
}

// SessionCreateParams is the body of POST /v1/sessions.
type SessionCreateParams struct {
	// AvatarID is the avatar to use (avt_…). Required.
	AvatarID string        `json:"avatar_id"`
	Voice    *SessionVoice `json:"voice,omitempty"`
	// IdleTimeoutSeconds without speaking before the session closes, 30 to 1800. Default 180.
	IdleTimeoutSeconds *int `json:"idle_timeout_seconds,omitempty"`
	// Metadata is your own labels, for example to split usage by metadata.<key>.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// IceServer is a STUN or TURN server for the browser. URLs is a string or a list of strings.
type IceServer struct {
	URLs       any    `json:"urls"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}

// SessionWebRTC is what the browser needs to connect.
type SessionWebRTC struct {
	SignalingURL string `json:"signaling_url"`
	// EventsURL is the speech events stream. The browser SDK opens it for you.
	EventsURL  string      `json:"events_url"`
	IceServers []IceServer `json:"ice_servers"`
}

// Session is the response of POST /v1/sessions. Pass only ClientToken and WebRTC to the browser, never the API key.
type Session struct {
	SessionID   string        `json:"session_id"`
	ClientToken string        `json:"client_token"`
	WebRTC      SessionWebRTC `json:"webrtc"`
	// ExpiresAt is when the session closes at the latest (the plan's longest session).
	ExpiresAt string `json:"expires_at"`
}

// SessionState is the response of GET /v1/sessions/:id.
type SessionState struct {
	SessionID string `json:"session_id"`
	// Status is "active" or "ended".
	Status string `json:"status"`
	// EndReason is "closed_by_api", "expired", "idle", "disconnected" or "gpu_failure" (a failure on avacast's
	// side), or nil while active.
	EndReason *string `json:"end_reason"`
	AvatarID  string  `json:"avatar_id"`
	Language  string  `json:"language"`
	// QueueLength is the lines waiting or being spoken.
	QueueLength int `json:"queue_length"`
	// ConnectedSeconds is what you are billed for. It counts up to now while the session is open.
	ConnectedSeconds int `json:"connected_seconds"`
	// TotalSpeakMs is the total speaking time (for reference, not billed).
	TotalSpeakMs int     `json:"total_speak_ms"`
	StartedAt    string  `json:"started_at"`
	EndedAt      *string `json:"ended_at"`
	ExpiresAt    string  `json:"expires_at"`
}

// EndedSession is the response of DELETE /v1/sessions/:id.
type EndedSession struct {
	SessionID        string `json:"session_id"`
	Status           string `json:"status"`
	ConnectedSeconds int    `json:"connected_seconds"`
	TotalSpeakMs     int    `json:"total_speak_ms"`
}

// Utterance is the response of POST /v1/sessions/:id/speak.
type Utterance struct {
	UtteranceID string `json:"utterance_id"`
	// Status is "queued".
	Status      string `json:"status"`
	QueueLength int    `json:"queue_length"`
}

// InterruptResult is the response of POST /v1/sessions/:id/interrupt.
type InterruptResult struct {
	// InterruptedUtteranceID is the line that was being spoken, or nil when nothing was.
	InterruptedUtteranceID *string `json:"interrupted_utterance_id"`
	// DiscardedCount is the lines removed from the queue.
	DiscardedCount int `json:"discarded_count"`
}

// Avatar is an avatar you can use.
type Avatar struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ThumbnailURL *string  `json:"thumbnail_url"`
	Languages    []string `json:"languages"`
	// IsPreset is true for avatars provided by avacast, false for the ones you made.
	IsPreset bool `json:"is_preset"`
}

// List is a list response ({"data": [...]}).
type List[T any] struct {
	Data []T `json:"data"`
}

// UsagePeriod is the period counted.
type UsagePeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// UsageLimits are the limits of your plan.
type UsageLimits struct {
	Concurrency int `json:"concurrency"`
	// MonthlyConnectedSeconds: when reached, creating a session fails with 402 payment_required. nil for no limit.
	MonthlyConnectedSeconds *int `json:"monthly_connected_seconds"`
}

// MonthlyUsage is GET /v1/usage without parameters: this month (UTC).
type MonthlyUsage struct {
	Period UsagePeriod `json:"period"`
	// ConnectedSeconds is what you are billed for. It includes open sessions.
	ConnectedSeconds  int         `json:"connected_seconds"`
	TotalSpeakSeconds int         `json:"total_speak_seconds"`
	SessionCount      int         `json:"session_count"`
	UtteranceCount    int         `json:"utterance_count"`
	Limits            UsageLimits `json:"limits"`
}

// UsageGroup is one metadata value of RangeUsage.
type UsageGroup struct {
	// Key is the metadata value, or nil for sessions without the key.
	Key              *string `json:"key"`
	ConnectedSeconds int     `json:"connected_seconds"`
	SessionCount     int     `json:"session_count"`
}

// RangeUsage is GET /v1/usage with from, to or group_by. Sessions across the edges count only the part inside.
type RangeUsage struct {
	Period           UsagePeriod `json:"period"`
	ConnectedSeconds int         `json:"connected_seconds"`
	// SessionCount is the sessions that started in the period.
	SessionCount int `json:"session_count"`
	// GroupBy and Data are set with UsageRangeParams.GroupBy (most connected first).
	GroupBy string       `json:"group_by,omitempty"`
	Data    []UsageGroup `json:"data,omitempty"`
}

// Status is the response of GET /v1/status.
type Status struct {
	OK bool `json:"ok"`
}
