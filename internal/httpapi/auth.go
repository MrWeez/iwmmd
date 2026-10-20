package httpapi

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/mrweez/iwmmd/internal/auth"
	"github.com/mrweez/iwmmd/internal/store"
)

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	if a.deps.Demo != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"user":       toUserDTO(a.deps.Demo.User),
			"csrf_token": a.deps.Demo.CSRF,
		})
		return
	}
	if a.deps.Sessions == nil || a.deps.OIDC == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Authentication is not configured",
			"Configure OIDC before signing in.")
		return
	}
	user, csrf, err := a.deps.Sessions.CurrentUser(r, now())
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeProblem(w, http.StatusUnauthorized, "Authentication required", "Sign in to continue.")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "Session error", "The session could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":       toUserDTO(user),
		"csrf_token": csrf,
	})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if a.deps.Demo != nil {
		http.Redirect(w, r, safeReturnTo(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	if a.deps.OIDC == nil || a.deps.Sessions == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Authentication is not configured",
			"Set the OIDC issuer, client ID, client secret, and redirect URL before signing in.")
		return
	}
	state, err := auth.NewLoginState(10 * time.Minute)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Login failed", "Could not create a login state.")
		return
	}
	state.ReturnTo = safeReturnTo(r.URL.Query().Get("next"))
	if err := a.deps.Sessions.SetState(w, state); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Login failed", "Could not store the login state.")
		return
	}
	http.Redirect(w, r, a.deps.OIDC.LoginURL(state.State, state.Nonce, state.Verifier), http.StatusFound)
}

func (a *API) callback(w http.ResponseWriter, r *http.Request) {
	if a.deps.Demo != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if a.deps.OIDC == nil || a.deps.Sessions == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Authentication is not configured",
			"OIDC is not configured.")
		return
	}
	a.deps.Sessions.ClearState(w)
	state, err := a.deps.Sessions.ReadState(r)
	if err != nil {
		redirectLoginError(w, r, "state")
		return
	}
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		redirectLoginError(w, r, "provider")
		return
	}
	code := r.URL.Query().Get("code")
	returnedState := r.URL.Query().Get("state")
	if code == "" || subtle.ConstantTimeCompare([]byte(returnedState), []byte(state.State)) != 1 {
		redirectLoginError(w, r, "state")
		return
	}
	identity, err := a.deps.OIDC.Exchange(r.Context(), code, state.Verifier, state.Nonce)
	if err != nil {
		a.logger().Warn("OIDC exchange failed", "error", err)
		redirectLoginError(w, r, "exchange")
		return
	}
	at := now()
	user, err := a.deps.Store.UpsertUser(r.Context(), store.User{
		Issuer:      a.deps.Config.OIDC.Issuer,
		Subject:     identity.Subject,
		Email:       identity.Email,
		DisplayName: identity.DisplayName,
		Role:        identity.Role,
	}, at)
	if err != nil {
		a.logger().Error("store user", "error", err)
		redirectLoginError(w, r, "storage")
		return
	}
	if _, err := a.deps.Sessions.Create(r.Context(), w, user, auth.SessionIP(r), r.UserAgent(), at); err != nil {
		a.logger().Error("create session", "error", err)
		redirectLoginError(w, r, "session")
		return
	}
	a.audit(r, user, "auth.login", "user", user.ID, "", "")
	http.Redirect(w, r, safeReturnTo(state.ReturnTo), http.StatusFound)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if a.deps.Demo != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	user, _ := userFrom(r.Context())
	if err := a.deps.Sessions.Logout(r.Context(), w, r, now()); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Logout failed", "The session could not be revoked.")
		return
	}
	a.audit(r, user, "auth.logout", "user", user.ID, "", "")
	w.WriteHeader(http.StatusNoContent)
}

func redirectLoginError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?error="+url.QueryEscape(code), http.StatusFound)
}

// Access log is intentionally handled in the router.
func (a *API) logger() *slog.Logger {
	if a.deps.Logger != nil {
		return a.deps.Logger
	}
	return slog.Default()
}
