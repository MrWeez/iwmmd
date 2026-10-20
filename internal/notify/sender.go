package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/platform/secrets"
	"github.com/mrweez/iwmmd/internal/settings"
	"github.com/mrweez/iwmmd/internal/store"
)

// Sender decrypts channel configs and performs one delivery.
type Sender struct {
	Store    *store.Store
	Settings *settings.Service
	Box      *secrets.Box
	Demo     bool
}

// DecryptChannel returns the typed configuration for a channel.
func (s *Sender) DecryptChannel(channel store.Channel) (any, error) {
	body, err := s.Box.Decrypt(channel.ConfigCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt channel config: %w", err)
	}
	return DecodeConfig(channel.Type, body)
}

// Send performs one delivery and returns an HTTP-like status code.
func (s *Sender) Send(ctx context.Context, channel store.Channel, payload alert.Payload, notificationID string) (int, error) {
	if s.Demo {
		return 202, nil
	}
	cfg, err := s.DecryptChannel(channel)
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	switch channel.Type {
	case "email":
		snap, err := s.Settings.Get(ctx)
		if err != nil {
			return 0, &DeliveryError{Retryable: true, Err: err}
		}
		return sendEmail(ctx, snap.SMTP, cfg.(EmailConfig), payload, notificationID)
	case "webhook":
		return sendWebhook(ctx, cfg.(WebhookConfig), payload, notificationID)
	case "discord":
		return sendDiscord(ctx, cfg.(DiscordConfig), payload)
	case "telegram":
		return sendTelegram(ctx, cfg.(TelegramConfig), payload)
	default:
		return 0, &DeliveryError{Retryable: false, Err: errors.New("unsupported channel type")}
	}
}

func sendWebhook(ctx context.Context, cfg WebhookConfig, payload alert.Payload, notificationID string) (int, error) {
	subject, text, _ := FormatMessage(payload)
	body, err := json.Marshal(map[string]any{
		"event":           payload.Event,
		"notification_id": notificationID,
		"subject":         subject,
		"text":            text,
		"payload":         payload,
	})
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	if err := ValidateOutboundURL(u, cfg.AllowPrivate); err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "iwmmd/1")
	req.Header.Set("X-IWMMD-Event", payload.Event)
	req.Header.Set("X-IWMMD-Delivery", notificationID)
	for key, value := range cfg.Headers {
		req.Header.Set(key, value)
	}
	if cfg.Secret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		_, _ = mac.Write([]byte(timestamp + "." + string(body)))
		req.Header.Set("X-IWMMD-Timestamp", timestamp)
		req.Header.Set("X-IWMMD-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	client := SafeHTTPClient(cfg.AllowPrivate, 15*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return 0, &DeliveryError{Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode, classifyHTTP(resp.StatusCode)
}

type discordWebhookPayload struct {
	Username   string             `json:"username,omitempty"`
	AvatarURL  string             `json:"avatar_url,omitempty"`
	Embeds     []discordEmbed     `json:"embeds"`
	Components []discordActionRow `json:"components,omitempty"`
}

