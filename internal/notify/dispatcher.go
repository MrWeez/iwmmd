package notify

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/platform/ids"
	"github.com/mrweez/iwmmd/internal/settings"
	"github.com/mrweez/iwmmd/internal/store"
)

// Dispatcher delivers queued notifications.
type Dispatcher struct {
	Store    *store.Store
	Sender   *Sender
	Settings *settings.Service
	Logger   *slog.Logger
	Interval time.Duration
	Batch    int
}

// Run processes the outbox until the context is canceled.
func (d *Dispatcher) Run(ctx context.Context) {
	interval := d.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	if _, err := d.ProcessOnce(ctx, time.Now().UTC()); err != nil && !errors.Is(err, context.Canceled) {
		d.logger().Error("dispatch notifications", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := d.ProcessOnce(ctx, time.Now().UTC()); err != nil && !errors.Is(err, context.Canceled) {
				d.logger().Error("dispatch notifications", "error", err)
			}
		}
	}
}

// ProcessOnce claims and delivers one batch. It is safe to call manually.
func (d *Dispatcher) ProcessOnce(ctx context.Context, now time.Time) (int, error) {
	batch := d.Batch
	if batch <= 0 {
		batch = 10
	}
	notifications, err := d.Store.ClaimDueNotifications(ctx, now, batch, 2*time.Minute)
	if err != nil {
		return 0, err
	}
	if len(notifications) == 0 {
		return 0, nil
	}
	snap, err := d.Settings.Get(ctx)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, notification := range notifications {
		ok, err := d.deliver(ctx, notification, snap, now)
		if err != nil {
			d.logger().Error("deliver notification", "notification_id", notification.ID, "error", err)
		}
		if ok {
			sent++
		}
	}
	return sent, nil
}

func (d *Dispatcher) deliver(ctx context.Context, n store.Notification, snap settings.Snapshot, now time.Time) (bool, error) {
	channel, err := d.Store.GetChannel(ctx, n.ChannelID)
	if err != nil || channel.ArchivedAt != nil || !channel.Enabled {
		if cancelErr := d.Store.CancelNotification(ctx, n.ID, now); cancelErr != nil {
			return false, cancelErr
		}
		return false, nil
	}

	var domain store.Domain
	if n.DomainID != "" {
		domain, err = d.Store.GetDomain(ctx, n.DomainID)
		if err != nil {
			if cancelErr := d.Store.CancelNotification(ctx, n.ID, now); cancelErr != nil {
				return false, cancelErr
			}
			return false, nil
		}
		if n.Kind == "expiry" && domain.State != "active" {
			return false, d.Store.CancelNotification(ctx, n.ID, now)
		}
	}

	hold := time.Time{}
	if n.Kind == "expiry" && domain.MutedUntil != nil && domain.MutedUntil.After(now) {
		hold = *domain.MutedUntil
	}
	if channel.MutedUntil != nil && channel.MutedUntil.After(hold) {
		hold = *channel.MutedUntil
	}
	if snap.Alerts.PausedUntil != nil && snap.Alerts.PausedUntil.After(hold) {
		hold = *snap.Alerts.PausedUntil
	}
	if hold.After(now) {
		return false, d.Store.RescheduleNotification(ctx, n.ID, now, hold)
	}

	newer, err := d.Store.NewerDueNotification(ctx, n, now)
	if err != nil {
		return false, err
	}
	if newer {
		return false, d.Store.SuppressNotification(ctx, n.ID, now)
	}

	payload, err := parsePayload(n.PayloadJSON)
	if err != nil {
		_ = d.Store.FailNotification(ctx, n.ID, "invalid notification payload", now)
		_ = d.Store.RecordDeliveryAttempt(ctx, store.DeliveryAttempt{
			ID:             ids.New(),
			NotificationID: n.ID,
			AttemptNo:      n.Attempts,
			StartedAt:      now,
			FinishedAt:     &now,
			Status:         "failed",
			Error:          err.Error(),
			CreatedAt:      now,
		})
		return false, err
	}

	started := time.Now().UTC()
	status, sendErr := d.Sender.Send(ctx, channel, payload, n.ID)
	finished := time.Now().UTC()
	attempt := store.DeliveryAttempt{
		ID:             ids.New(),
		NotificationID: n.ID,
		AttemptNo:      n.Attempts,
		StartedAt:      started,
		FinishedAt:     &finished,
		Status:         "success",
		ResponseCode:   &status,
		CreatedAt:      finished,
	}
	if sendErr == nil {
		if err := d.Store.MarkNotificationSent(ctx, n.ID, finished); err != nil {
			return false, err
		}
		return true, d.Store.RecordDeliveryAttempt(ctx, attempt)
	}

	var delivery *DeliveryError
	retryable := false
	if errors.As(sendErr, &delivery) {
		retryable = delivery.Retryable
	} else {
		retryable = true
	}
	if retryable && n.Attempts < 8 {
		retryAt := finished.Add(retryDelay(n.Attempts))
		attempt.Status = "retry"
		attempt.Error = sendErr.Error()
		if err := d.Store.RetryNotification(ctx, n.ID, sendErr.Error(), retryAt, finished); err != nil {
			return false, err
		}
		return false, d.Store.RecordDeliveryAttempt(ctx, attempt)
	}
	attempt.Status = "failed"
	attempt.Error = sendErr.Error()
	if err := d.Store.FailNotification(ctx, n.ID, sendErr.Error(), finished); err != nil {
		return false, err
	}
	return false, d.Store.RecordDeliveryAttempt(ctx, attempt)
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 7 {
		shift = 7
	}
	delay := 30 * time.Second * time.Duration(1<<shift)
	if delay > time.Hour {
		delay = time.Hour
	}
	return delay
}

func (d *Dispatcher) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

func parsePayload(body string) (alert.Payload, error) {
	var payload alert.Payload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}
