package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/platform/ids"
	"github.com/mrweez/iwmmd/internal/platform/secrets"
	"github.com/mrweez/iwmmd/internal/store"
)

// ChannelService manages notification channel configuration.
type ChannelService struct {
	Store  *store.Store
	Box    *secrets.Box
	Sender *Sender
}

// CreateChannelInput is a channel create request.
type CreateChannelInput struct {
	Name                string          `json:"name"`
	Type                string          `json:"type"`
	Config              json.RawMessage `json:"config"`
	Enabled             bool            `json:"enabled"`
	ReceiveSystemAlerts bool            `json:"receive_system_alerts"`
}

// UpdateChannelInput is a channel update request.
type UpdateChannelInput struct {
	Name                string          `json:"name"`
	Config              json.RawMessage `json:"config,omitempty"`
	Enabled             bool            `json:"enabled"`
	ReceiveSystemAlerts bool            `json:"receive_system_alerts"`
}

// ChannelView is a safe API representation of a channel.
type ChannelView struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	Enabled             bool           `json:"enabled"`
	ReceiveSystemAlerts bool           `json:"receive_system_alerts"`
	Config              map[string]any `json:"config"`
	MutedUntil          *time.Time     `json:"muted_until,omitempty"`
	MuteReason          string         `json:"mute_reason,omitempty"`
	ArchivedAt          *time.Time     `json:"archived_at,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// Create validates and stores a channel.
func (s *ChannelService) Create(ctx context.Context, in CreateChannelInput, userID string, at time.Time) (store.Channel, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 100 {
		return store.Channel{}, errors.New("channel name must be between 1 and 100 characters")
	}
	if !validType(in.Type) {
		return store.Channel{}, errors.New("channel type must be email, webhook, discord, or telegram")
	}
	if len(in.Config) == 0 || string(in.Config) == "null" {
		return store.Channel{}, errors.New("channel config is required")
	}
	cfg, err := DecodeConfig(in.Type, in.Config)
	if err != nil {
		return store.Channel{}, err
	}
	body, err := EncodeConfig(cfg)
	if err != nil {
		return store.Channel{}, err
	}
	ciphertext, err := s.Box.Encrypt(body)
	if err != nil {
		return store.Channel{}, err
	}
	channel := store.Channel{
		ID:                  ids.New(),
		Name:                name,
		Type:                in.Type,
		ConfigCiphertext:    ciphertext,
		Enabled:             in.Enabled,
		ReceiveSystemAlerts: in.ReceiveSystemAlerts,
		CreatedBy:           userID,
		CreatedAt:           at,
		UpdatedAt:           at,
	}
	if err := s.Store.CreateChannel(ctx, channel); err != nil {
		return store.Channel{}, err
	}
	return channel, nil
}

// Update modifies a channel. Empty config preserves the stored config.
func (s *ChannelService) Update(ctx context.Context, id string, in UpdateChannelInput, at time.Time) (store.Channel, error) {
	existing, err := s.Store.GetChannel(ctx, id)
	if err != nil {
		return store.Channel{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 100 {
		return store.Channel{}, errors.New("channel name must be between 1 and 100 characters")
	}
	ciphertext := existing.ConfigCiphertext
	if len(in.Config) > 0 && string(in.Config) != "null" {
		oldBody, err := s.Box.Decrypt(existing.ConfigCiphertext)
		if err != nil {
			return store.Channel{}, err
		}
		merged, err := MergeConfig(existing.Type, oldBody, in.Config)
		if err != nil {
			return store.Channel{}, err
		}
		cfg, err := DecodeConfig(existing.Type, merged)
		if err != nil {
			return store.Channel{}, err
		}
		body, err := EncodeConfig(cfg)
		if err != nil {
			return store.Channel{}, err
		}
		ciphertext, err = s.Box.Encrypt(body)
		if err != nil {
			return store.Channel{}, err
		}
	}
	existing.Name = name
	existing.ConfigCiphertext = ciphertext
	existing.Enabled = in.Enabled
	existing.ReceiveSystemAlerts = in.ReceiveSystemAlerts
	existing.UpdatedAt = at
	if err := s.Store.UpdateChannel(ctx, existing); err != nil {
		return store.Channel{}, err
	}
	return existing, nil
}

// View builds a safe representation.
func (s *ChannelService) View(channel store.Channel) (ChannelView, error) {
	cfg, err := s.Sender.DecryptChannel(channel)
	if err != nil {
		return ChannelView{}, err
	}
	return ChannelView{
		ID:                  channel.ID,
		Name:                channel.Name,
		Type:                channel.Type,
		Enabled:             channel.Enabled,
		ReceiveSystemAlerts: channel.ReceiveSystemAlerts,
		Config:              Summary(cfg),
		MutedUntil:          channel.MutedUntil,
		MuteReason:          channel.MuteReason,
		ArchivedAt:          channel.ArchivedAt,
		CreatedAt:           channel.CreatedAt,
		UpdatedAt:           channel.UpdatedAt,
	}, nil
}

// List returns safe channel representations.
func (s *ChannelService) List(ctx context.Context, includeArchived bool) ([]ChannelView, error) {
	channels, err := s.Store.ListChannels(ctx, includeArchived)
	if err != nil {
		return nil, err
	}
	views := make([]ChannelView, 0, len(channels))
	for _, channel := range channels {
		view, err := s.View(channel)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// Test delivers a synthetic notification through one channel.
func (s *ChannelService) Test(ctx context.Context, id string) error {
	channel, err := s.Store.GetChannel(ctx, id)
	if err != nil {
		return err
	}
	if !channel.Enabled {
		return errors.New("channel is disabled")
	}
	payload := alert.Payload{
		Event:   "test",
		Kind:    "system",
		Domain:  "example.com",
		Message: "This is a test notification from IWMMD.",
		URL:     "",
	}
	status, err := s.Sender.Send(ctx, channel, payload, "test-"+id)
	if err != nil {
		var delivery *DeliveryError
		if errors.As(err, &delivery) {
			return fmt.Errorf("test delivery failed (status %d): %w", status, err)
		}
		return err
	}
	return nil
}

func validType(value string) bool {
	switch value {
	case "email", "webhook", "discord", "telegram":
		return true
	default:
		return false
	}
}
