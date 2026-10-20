package checker

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/checker/model"
)

// Demo returns deterministic fake lookup results without network access.
type Demo struct{}

var demoExpiryDays = map[string]int{
	"example.com":        6,
	"example.net":        21,
	"example.org":        75,
	"northwind-demo.com": -4,
	"paused-demo.org":    120,
	"archived-demo.com":  180,
	"labs-demo.io":       2,
}

// Check returns a stable result derived from the domain name.
func (Demo) Check(ctx context.Context, name string) model.Result {
	_ = ctx
	lower := strings.ToLower(name)
	now := time.Now().UTC()

	if lower == "acme-demo.net" {
		return model.Result{
			Source:       "rdap",
			Status:       "unsupported",
			Confidence:   "unknown",
			ErrorCode:    "no_checker",
			ErrorMessage: "Demo checker has no expiry source for this domain",
			Duration:     140 * time.Millisecond,
			ContentType:  "application/json",
			Raw:          []byte(`{"demo":true,"error":"no_expiry_source"}`),
		}
	}
	if strings.Contains(lower, "broken") || strings.Contains(lower, "fail") {
		status := 404
		return model.Result{
			Source:       "rdap",
			Status:       "error",
			HTTPStatus:   &status,
			Confidence:   "unknown",
			ErrorCode:    "lookup_failed",
			ErrorMessage: "Demo checker simulated a failed lookup",
			Duration:     160 * time.Millisecond,
			ContentType:  "application/json",
			Raw:          []byte(`{"demo":true,"error":"lookup_failed"}`),
		}
	}

	days, ok := demoExpiryDays[lower]
	if !ok {
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(lower))
		days = int(hash.Sum32()%360) - 5
	}
	expiry := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC).AddDate(0, 0, days)
	status := 200
	raw := fmt.Sprintf(`{"objectClassName":"domain","ldhName":%q,"demo":true,"events":[{"eventAction":"expiration","eventDate":%q}]}`,
		lower, expiry.Format(time.RFC3339))
	return model.Result{
		Source:      "rdap",
		Status:      "ok",
		HTTPStatus:  &status,
		ExpiresAt:   &expiry,
		Registrar:   "Demo Registrar",
		Confidence:  "high",
		Duration:    180 * time.Millisecond,
		Raw:         []byte(raw),
		ContentType: "application/rdap+json",
	}
}
