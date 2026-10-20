// Command iwmmd runs the IWMMD domain expiration monitor.
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/mrweez/iwmmd/internal/app"
	"github.com/mrweez/iwmmd/internal/config"
	"github.com/mrweez/iwmmd/internal/platform/logging"
	"github.com/mrweez/iwmmd/internal/platform/version"
	"github.com/mrweez/iwmmd/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(os.Args[2:])
	case "migrate":
		err = runMigrate(os.Args[2:])
	case "backup":
		err = runBackup(os.Args[2:])
	case "healthcheck":
		err = runHealthcheck(os.Args[2:])
	case "gen-key":
		err = runGenKey()
	case "version":
		fmt.Println(version.String())
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.toml")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	logger := logging.New(cfg.Log.Level, cfg.Log.Format)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(cfg, logger)
	if err != nil {
		return err
	}
	return application.Run(ctx)
}

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.toml")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	logger := logging.New(cfg.Log.Level, cfg.Log.Format)
	st, err := store.Open(store.Options{DataDir: cfg.DataDir, DSN: cfg.DBDSN})
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			logger.Error("close store", "error", err)
		}
	}()
	if err := st.Migrate(); err != nil {
		return err
	}
	logger.Info("migrations applied", "data_dir", cfg.DataDir)
	return nil
}

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to config.toml")
	out := fs.String("out", "", "output database file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	outPath := *out
	if outPath == "" {
		outPath = filepath.Join(cfg.DataDir, "backups", "iwmmd-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	}
	outPath, err = filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
		return err
	}
	if cfg.DBDSN == "" {
		if _, err := os.Stat(store.DatabasePath(cfg.DataDir)); err != nil {
			return fmt.Errorf("database not found: %w", err)
		}
	}

	db, err := store.OpenRaw(store.Options{DataDir: cfg.DataDir, DSN: cfg.DBDSN})
	if err != nil {
		return err
	}
	defer db.Close()

	escaped := strings.ReplaceAll(outPath, "'", "''")
	if _, err := db.ExecContext(context.Background(), "VACUUM INTO '"+escaped+"'"); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	if err := os.Chmod(outPath, 0o600); err != nil {
		return err
	}
	fmt.Println(outPath)
	return nil
}

func runHealthcheck(args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	url := fs.String("url", "http://127.0.0.1:8080/healthz", "health endpoint URL")
	insecure := fs.Bool("insecure", false, "skip TLS certificate verification")
	if err := fs.Parse(args); err != nil {
		return err
	}

	transport := &http.Transport{}
	if *insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	resp, err := client.Get(*url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("health endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func runGenKey() error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(key))
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `iwmmd - self-hosted domain expiration monitor

Usage:
  iwmmd serve       [--config PATH]
  iwmmd migrate     [--config PATH]
  iwmmd backup      [--config PATH] [--out PATH]
  iwmmd healthcheck [--url URL] [--insecure]
  iwmmd gen-key
  iwmmd version
`)
}
