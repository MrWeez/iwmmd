// Package checker orchestrates RDAP with WHOIS fallback.
package checker

import (
	"context"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/checker/model"
	"github.com/mrweez/iwmmd/internal/checker/rdap"
	"github.com/mrweez/iwmmd/internal/checker/whois"
)

// Service performs domain checks.
type Service struct {
	RDAP  *rdap.Client
	WHOIS *whois.Client
}

// New creates a checker service.
func New(dataDir string) *Service {
	return &Service{RDAP: rdap.New(dataDir), WHOIS: whois.New()}
}

// Check tries RDAP first and falls back to WHOIS.
func (s *Service) Check(ctx context.Context, name string) model.Result {
	start := time.Now()
	rdapResult := s.RDAP.Check(ctx, name)
	if rdapResult.Status == "ok" && rdapResult.ExpiresAt != nil {
		return rdapResult
	}
	whoisResult := s.WHOIS.Check(ctx, name)
	if whoisResult.Status == "ok" && whoisResult.ExpiresAt != nil {
		whoisResult.Duration = time.Since(start)
		return whoisResult
	}

	result := rdapResult
	if whoisResult.Raw != nil {
		result = whoisResult
	}
	result.Duration = time.Since(start)
	result.Source = "rdap+whois"
	if rdapResult.Status == "unsupported" && whoisResult.Status == "unsupported" {
		result.Status = "unsupported"
		result.ErrorCode = "no_checker"
		result.ErrorMessage = "no RDAP or WHOIS service is available for this domain"
		return result
	}
	result.Status = "error"
	if isParseFailure(rdapResult) || isParseFailure(whoisResult) {
		result.ErrorCode = "parse"
		result.ErrorMessage = "RDAP and WHOIS responses did not contain a usable expiry date"
		return result
	}
	var messages []string
	if rdapResult.ErrorMessage != "" {
		messages = append(messages, "RDAP: "+rdapResult.ErrorMessage)
	}
	if whoisResult.ErrorMessage != "" {
		messages = append(messages, "WHOIS: "+whoisResult.ErrorMessage)
	}
	if result.ErrorCode == "" || result.ErrorCode == "expiry_not_found" {
		result.ErrorCode = "lookup_failed"
	}
	result.ErrorMessage = strings.Join(messages, "; ")
	return result
}

func isParseFailure(result model.Result) bool {
	return result.ErrorCode == "parse" || result.ErrorCode == "expiry_not_found"
}
