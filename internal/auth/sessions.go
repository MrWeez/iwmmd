// Package auth implements OIDC login, session cookies, CSRF, and role mapping.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/mrweez/iwmmd/internal/platform/secrets"
	"github.com/mrweez/iwmmd/internal/store"
)

const (
	sessionCookie = "iwmmd_session"
	csrfCookie    = "iwmmd_csrf"
	stateCookie   = "iwmmd_oidc_state"
)

// ErrUnauthorized means no valid session was presented.
var ErrUnauthorized = errors.New("authentication required")

// Sessions creates and validates server-side sessions.
type Sessions struct {
	Store            *store.Store
	Box              *secrets.Box
	Secure           bool
	Lifetime         time.Duration
	AbsoluteLifetime time.Duration
}

// NewSessions builds a session manager.
func NewSessions(st *store.Store, box *secrets.Box, secure bool) *Sessions {
	return &Sessions{
		Store:            st,
		Box:              box,
		Secure:           secure,
		Lifetime:         12 * time.Hour,
		AbsoluteLifetime: 7 * 24 * time.Hour,
	}
}

// CurrentUser resolves the session and returns the user and CSRF token.
func (s *Sessions) CurrentUser(r *http.Request, now time.Time) (store.User, string, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return store.User{}, "", ErrUnauthorized
	}
	session, user, err := s.Store.GetSessionUser(r.Context(), hashToken(cookie.Value), now)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.User{}, "", ErrUnauthorized
		}
		return store.User{}, "", err
	}
	if now.Sub(session.CreatedAt) < s.AbsoluteLifetime {
		expires := now.Add(s.Lifetime)
		absolute := session.CreatedAt.Add(s.AbsoluteLifetime)
		if expires.After(absolute) {
			expires = absolute
		}
		if expires.After(session.ExpiresAt.Add(-1 * time.Hour)) {
			_ = s.Store.ExtendSession(r.Context(), session.ID, expires, now)
		}
	}
	csrf := s.Box.HMAC("csrf:" + cookie.Value)
	return user, csrf, nil
}

// Create creates a session and sets cookies.
func (s *Sessions) Create(ctx context.Context, w http.ResponseWriter, user store.User, ip, agent string, now time.Time) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	expires := now.Add(s.Lifetime)
	absolute := now.Add(s.AbsoluteLifetime)
	if expires.After(absolute) {
		expires = absolute
	}
	session := store.Session{
		ID:        hashToken(token),
		UserID:    user.ID,
		CreatedAt: now,
		ExpiresAt: expires,
		IP:        ip,
		UserAgent: agent,
	}
	if err := s.Store.CreateSession(ctx, session); err != nil {
		return "", err
	}
	csrf := s.Box.HMAC("csrf:" + token)
	s.setCookie(w, sessionCookie, token, expires, true)
	s.setCookie(w, csrfCookie, csrf, expires, false)
	return csrf, nil
}

// Logout revokes the current session and clears cookies.
func (s *Sessions) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request, now time.Time) error {
	cookie, err := r.Cookie(sessionCookie)
	if err == nil && cookie.Value != "" {
		if err := s.Store.DeleteSession(ctx, hashToken(cookie.Value), now); err != nil {
			return err
		}
	}
	s.clearCookie(w, sessionCookie)
	s.clearCookie(w, csrfCookie)
	return nil
}

// CSRFValid compares the X-CSRF-Token header with the token derived from the
// session cookie.
func (s *Sessions) CSRFValid(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	header := r.Header.Get("X-CSRF-Token")
	if header == "" {
		return false
	}
	expected := s.Box.HMAC("csrf:" + cookie.Value)
	return subtle.ConstantTimeCompare([]byte(header), []byte(expected)) == 1
}

// CurrentSessionID returns the hashed session identifier from the request.
func (s *Sessions) CurrentSessionID(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return ""
	}
	return hashToken(cookie.Value)
}

// Clear writes anonymous cookies.
func (s *Sessions) Clear(w http.ResponseWriter) {
	s.clearCookie(w, sessionCookie)
	s.clearCookie(w, csrfCookie)
}

// State cookie management.

// LoginState is encrypted into a short-lived cookie.
type LoginState struct {
	State    string    `json:"state"`
	Nonce    string    `json:"nonce"`
	Verifier string    `json:"verifier"`
	ReturnTo string    `json:"return_to,omitempty"`
	Expires  time.Time `json:"expires"`
}

// SetState stores OIDC state in an encrypted cookie.
func (s *Sessions) SetState(w http.ResponseWriter, state LoginState) error {
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	ciphertext, err := s.Box.Encrypt(body)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Value:    ciphertext,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	return nil
}

// ReadState loads and validates the OIDC state cookie.
func (s *Sessions) ReadState(r *http.Request) (LoginState, error) {
	cookie, err := r.Cookie(stateCookie)
	if err != nil || cookie.Value == "" {
		return LoginState{}, errors.New("OIDC state cookie is missing")
	}
	body, err := s.Box.Decrypt(cookie.Value)
	if err != nil {
		return LoginState{}, errors.New("OIDC state cookie is invalid")
	}
	var state LoginState
	if err := json.Unmarshal(body, &state); err != nil {
		return LoginState{}, errors.New("OIDC state cookie is invalid")
	}
	if state.Expires.Before(time.Now()) {
		return LoginState{}, errors.New("OIDC state expired")
	}
	return state, nil
}

// ClearState removes the OIDC state cookie.
func (s *Sessions) ClearState(w http.ResponseWriter) {
	s.clearCookiePath(w, stateCookie, "/api/v1/auth")
}

// NewLoginState builds random state, nonce, and PKCE values.
func NewLoginState(ttl time.Duration) (LoginState, error) {
	state, err := randomToken()
	if err != nil {
		return LoginState{}, err
	}
	nonce, err := randomToken()
	if err != nil {
		return LoginState{}, err
	}
	verifier, err := randomToken()
	if err != nil {
		return LoginState{}, err
	}
	return LoginState{
		State:    state,
		Nonce:    nonce,
		Verifier: verifier,
		Expires:  time.Now().Add(ttl),
	}, nil
}

func (s *Sessions) setCookie(w http.ResponseWriter, name, value string, expires time.Time, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: httpOnly,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Sessions) clearCookie(w http.ResponseWriter, name string) {
	s.clearCookiePath(w, name, "/")
}

func (s *Sessions) clearCookiePath(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	body := make([]byte, 32)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

// SessionIP returns the effective client address for storage.
func SessionIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if r.RemoteAddr == "" {
		return "unknown"
	}
	return r.RemoteAddr
}
