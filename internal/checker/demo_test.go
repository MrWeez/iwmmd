package checker

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestDemoCheckerKnownDomain(t *testing.T) {
	result := Demo{}.Check(context.Background(), "example.com")
	if result.Status != "ok" || result.ExpiresAt == nil || result.Source != "rdap" {
		t.Fatalf("result = %+v", result)
	}
	want := time.Now().UTC().AddDate(0, 0, 6).Format("2006-01-02")
	if got := result.ExpiresAt.Format("2006-01-02"); got != want {
		t.Fatalf("expiry = %s want %s", got, want)
	}
	var body map[string]any
	if err := json.Unmarshal(result.Raw, &body); err != nil {
		t.Fatalf("raw JSON: %v", err)
	}
	if body["demo"] != true {
		t.Fatalf("raw = %s", result.Raw)
	}
}

func TestDemoCheckerFailures(t *testing.T) {
	unsupported := Demo{}.Check(context.Background(), "acme-demo.net")
	if unsupported.Status != "unsupported" {
		t.Fatalf("unsupported result = %+v", unsupported)
	}
	failed := Demo{}.Check(context.Background(), "broken.example")
	if failed.Status != "error" || failed.ErrorCode != "lookup_failed" {
		t.Fatalf("failed result = %+v", failed)
	}
}

func TestDemoCheckerIsDeterministic(t *testing.T) {
	first := Demo{}.Check(context.Background(), "random-demo.test")
	second := Demo{}.Check(context.Background(), "random-demo.test")
	if first.ExpiresAt == nil || second.ExpiresAt == nil {
		t.Fatalf("missing expiry: %+v %+v", first, second)
	}
	if !first.ExpiresAt.Equal(*second.ExpiresAt) {
		t.Fatalf("expiry changed: %s != %s", first.ExpiresAt, second.ExpiresAt)
	}
}
