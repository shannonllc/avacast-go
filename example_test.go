package avacast_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	avacast "github.com/shannonllc/avacast-go"
)

func Example() {
	client, err := avacast.NewClient("sk_live_...") // or set AVACAST_API_KEY
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
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
	// Give only these to the browser. Never ship the API key to a browser.
	fmt.Println(session.ClientToken, session.WebRTC.SignalingURL)
}

func ExampleSessionsService_Speak() {
	client, _ := avacast.NewClient("sk_live_...")
	ctx := context.Background()
	_, err := client.Sessions.Speak(ctx, "ses_...", "ご相談ありがとうございます。")
	var e *avacast.Error
	switch {
	case errors.As(err, &e) && e.Code == "session_not_connected":
		fmt.Println("wait for session.ready in the browser")
	case errors.As(err, &e) && e.Code == "queue_full":
		fmt.Println("wait for speak.ended, or interrupt")
	case err != nil:
		log.Fatal(err)
	}
}

func ExampleUsageService_Range() {
	client, _ := avacast.NewClient("sk_live_...")
	u, err := client.Usage.Range(context.Background(), avacast.UsageRangeParams{
		From:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		GroupBy: "metadata.tenant",
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, g := range u.Data {
		fmt.Println(g.Key, g.ConnectedSeconds)
	}
}
