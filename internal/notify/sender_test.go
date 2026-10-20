package notify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/store"
)

func TestDemoSenderSkipsNetwork(t *testing.T) {
	sender := &Sender{Demo: true}
	status, err := sender.Send(context.Background(), store.Channel{
		Type:             "email",
		ConfigCiphertext: "not-a-valid-ciphertext",
	}, alert.Payload{}, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if status != 202 {
		t.Fatalf("status = %d", status)
	}
}

func TestDiscordPayloadOmitsEmptyOverrides(t *testing.T) {
	body, err := json.Marshal(buildDiscordPayload(DiscordConfig{}, alert.Payload{Event: "test"}))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"username", "avatar_url"} {
		if _, ok := raw[key]; ok {
			t.Errorf("%s should be omitted when empty", key)
		}
	}
	if _, ok := raw["embeds"]; !ok {
		t.Error("embeds should be present")
	}
}

func TestDiscordPayloadIncludesOverrides(t *testing.T) {
	body, err := json.Marshal(buildDiscordPayload(DiscordConfig{
		Username:  "IWMMD",
		AvatarURL: "https://example.com/avatar.png",
	}, alert.Payload{Event: "test"}))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"username", "avatar_url"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("%s should be present when set", key)
		}
	}
}

func TestDiscordPayloadBuildsRichEmbed(t *testing.T) {
	payload := alert.Payload{
		Event:   "test",
		Message: "This is a test notification from IWMMD.",
		URL:     "https://iwmmd.example/domains/123",
	}
	body := buildDiscordPayload(DiscordConfig{}, payload)
	if len(body.Embeds) != 1 {
		t.Fatalf("embeds = %d", len(body.Embeds))
	}
	embed := body.Embeds[0]
	if embed.Author == nil || embed.Author.Name != "IWMMD" {
		t.Fatalf("author = %+v", embed.Author)
	}
	if embed.Title != "Test notification" {
		t.Errorf("title = %q", embed.Title)
	}
	if embed.Description != payload.Message {
		t.Errorf("description = %q", embed.Description)
	}
	if embed.URL != payload.URL {
		t.Errorf("url = %q", embed.URL)
	}
	if embed.Footer == nil || embed.Footer.Text == "" {
		t.Error("footer is missing")
	}
	if _, err := time.Parse(time.RFC3339, embed.Timestamp); err != nil {
		t.Errorf("timestamp = %q: %v", embed.Timestamp, err)
	}
	if len(embed.Fields) != 1 || embed.Fields[0].Name != "Status" || embed.Fields[0].Value != "Delivered" {
		t.Errorf("fields = %+v", embed.Fields)
	}
	if len(body.Components) != 1 || len(body.Components[0].Components) != 1 {
		t.Fatalf("components = %+v", body.Components)
	}
	button := body.Components[0].Components[0]
	if button.Style != 5 || button.URL != payload.URL || button.Label != "Open IWMMD" {
		t.Errorf("button = %+v", button)
	}
}

func TestDiscordPayloadBuildsExpiryEmbed(t *testing.T) {
	payload := alert.Payload{
		Event:    "expiry",
		Domain:   "example.com",
		Expiry:   "2026-11-20",
		DaysLeft: 30,
		Rule:     "30 days before",
		URL:      "https://iwmmd.example/domains/123",
	}
	body := buildDiscordPayload(DiscordConfig{}, payload)
	if len(body.Embeds) != 1 {
		t.Fatalf("embeds = %d", len(body.Embeds))
	}
	embed := body.Embeds[0]
	if embed.Title != "example.com expires in 30 days" {
		t.Errorf("title = %q", embed.Title)
	}
	if len(embed.Fields) != 2 {
		t.Fatalf("fields = %+v", embed.Fields)
	}
	if embed.Fields[0].Name != "Expiry date" || embed.Fields[0].Value != "Nov 20, 2026" {
		t.Errorf("expiry field = %+v", embed.Fields[0])
	}
	if embed.Fields[1].Name != "Alert rule" || embed.Fields[1].Value != "30 days before" {
		t.Errorf("rule field = %+v", embed.Fields[1])
	}
	if len(body.Components) != 1 || body.Components[0].Components[0].Label != "Open domain" {
		t.Errorf("components = %+v", body.Components)
	}
}

func TestDiscordPayloadUsesResolvedColor(t *testing.T) {
	body := buildDiscordPayload(DiscordConfig{}, alert.Payload{
		Event:        "system",
		Kind:         "system",
		SystemEvent:  "resolved",
		IncidentKind: "dns",
		Domain:       "example.com",
	})
	if len(body.Embeds) != 1 {
		t.Fatalf("embeds = %d", len(body.Embeds))
	}
	if got, want := body.Embeds[0].Color, discordColor(toneSuccess.accent); got != want {
		t.Errorf("color = %#x, want %#x", got, want)
	}
}

func TestRedactWebhookError(t *testing.T) {
	webhookURL := "https://discord.com/api/webhooks/123/secret-token"
	err := errors.New(`Post "` + webhookURL + `": dial tcp: lookup discord.com: no such host`)
	got := redactWebhookError(err, webhookURL)
	if strings.Contains(got.Error(), "secret-token") {
		t.Errorf("token leaked: %q", got)
	}
	if !strings.Contains(got.Error(), "https://discord.com/api/webhooks/<redacted>") {
		t.Errorf("error = %q", got)
	}
	plain := errors.New("context deadline exceeded")
	if got := redactWebhookError(plain, webhookURL); got != plain {
		t.Errorf("unrelated error changed: %q", got)
	}
}

func TestClassifyHTTPWithBody(t *testing.T) {
	detail := `{"username":["Username cannot be \"\""]}`
	err := classifyHTTPWithBody(400, []byte(detail))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), detail) {
		t.Errorf("error = %q", err)
	}
	var delivery *DeliveryError
	if !errors.As(err, &delivery) {
		t.Fatal("expected DeliveryError")
	}
	if delivery.Retryable {
		t.Error("400 should not be retryable")
	}
	if err := classifyHTTPWithBody(204, nil); err != nil {
		t.Errorf("204 returned %v", err)
	}
}
