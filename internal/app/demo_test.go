package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/mrweez/iwmmd/internal/checker"
	"github.com/mrweez/iwmmd/internal/config"
)

func TestNewDemoMode(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.MasterKey = bytes.Repeat([]byte{0x42}, 32)
	cfg.Demo.Enabled = true
	cfg.Demo.Role = "member"
	cfg.Demo.ResetInterval = "30m"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := New(&cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	if a.demo == nil {
		t.Fatal("demo seeder is nil")
	}
	if _, ok := a.domains.Checker.(checker.Demo); !ok {
		t.Fatalf("checker = %T", a.domains.Checker)
	}
	channels, err := a.channels.List(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 4 {
		t.Fatalf("channels = %d", len(channels))
	}
	if _, err := a.settings.Get(context.Background()); err != nil {
		t.Fatal(err)
	}

	var domains, notifications int
	if err := a.store.Reader.QueryRow("SELECT COUNT(*) FROM domains").Scan(&domains); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Reader.QueryRow("SELECT COUNT(*) FROM notifications").Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	if domains != 8 {
		t.Fatalf("domains = %d", domains)
	}
	if notifications != 11 {
		t.Fatalf("notifications = %d", notifications)
	}
}
