package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/mrweez/iwmmd/internal/config"
)

// OIDC drives the authorization-code flow for one provider.
type OIDC struct {
	cfg      config.OIDC
	provider *oidc.Provider
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// NewOIDC discovers the provider and returns nil when OIDC is not configured.
func NewOIDC(ctx context.Context, cfg config.OIDC) (*OIDC, error) {
	if cfg.Issuer == "" {
		return nil, nil
	}
	client := &http.Client{Timeout: 20 * time.Second}
	discoveryCtx, cancel := context.WithTimeout(oidc.ClientContext(ctx, client), 20*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(discoveryCtx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery: %w", err)
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email", "groups"}
	}
	hasOpenID := false
	for _, scope := range scopes {
		if scope == oidc.ScopeOpenID {
			hasOpenID = true
		}
	}
	if !hasOpenID {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	oauthCfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       scopes,
	}
	return &OIDC{
		cfg:      cfg,
		provider: provider,
		oauth:    oauthCfg,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
	}, nil
}

// LoginURL builds the authorization redirect.
func (o *OIDC) LoginURL(state, nonce, verifier string) string {
	return o.oauth.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", oauth2.S256ChallengeFromVerifier(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// Exchange completes login and maps the identity.
func (o *OIDC) Exchange(ctx context.Context, code, verifier, nonce string) (Identity, error) {
	exchangeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	token, err := o.oauth.Exchange(exchangeCtx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return Identity{}, errors.New("token response did not include an ID token")
	}
	idToken, err := o.verifier.Verify(exchangeCtx, rawIDToken)
	if err != nil {
		return Identity{}, fmt.Errorf("verify ID token: %w", err)
	}
	if idToken.Nonce != nonce {
		return Identity{}, errors.New("ID token nonce mismatch")
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("decode ID token claims: %w", err)
	}
	return MapIdentity(o.cfg, claims)
}

// Issuer returns the configured issuer.
func (o *OIDC) Issuer() string { return o.cfg.Issuer }

// ProviderName returns the configured preset name.
func (o *OIDC) ProviderName() string { return o.cfg.Provider }