type discordEmbed struct {
	Author      *discordAuthor `json:"author,omitempty"`
	Title       string         `json:"title,omitempty"`
	URL         string         `json:"url,omitempty"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color,omitempty"`
	Fields      []discordField `json:"fields,omitempty"`
	Footer      *discordFooter `json:"footer,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

type discordAuthor struct {
	Name string `json:"name"`
}

type discordFooter struct {
	Text string `json:"text"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type discordActionRow struct {
	Type       int             `json:"type"`
	Components []discordButton `json:"components"`
}

type discordButton struct {
	Type  int    `json:"type"`
	Style int    `json:"style"`
	Label string `json:"label,omitempty"`
	URL   string `json:"url,omitempty"`
}

func buildDiscordPayload(cfg DiscordConfig, payload alert.Payload) discordWebhookPayload {
	msg := buildMessage(payload)
	embed := discordEmbed{
		Author:      &discordAuthor{Name: "IWMMD"},
		Title:       strings.TrimPrefix(msg.subject, "[IWMMD] "),
		Description: msg.summary,
		Color:       discordColor(msg.tone.accent),
		Footer:      &discordFooter{Text: "IWMMD • Automated domain monitoring"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	if isHTTPURL(msg.actionURL) {
		embed.URL = msg.actionURL
	}
	if payload.Event == "test" || payload.Kind == "system" {
		if msg.status != "" {
			embed.Fields = append(embed.Fields, discordField{Name: "Status", Value: msg.status, Inline: true})
		}
	}
	for _, fact := range msg.visibleFacts() {
		embed.Fields = append(embed.Fields, discordField{Name: fact.label, Value: fact.value, Inline: true})
	}

	body := discordWebhookPayload{
		Username:  cfg.Username,
		AvatarURL: cfg.AvatarURL,
		Embeds:    []discordEmbed{embed},
	}
	if isHTTPURL(msg.actionURL) {
		label := msg.actionLabel
		if label == "" {
			label = "Open IWMMD"
		}
		body.Components = []discordActionRow{{
			Type: 1,
			Components: []discordButton{{
				Type:  2,
				Style: 5,
				Label: label,
				URL:   msg.actionURL,
			}},
		}}
	}
	return body
}

func discordColor(value string) int {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	color, err := strconv.ParseInt(value, 16, 32)
	if err != nil {
		return 0x0b6e69
	}
	return int(color)
}

func isHTTPURL(value string) bool {
	value = strings.ToLower(value)
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}

func redactWebhookError(err error, webhookURL string) error {
	if err == nil {
		return nil
	}
	u, parseErr := url.Parse(webhookURL)
	if parseErr != nil {
		return err
	}
	raw := u.String()
	if !strings.Contains(err.Error(), raw) {
		return err
	}
	redacted := u.Scheme + "://" + u.Host + "/api/webhooks/<redacted>"
	return errors.New(strings.ReplaceAll(err.Error(), raw, redacted))
}

func sendDiscord(ctx context.Context, cfg DiscordConfig, payload alert.Payload) (int, error) {
	body, err := json.Marshal(buildDiscordPayload(cfg, payload))
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: redactWebhookError(err, cfg.WebhookURL)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "iwmmd/1")
	resp, err := SafeHTTPClient(false, 15*time.Second).Do(req)
	if err != nil {
		return 0, &DeliveryError{Retryable: true, Err: redactWebhookError(err, cfg.WebhookURL)}
	}
	defer resp.Body.Close()
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode, classifyHTTPWithBody(resp.StatusCode, detail)
}

func sendTelegram(ctx context.Context, cfg TelegramConfig, payload alert.Payload) (int, error) {
	subject, text, _ := FormatMessage(payload)
	message := "<b>" + escapeHTML(subject) + "</b>\n" + escapeHTML(text)
	body, err := json.Marshal(map[string]any{
		"chat_id":    cfg.ChatID,
		"text":       message,
		"parse_mode": "HTML",
	})
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	endpoint := "https://api.telegram.org/bot" + cfg.BotToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "iwmmd/1")
	resp, err := SafeHTTPClient(false, 15*time.Second).Do(req)
	if err != nil {
		return 0, &DeliveryError{Retryable: true, Err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode, classifyHTTP(resp.StatusCode)
}

func classifyHTTP(status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	retryable := status == http.StatusRequestTimeout || status == http.StatusTooManyRequests ||
		status == 425 || status >= 500
	return &DeliveryError{
		Status:    status,
		Retryable: retryable,
		Err:       fmt.Errorf("endpoint returned HTTP %d", status),
	}
}

func classifyHTTPWithBody(status int, body []byte) error {
	err := classifyHTTP(status)
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		return err
	}
	var delivery *DeliveryError
	if errors.As(err, &delivery) {
		delivery.Err = fmt.Errorf("%w: %s", delivery.Err, detail)
	}
	return err
}

func escapeHTML(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}
