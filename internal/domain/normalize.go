// Package domain contains domain validation and expiry schedule logic.
package domain

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"golang.org/x/net/idna"
)

// Normalized is a validated domain name in ASCII and display form.
type Normalized struct {
	ACE     string
	Unicode string
}

// Normalize validates and canonicalizes a user-supplied domain name.
func Normalize(raw string) (Normalized, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Normalized{}, errors.New("domain name is required")
	}
	if strings.Contains(value, "://") {
		return Normalized{}, errors.New("enter a domain name, not a URL")
	}
	if strings.ContainsAny(value, "/?#@* \\") {
		return Normalized{}, errors.New("domain name contains invalid characters")
	}
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return Normalized{}, errors.New("domain name is required")
	}
	if net.ParseIP(value) != nil {
		return Normalized{}, errors.New("IP addresses are not supported")
	}

	ace, err := idna.Lookup.ToASCII(value)
	if err != nil {
		return Normalized{}, fmt.Errorf("domain name is not valid IDNA: %w", err)
	}
	ace = strings.ToLower(strings.TrimSuffix(ace, "."))
	if ace == "" || len(ace) > 253 {
		return Normalized{}, errors.New("domain name must be between 1 and 253 characters")
	}
	labels := strings.Split(ace, ".")
	if len(labels) < 2 {
		return Normalized{}, errors.New("domain name must include a public suffix")
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return Normalized{}, errors.New("each domain label must be between 1 and 63 characters")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return Normalized{}, errors.New("domain labels cannot start or end with a hyphen")
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return Normalized{}, errors.New("domain name contains an unsupported character")
			}
		}
	}
	tld := labels[len(labels)-1]
	if len(tld) < 2 || strings.Trim(tld, "0123456789") == "" {
		return Normalized{}, errors.New("domain name must end with a valid alphabetic TLD")
	}

	unicode, err := idna.Lookup.ToUnicode(ace)
	if err != nil || unicode == "" {
		unicode = ace
	}
	return Normalized{ACE: ace, Unicode: unicode}, nil
}
