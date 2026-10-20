package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
)

// EmailConfig delivers to one or more addresses.
type EmailConfig struct {
	Recipients []string `json:"recipients"`
}

// WebhookConfig posts JSON to an arbitrary endpoint.
type WebhookConfig struct {
	URL          string            `json:"url"`
	Secret       string            `json:"secret,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	AllowPrivate bool              `json:"allow_private,omitempty"`
}

// DiscordConfig posts to a Discord webhook.
type DiscordConfig struct {
	WebhookURL string `json:"webhook_url"`
	Username   string `json:"username,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
}

// TelegramConfig sends messages through a Telegram bot.
type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	ThreadID string `json:"thread_id,omitempty"`
}

// DecodeConfig validates and decodes a channel configuration.
func DecodeConfig(channelType string, body []byte) (any, error) {
	switch channelType {
	case "email":
		var cfg EmailConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			return nil, fmt.Errorf("decode email config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return cfg, nil
	case "webhook":
		var cfg WebhookConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			return nil, fmt.Errorf("decode webhook config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return cfg, nil
	case "discord":
		var cfg DiscordConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			return nil, fmt.Errorf("decode discord config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return cfg, nil
	case "telegram":
		var cfg TelegramConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			return nil, fmt.Errorf("decode telegram config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		return cfg, nil
	default:
		return nil, errors.New("unsupported channel type")
	}
}

// EncodeConfig marshals a validated config.
func EncodeConfig(cfg any) ([]byte, error) { return json.Marshal(cfg) }

// Validate checks an email config.
func (c *EmailConfig) Validate() error {
	if len(c.Recipients) == 0 {
		return errors.New("at least one email recipient is required")
	}
	if len(c.Recipients) > 50 {
		return errors.New("at most 50 email recipients are allowed")
	}
	seen := map[string]bool{}
	for i, recipient := range c.Recipients {
		address, err := mail.ParseAddress(strings.TrimSpace(recipient))
		if err != nil || strings.ContainsAny(recipient, "\r\n") {
			return fmt.Errorf("recipient %d is not a valid email address", i+1)
		}
		normalized := strings.ToLower(address.Address)
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		c.Recipients[i] = address.Address
	}
	return nil
}

// Validate checks a webhook config.
func (c *WebhookConfig) Validate() error {
	u, err := url.Parse(c.URL)
	if err != nil {
		return errors.New("webhook URL is invalid")
	}
	if err := ValidateOutboundURL(u, c.AllowPrivate); err != nil {
		return err
	}
	if len(c.Headers) > 20 {
		return errors.New("at most 20 webhook headers are allowed")
	}
	for key, value := range c.Headers {
		if key == "" || strings.ContainsAny(key, "\r\n:") {
			return errors.New("webhook header name is invalid")
		}
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("webhook header value contains a newline")
		}
	}
	return nil
}

// Validate checks a Discord config.
func (c *DiscordConfig) Validate() error {
	u, err := url.Parse(c.WebhookURL)
	if err != nil || u.Scheme != "https" {
		return errors.New("Discord webhook URL must use https")
	}
	host := strings.ToLower(u.Hostname())
	if host != "discord.com" && host != "discordapp.com" && host != "ptb.discord.com" && host != "canary.discord.com" {
		return errors.New("Discord webhook URL must point to discord.com")
	}
	if !strings.Contains(u.Path, "/api/webhooks/") {
		return errors.New("Discord webhook URL is not a webhook endpoint")
	}
	if c.AvatarURL != "" {
		avatar, err := url.Parse(c.AvatarURL)
		if err != nil || (avatar.Scheme != "https" && avatar.Scheme != "http") {
			return errors.New("Discord avatar URL is invalid")
		}
	}
	return nil
}

// Validate checks a Telegram config.
func (c *TelegramConfig) Validate() error {
	if strings.TrimSpace(c.BotToken) == "" {
		return errors.New("Telegram bot token is required")
	}
	if strings.ContainsAny(c.BotToken, " \r\n/") {
		return errors.New("Telegram bot token is invalid")
	}
	if strings.TrimSpace(c.ChatID) == "" {
		return errors.New("Telegram chat ID is required")
	}
	if strings.ContainsAny(c.ChatID, "\r\n") {
		return errors.New("Telegram chat ID is invalid")
	}
	return nil
}

// Summary returns non-secret display fields for a config.
func Summary(cfg any) map[string]any {
	switch value := cfg.(type) {
	case EmailConfig:
		return map[string]any{"recipients": value.Recipients}
	case WebhookConfig:
		host := ""
		if u, err := url.Parse(value.URL); err == nil {
			host = u.Host
		}
		return map[string]any{"host": host, "secret_set": value.Secret != "", "allow_private": value.AllowPrivate}
	case DiscordConfig:
		host := ""
		if u, err := url.Parse(value.WebhookURL); err == nil {
			host = u.Host
		}
		return map[string]any{"host": host}
	case TelegramConfig:
		return map[string]any{"chat_id": value.ChatID, "thread_id": value.ThreadID, "token_set": value.BotToken != ""}
	default:
		return map[string]any{}
	}
}

// MergeConfig overlays new values onto an existing config while preserving
// secret fields that were left empty.
func MergeConfig(channelType string, oldBody, newBody []byte) ([]byte, error) {
	var oldMap, newMap map[string]any
	if err := json.Unmarshal(oldBody, &oldMap); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(newBody, &newMap); err != nil {
		return nil, err
	}
	switch channelType {
	case "webhook":
		if text, _ := newMap["secret"].(string); text == "" {
			delete(newMap, "secret")
		}
	case "telegram":
		if text, _ := newMap["bot_token"].(string); text == "" {
			delete(newMap, "bot_token")
		}
	}
	for key, value := range newMap {
		oldMap[key] = value
	}
	return json.Marshal(oldMap)
}
