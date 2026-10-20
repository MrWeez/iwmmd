package demo

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mrweez/iwmmd/internal/notify"
	"github.com/mrweez/iwmmd/internal/platform/secrets"
	"github.com/mrweez/iwmmd/internal/store"
)

func testSeeder(t *testing.T) (*Seeder, *store.Store) {
	t.Helper()
	st, err := store.Open(store.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	box, err := secrets.New(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	state := NewState("member", 30*time.Minute)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewSeeder(st, box, state, logger), st
}

func count(t *testing.T, st *store.Store, table string) int {
	t.Helper()
	var n int
	if err := st.Reader.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeederInitializeAndReset(t *testing.T) {
	seeder, st := testSeeder(t)
	now := time.Now().UTC()
	if err := seeder.Initialize(context.Background(), now); err != nil {
		t.Fatal(err)
	}

	if got := count(t, st, "domains"); got != 8 {
		t.Fatalf("domains = %d", got)
	}
	if got := count(t, st, "channels"); got != 4 {
		t.Fatalf("channels = %d", got)
	}
	if got := count(t, st, "alert_rules"); got != 4 {
		t.Fatalf("alert_rules = %d", got)
	}
	if got := count(t, st, "incidents"); got != 3 {
		t.Fatalf("incidents = %d", got)
	}
	if got := count(t, st, "notifications"); got != 11 {
		t.Fatalf("notifications = %d", got)
	}
	if got := count(t, st, "audit_log"); got != 6 {
		t.Fatalf("audit_log = %d", got)
	}

	var role, email string
	if err := st.Reader.QueryRow("SELECT role, email FROM users WHERE id = ?", UserID).Scan(&role, &email); err != nil {
		t.Fatal(err)
	}
	if role != "member" || email != UserEmail {
		t.Fatalf("user = %q %q", role, email)
	}

	var ciphertext string
	if err := st.Reader.QueryRow("SELECT config_ciphertext FROM channels WHERE name = 'Ops email'").Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	body, err := seeder.Box.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := notify.DecodeConfig("email", body)
	if err != nil {
		t.Fatal(err)
	}
	emailCfg, ok := cfg.(notify.EmailConfig)
	if !ok || len(emailCfg.Recipients) != 2 {
		t.Fatalf("channel config = %#v", cfg)
	}

	if _, err := st.Writer.Exec("UPDATE settings SET value_json = '\"changed\"' WHERE key = 'smtp.host'"); err != nil {
		t.Fatal(err)
	}
	if err := seeder.Initialize(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var hostJSON string
	if err := st.Reader.QueryRow("SELECT value_json FROM settings WHERE key = 'smtp.host'").Scan(&hostJSON); err != nil {
		t.Fatal(err)
	}
	if hostJSON != `"smtp.demo.invalid"` {
		t.Fatalf("smtp.host = %s", hostJSON)
	}
	if got := count(t, st, "domains"); got != 8 {
		t.Fatalf("domains after reset = %d", got)
	}
	if got := count(t, st, "notifications"); got != 11 {
		t.Fatalf("notifications after reset = %d", got)
	}
}

func TestSeederRefusesExistingData(t *testing.T) {
	seeder, st := testSeeder(t)
	if _, err := st.Writer.Exec("INSERT INTO settings (key, value_json, is_secret) VALUES ('smtp.host', '\"real.example.com\"', 0)"); err != nil {
		t.Fatal(err)
	}
	err := seeder.Initialize(context.Background(), time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "refuses to start") {
		t.Fatalf("error = %v", err)
	}
	if got := count(t, st, "settings"); got != 1 {
		t.Fatalf("settings after refusal = %d", got)
	}
}

func TestSeederRunResets(t *testing.T) {
	seeder, st := testSeeder(t)
	if err := seeder.Initialize(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Writer.Exec("UPDATE settings SET value_json = '\"changed\"' WHERE key = 'smtp.host'"); err != nil {
		t.Fatal(err)
	}
	seeder.State.ResetInterval = 50 * time.Millisecond
	seeder.State.SetNextReset(time.Now().Add(50 * time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		seeder.Run(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var value string
		if err := st.Reader.QueryRow("SELECT value_json FROM settings WHERE key = 'smtp.host'").Scan(&value); err != nil {
			t.Fatal(err)
		}
		if value == `"smtp.demo.invalid"` {
			cancel()
			<-done
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatal("demo reset loop did not run")
}
