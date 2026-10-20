package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.deps.Store.ListUsers(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	view := make([]userDTO, 0, len(users))
	for _, user := range users {
		view = append(view, toUserDTO(user))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": view})
}

func (a *API) setUserRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Role != "viewer" && input.Role != "member" && input.Role != "admin" {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid role", "Role must be viewer, member, or admin.")
		return
	}
	userID := chi.URLParam(r, "userID")
	target, err := a.deps.Store.GetUser(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if target.Role == "admin" && input.Role != "admin" {
		admins, err := a.deps.Store.CountAdmins(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if admins <= 1 {
			writeProblem(w, http.StatusConflict, "Last admin", "Promote another admin before changing this role.")
			return
		}
	}
	if err := a.deps.Store.SetUserRole(r.Context(), userID, input.Role, now()); err != nil {
		writeServiceError(w, err)
		return
	}
	actor, _ := userFrom(r.Context())
	a.audit(r, actor, "user.role", "user", userID, mustJSON(map[string]string{"role": target.Role}), mustJSON(map[string]string{"role": input.Role}))
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (a *API) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	keep := ""
	if actor, ok := userFrom(r.Context()); ok && actor.ID == userID {
		keep = a.deps.Sessions.CurrentSessionID(r)
	}
	if err := a.deps.Store.RevokeUserSessions(r.Context(), userID, keep, now()); err != nil {
		writeServiceError(w, err)
		return
	}
	actor, _ := userFrom(r.Context())
	a.audit(r, actor, "user.sessions.revoke", "user", userID, "", "")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := a.deps.Store.ListAudit(r.Context(), parseLimit(r, 200, 500))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toAuditDTOs(entries)})
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := a.deps.Store.ListJobs(r.Context(), parseLimit(r, 50, 200))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toJobDTOs(jobs)})
}

func (a *API) retryNotification(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "notificationID")
	notification, err := a.deps.Store.GetNotification(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if notification.State == "sent" || notification.State == "pending" || notification.State == "sending" {
		writeProblem(w, http.StatusConflict, "Cannot retry", "Only failed, suppressed, or canceled notifications can be retried.")
		return
	}
	if err := a.deps.Store.RequeueNotification(r.Context(), id, now()); err != nil {
		writeServiceError(w, err)
		return
	}
	actor, _ := userFrom(r.Context())
	a.audit(r, actor, "notification.retry", "notification", id, mustJSON(map[string]string{"state": notification.State}), "")
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": true})
}
