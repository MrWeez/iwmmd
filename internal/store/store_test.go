package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOpenMigrateReady(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	if err := st.Ready(context.Background()); err == nil {
		t.Fatal("Ready succeeded before migrations")
	}
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := st.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := st.Reader.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'domains'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("domains table count = %d", count)
	}

	var journalMode string
	if err := st.Writer.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q", journalMode)
	}
}

func TestInstanceLock(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	_, err = Open(Options{DataDir: dir})
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenRawDoesNotLock(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	raw, err := OpenRaw(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if err := raw.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestClaimedNotificationIsNotReactivated(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	now := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	input := NotificationInput{
		Kind:          "system",
		DedupeKey:     "test-claimed-notification",
		ScheduledAt:   now.Add(-time.Minute),
		PayloadJSON:   `{"event":"system.incident"}`,
		UpdatePending: true,
		Reactivate:    true,
	}
	created, isNew, err := st.EnqueueNotification(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !isNew {
		t.Fatal("notification was not inserted")
	}

	claimed, err := st.ClaimDueNotifications(ctx, now, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != created.ID {
		t.Fatalf("claimed = %+v, want notification %s", claimed, created.ID)
	}

	input.ScheduledAt = now.Add(time.Hour)
	existing, isNew, err := st.EnqueueNotification(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if isNew {
		t.Fatal("duplicate notification was inserted")
	}
	if existing.State != "sending" {
		t.Fatalf("state = %q, want sending", existing.State)
	}
	current, err := st.GetNotification(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", current.Attempts)
	}

	if err := st.MarkNotificationSent(ctx, created.ID, now); err != nil {
		t.Fatalf("mark claimed notification sent: %v", err)
	}
	sent, err := st.GetNotification(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sent.State != "sent" {
		t.Fatalf("state = %q, want sent", sent.State)
	}
}
