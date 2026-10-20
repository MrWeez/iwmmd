package auth

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mrweez/iwmmd/internal/config"
)

// ErrDenied means the identity matched no authorization source.
var ErrDenied = errors.New("access denied")

// Identity is the authorized user derived from OIDC claims.
type Identity struct {
	Subject     string
	Email       string
	DisplayName string
	Role        string
}

// MapIdentity applies group and email authorization rules.
func MapIdentity(cfg config.OIDC, claims map[string]any) (Identity, error) {
	subject := claimString(claims, "sub")
	if subject == "" {
		return Identity{}, errors.New("ID token has no subject")
	}
	email := strings.ToLower(strings.TrimSpace(claimString(claims, cfg.EmailClaim)))
	name := claimString(claims, cfg.NameClaim)
	groups := claimStrings(claims, cfg.GroupsClaim)
	emailVerified, hasVerification := claimBool(claims, "email_verified")

	groupSet := make(map[string]bool, len(groups))
	for _, group := range groups {
		groupSet[group] = true
	}
	emailUsable := email != "" && (cfg.TrustEmailClaim || !hasVerification || emailVerified)

	if matchesAny(groupSet, cfg.AdminGroups) {
		return Identity{Subject: subject, Email: email, DisplayName: name, Role: "admin"}, nil
	}
	if emailUsable && containsFold(cfg.AdminEmails, email) {
		return Identity{Subject: subject, Email: email, DisplayName: name, Role: "admin"}, nil
	}
	if matchesAny(groupSet, cfg.AllowedGroups) {
		return Identity{Subject: subject, Email: email, DisplayName: name, Role: "member"}, nil
	}
	if emailUsable {
		domain := email
		if idx := strings.LastIndex(email, "@"); idx >= 0 {
			domain = email[idx+1:]
		}
		if containsFold(cfg.AllowedEmailDomains, domain) {
			return Identity{Subject: subject, Email: email, DisplayName: name, Role: "member"}, nil
		}
		if containsFold(cfg.AllowedEmails, email) {
			return Identity{Subject: subject, Email: email, DisplayName: name, Role: "member"}, nil
		}
	}
	return Identity{}, ErrDenied
}

// ClaimValue walks a dot-separated claim path, supporting numeric array indexes.
func ClaimValue(claims map[string]any, path string) any {
	if path == "" {
		return nil
	}
	var current any = claims
	for _, part := range strings.Split(path, ".") {
		switch value := current.(type) {
		case map[string]any:
			current = value[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) {
				return nil
			}
			current = value[index]
		default:
			return nil
		}
	}
	return current
}

func claimString(claims map[string]any, path string) string {
	value := ClaimValue(claims, path)
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func claimStrings(claims map[string]any, path string) []string {
	value := ClaimValue(claims, path)
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []any:
		groups := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				groups = append(groups, text)
			}
		}
		return groups
	case []string:
		return typed
	default:
		return nil
	}
}

func claimBool(claims map[string]any, key string) (bool, bool) {
	value, ok := claims[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err != nil {
			return false, false
		}
		return parsed, true
	default:
		return false, false
	}
}

func matchesAny(set map[string]bool, values []string) bool {
	for _, value := range values {
		if set[value] {
			return true
		}
	}
	return false
}

func containsFold(values []string, want string) bool {
	want = strings.ToLower(want)
	for _, value := range values {
		if strings.ToLower(value) == want {
			return true
		}
	}
	return false
}
