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

func sendDiscord(ctx context.Context, cfg DiscordConfig, payload alert.Payload) (int, error) {
	subject, text, _ := FormatMessage(payload)
	color := 0xdc2626
	if payload.SystemEvent == "resolved" {
		color = 0x16a34a
	}
	body, err := json.Marshal(map[string]any{
		"username":   cfg.Username,
		"avatar_url": cfg.AvatarURL,
		"embeds": []map[string]any{{
			"title":       subject,
			"description": text,
			"color":       color,
		}},
	})
	if err != nil {
		return 0, &DeliveryError{Retryable: false, Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
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

func escapeHTML(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}
