package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/mrweez/iwmmd/internal/auth"
	"github.com/mrweez/iwmmd/internal/store"
)

type contextKey int

const (
	userContextKey contextKey = iota
	csrfContextKey
)

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Demo != nil {
			ctx := context.WithValue(r.Context(), userContextKey, a.deps.Demo.User)
			ctx = context.WithValue(ctx, csrfContextKey, a.deps.Demo.CSRF)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if a.deps.Sessions == nil || a.deps.OIDC == nil {
			writeProblem(w, http.StatusServiceUnavailable, "Authentication is not configured",
				"Configure OIDC before using the application.")
			return
		}
		user, csrf, err := a.deps.Sessions.CurrentUser(r, now())
		if err != nil {
			if err == auth.ErrUnauthorized {
				writeProblem(w, http.StatusUnauthorized, "Authentication required", "Sign in to continue.")
				return
			}
			writeProblem(w, http.StatusInternalServerError, "Session error", "The session could not be loaded.")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		ctx = context.WithValue(ctx, csrfContextKey, csrf)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) requireRole(minimum string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := userFrom(r.Context())
			if !ok {
				writeProblem(w, http.StatusUnauthorized, "Authentication required", "Sign in to continue.")
				return
			}
			if roleRank(user.Role) < roleRank(minimum) {
				writeProblem(w, http.StatusForbidden, "Forbidden", "Your role does not allow this action.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (a *API) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if a.deps.Demo != nil {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(a.deps.Demo.CSRF)) != 1 {
				writeProblem(w, http.StatusForbidden, "CSRF validation failed", "Refresh the page and try again.")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if a.deps.Sessions == nil || !a.deps.Sessions.CSRFValid(r) {
			writeProblem(w, http.StatusForbidden, "CSRF validation failed", "Refresh the page and try again.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func userFrom(ctx context.Context) (store.User, bool) {
	user, ok := ctx.Value(userContextKey).(store.User)
	return user, ok
}

func csrfFrom(ctx context.Context) string {
	value, _ := ctx.Value(csrfContextKey).(string)
	return value
}

func roleRank(role string) int {
	switch role {
	case "admin":
		return 2
	case "member":
		return 1
	default:
		return 0
	}
}

func safeReturnTo(value string) string {
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/"
	}
	return value
}
