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

func TestDomainRestoreAndPermanentDelete(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	archivedAt := now.Add(-24 * time.Hour)

	const domainID = "dom_lifecycle"
	domain := Domain{
		ID:                   domainID,
		NameACE:              "lifecycle.example",
		NameUnicode:          "lifecycle.example",
		State:                "archived",
		InheritGlobalAlerts:  true,
		CheckIntervalSeconds: 86400,
		ExpiryConfidence:     "high",
		Version:              1,
		CreatedAt:            archivedAt,
		UpdatedAt:            archivedAt,
		ArchivedAt:           &archivedAt,
	}
	if err := st.CreateDomain(ctx, domain); err != nil {
		t.Fatal(err)
	}
	at := formatTime(now)
	if _, err := st.Writer.ExecContext(ctx, `
		INSERT INTO domain_checks (id, domain_id, source, status, confidence, duration_ms, started_at)
		VALUES ('check_lifecycle', ?, 'rdap', 'ok', 'high', 12, ?)`, domainID, at); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Writer.ExecContext(ctx, `
		INSERT INTO alert_rules (id, name, scope, domain_id, offset_value, offset_unit, direction, send_time_local, enabled, created_at, updated_at)
		VALUES ('rule_lifecycle', 'Lifecycle rule', 'domain', ?, 30, 'days', 'before', '09:00', 1, ?, ?)`,
		domainID, at, at); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Writer.ExecContext(ctx, `
		INSERT INTO incidents (id, domain_id, kind, state, opened_at, last_seen_at, details_json, created_at, updated_at)
		VALUES ('incident_lifecycle', ?, 'check_failure', 'open', ?, ?, '{}', ?, ?)`,
		domainID, at, at, at, at); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.EnqueueNotification(ctx, NotificationInput{
		Kind:        "expiry",
		DomainID:    domainID,
		DedupeKey:   "lifecycle-notification",
		ScheduledAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.EnqueueJob(ctx, EnqueueJobInput{
		Kind:      "domain-check",
		RunAt:     now,
		UniqueKey: "domain-check:" + domainID,
	}); err != nil {
		t.Fatal(err)
	}

	restored, err := st.GetDomain(ctx, domainID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.State != "archived" || restored.ArchivedAt == nil {
		t.Fatalf("precondition state = %q archived_at = %v", restored.State, restored.ArchivedAt)
	}

	if err := st.RestoreDomain(ctx, domainID, now); err != nil {
		t.Fatal(err)
	}
	restored, err = st.GetDomain(ctx, domainID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.State != "active" {
		t.Fatalf("state = %q, want active", restored.State)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("archived_at = %v, want nil", restored.ArchivedAt)
	}
	if restored.NextCheckAt == nil || restored.NextCheckAt.Before(now) {
		t.Fatalf("next_check_at = %v, want at or after %v", restored.NextCheckAt, now)
	}
	if restored.Version != 2 {
		t.Fatalf("version = %d, want 2", restored.Version)
	}

	if err := st.DeleteDomainPermanent(ctx, domainID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete active domain error = %v, want ErrNotFound", err)
	}
	if _, err := st.GetDomain(ctx, domainID); err != nil {
		t.Fatalf("active domain removed: %v", err)
	}

	if err := st.ArchiveDomain(ctx, domainID, now); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteDomainPermanent(ctx, domainID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetDomain(ctx, domainID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("domain still exists after permanent delete: %v", err)
	}

	for _, table := range []string{"domain_checks", "alert_rules", "incidents", "notifications"} {
		var count int
		if err := st.Reader.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE domain_id = ?", domainID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s rows = %d, want 0", table, count)
		}
	}
	var jobs int
	if err := st.Reader.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE unique_key = ?", "domain-check:"+domainID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("domain-check jobs = %d, want 0", jobs)
	}
}

func TestListNotificationsExcludeArchived(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)

	for _, item := range []struct {
		id    string
		state string
	}{
		{id: "dom_active", state: "active"},
		{id: "dom_archived", state: "archived"},
	} {
		if err := st.CreateDomain(ctx, Domain{
			ID:                   item.id,
			NameACE:              item.id + ".example",
			NameUnicode:          item.id + ".example",
			State:                item.state,
			InheritGlobalAlerts:  true,
			CheckIntervalSeconds: 86400,
			ExpiryConfidence:     "high",
			Version:              1,
			CreatedAt:            now,
			UpdatedAt:            now,
		}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := st.EnqueueNotification(ctx, NotificationInput{
			Kind:        "expiry",
			DomainID:    item.id,
			DedupeKey:   "notification-" + item.id,
			ScheduledAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := st.EnqueueNotification(ctx, NotificationInput{
		Kind:        "system",
		DedupeKey:   "notification-system",
		ScheduledAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := st.ListNotifications(ctx, "", "", "", 50, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("all notifications = %d, want 3", len(all))
	}
	visible, err := st.ListNotifications(ctx, "", "", "", 50, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 2 {
		t.Fatalf("visible notifications = %d, want 2", len(visible))
	}
	for _, item := range visible {
		if item.DomainID == "dom_archived" {
			t.Fatal("archived domain notification was not excluded")
		}
	}
}

func TestListNotificationsHidesFuturePending(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, item := range []struct {
		key         string
		state       string
		scheduledAt time.Time
	}{
		{key: "past", state: "pending", scheduledAt: now.Add(-time.Hour)},
		{key: "future", state: "pending", scheduledAt: now.AddDate(1, 0, 0)},
		{key: "future-canceled", state: "canceled", scheduledAt: now.AddDate(2, 0, 0)},
	} {
		n, _, err := st.EnqueueNotification(ctx, NotificationInput{
			Kind:        "expiry",
			DedupeKey:   "future-filter-" + item.key,
			ScheduledAt: item.scheduledAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if item.state == "canceled" {
			if err := st.CancelNotification(ctx, n.ID, now); err != nil {
				t.Fatal(err)
			}
		}
	}

	visible, err := st.ListNotifications(ctx, "", "", "", 50, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 2 {
		t.Fatalf("visible notifications = %d, want 2", len(visible))
	}
	for _, item := range visible {
		if item.DedupeKey == "future-filter-future" {
			t.Fatal("future pending notification was not hidden")
		}
	}

	withFuture, err := st.ListNotifications(ctx, "", "", "", 50, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(withFuture) != 3 {
		t.Fatalf("notifications with future = %d, want 3", len(withFuture))
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	return st
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

func TestListAuditIncludesActorIdentity(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)

	user, err := st.UpsertUser(ctx, User{
		Issuer:      "https://id.example.com",
		Subject:     "audit-user",
		Email:       "admin@example.com",
		DisplayName: "Audit Admin",
		Role:        "admin",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.WriteAudit(ctx, AuditInput{
		ActorUserID: user.ID,
		Action:      "domain.create",
		EntityType:  "domain",
		EntityID:    "dom_audit",
		IP:          "203.0.113.5",
	}, now); err != nil {
		t.Fatal(err)
	}

	entries, err := st.ListAudit(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.ActorUserID != user.ID || entry.ActorEmail != "admin@example.com" || entry.ActorDisplayName != "Audit Admin" {
		t.Fatalf("actor = %q %q %q", entry.ActorUserID, entry.ActorEmail, entry.ActorDisplayName)
	}
}
