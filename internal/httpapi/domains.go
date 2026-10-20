package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mrweez/iwmmd/internal/domain"
	"github.com/mrweez/iwmmd/internal/store"
)

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.deps.Store.Stats(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"total":          stats.Total,
		"active":         stats.Active,
		"expired":        stats.Expired,
		"expiring_7":     stats.Expiring7,
		"expiring_30":    stats.Expiring30,
		"expiring_90":    stats.Expiring90,
		"unknown":        stats.Unknown,
		"paused":         stats.Paused,
		"archived":       stats.Archived,
		"open_incidents": stats.OpenIncidents,
	})
}

func (a *API) listDomains(w http.ResponseWriter, r *http.Request) {
	filter := store.DomainFilter{
		Search: strings.TrimSpace(r.URL.Query().Get("search")),
		State:  r.URL.Query().Get("state"),
		Expiry: r.URL.Query().Get("expiry"),
		Limit:  parseLimit(r, 50, 200),
		Offset: parseOffset(r),
	}
	items, total, err := a.deps.Store.ListDomains(r.Context(), filter)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	now := now()
	view := make([]domainDTO, 0, len(items))
	for _, item := range items {
		view = append(view, toDomainDTO(item, now))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  view,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (a *API) createDomain(w http.ResponseWriter, r *http.Request) {
	var input domain.CreateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	user, _ := userFrom(r.Context())
	d, err := a.deps.Domains.Create(r.Context(), input, user.ID, now())
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeProblem(w, http.StatusConflict, "Domain already exists", "This domain is already monitored.")
			return
		}
		if isValidationError(err) {
			writeProblem(w, http.StatusUnprocessableEntity, "Invalid domain", err.Error())
			return
		}
		writeServiceError(w, err)
		return
	}
	a.audit(r, user, "domain.create", "domain", d.ID, "", mustJSON(toDomainDTO(d, now())))
	writeJSON(w, http.StatusCreated, toDomainDTO(d, now()))
}

func (a *API) getDomain(w http.ResponseWriter, r *http.Request) {
	d, err := a.deps.Store.GetDomain(r.Context(), chi.URLParam(r, "domainID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDomainDTO(d, now()))
}

func (a *API) updateDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "domainID")
	before, err := a.deps.Store.GetDomain(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	var input domain.UpdateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	d, err := a.deps.Domains.Update(r.Context(), id, input, now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "domain.update", "domain", id, mustJSON(toDomainDTO(before, now())), mustJSON(toDomainDTO(d, now())))
	writeJSON(w, http.StatusOK, toDomainDTO(d, now()))
}

func (a *API) archiveDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "domainID")
	before, err := a.deps.Store.GetDomain(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err := a.deps.Domains.Archive(r.Context(), id, now()); err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "domain.archive", "domain", id, mustJSON(toDomainDTO(before, now())), "")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) restoreDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "domainID")
	before, err := a.deps.Store.GetDomain(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	d, err := a.deps.Domains.Restore(r.Context(), id, now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "domain.restore", "domain", id, mustJSON(toDomainDTO(before, now())), mustJSON(toDomainDTO(d, now())))
	writeJSON(w, http.StatusOK, toDomainDTO(d, now()))
}

func (a *API) deleteDomainPermanent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "domainID")
	before, err := a.deps.Store.GetDomain(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err := a.deps.Domains.DeletePermanently(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "domain.delete", "domain", id, mustJSON(toDomainDTO(before, now())), "")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) checkDomain(w http.ResponseWriter, r *http.Request) {
	queued, err := a.deps.Domains.ManualCheck(r.Context(), chi.URLParam(r, "domainID"), now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"queued": queued})
}

func (a *API) muteDomain(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Until  *time.Time `json:"until"`
		Reason string     `json:"reason,omitempty"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	id := chi.URLParam(r, "domainID")
	d, err := a.deps.Domains.Mute(r.Context(), id, input.Until, strings.TrimSpace(input.Reason), now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "domain.mute", "domain", id, "", mustJSON(map[string]any{"until": input.Until, "reason": input.Reason}))
	writeJSON(w, http.StatusOK, toDomainDTO(d, now()))
}

func (a *API) unmuteDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "domainID")
	d, err := a.deps.Domains.Mute(r.Context(), id, nil, "", now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "domain.unmute", "domain", id, "", "")
	writeJSON(w, http.StatusOK, toDomainDTO(d, now()))
}

func (a *API) listChecks(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainID")
	if _, err := a.deps.Store.GetDomain(r.Context(), domainID); err != nil {
		writeServiceError(w, err)
		return
	}
	checks, err := a.deps.Store.ListChecks(r.Context(), domainID, parseLimit(r, 50, 200))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	view := make([]checkDTO, 0, len(checks))
	for _, check := range checks {
		view = append(view, toCheckDTO(check))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": view})
}

func (a *API) rawCheck(w http.ResponseWriter, r *http.Request) {
	body, contentType, err := a.deps.Store.GetRawCheckForDomain(r.Context(),
		chi.URLParam(r, "domainID"), chi.URLParam(r, "checkID"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (a *API) domainNotifications(w http.ResponseWriter, r *http.Request) {
	items, err := a.deps.Store.ListNotifications(r.Context(), chi.URLParam(r, "domainID"), "", "", parseLimit(r, 50, 200), false)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toNotificationDTOs(items)})
}

func (a *API) domainIncidents(w http.ResponseWriter, r *http.Request) {
	items, err := a.deps.Store.ListIncidents(r.Context(), chi.URLParam(r, "domainID"), "", parseLimit(r, 50, 200))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toIncidentDTOs(items)})
}

func (a *API) importDomains(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid upload", "Send a CSV file in the file field.")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid upload", "CSV file field is required.")
		return
	}
	defer file.Close()
	reader := csv.NewReader(io.LimitReader(file, 2<<20))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid CSV", err.Error())
		return
	}
	user, _ := userFrom(r.Context())
	imported := 0
	var failures []map[string]string
	for i, record := range records {
		if len(record) == 0 {
			continue
		}
		name := strings.TrimSpace(record[0])
		if i == 0 && (strings.EqualFold(name, "domain") || strings.EqualFold(name, "name")) {
			continue
		}
		notes := ""
		if len(record) > 1 {
			notes = strings.TrimSpace(record[1])
		}
		if _, err := a.deps.Domains.Create(r.Context(), domain.CreateInput{Name: name, Notes: notes}, user.ID, now()); err != nil {
			failures = append(failures, map[string]string{"domain": name, "error": err.Error()})
			continue
		}
		imported++
	}
	a.audit(r, user, "domain.import", "domain", "", "", mustJSON(map[string]any{"imported": imported, "failed": len(failures)}))
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "failed": failures})
}

func (a *API) listIncidents(w http.ResponseWriter, r *http.Request) {
	items, err := a.deps.Store.ListIncidents(r.Context(), r.URL.Query().Get("domain_id"), r.URL.Query().Get("state"), parseLimit(r, 100, 200))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toIncidentDTOs(items)})
}

func (a *API) listNotifications(w http.ResponseWriter, r *http.Request) {
	items, err := a.deps.Store.ListNotifications(r.Context(), r.URL.Query().Get("domain_id"),
		r.URL.Query().Get("channel_id"), r.URL.Query().Get("state"), parseLimit(r, 50, 200),
		r.URL.Query().Get("exclude_archived") == "true")
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toNotificationDTOs(items)})
}

func toIncidentDTOs(items []store.Incident) []incidentDTO {
	out := make([]incidentDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toIncidentDTO(item))
	}
	return out
}

func toNotificationDTOs(items []store.Notification) []notificationDTO {
	out := make([]notificationDTO, 0, len(items))
	for _, item := range items {
		out = append(out, toNotificationDTO(item))
	}
	return out
}

func mustJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(body)
}

func isValidationError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "domain name") || strings.Contains(message, "idna") ||
		strings.Contains(message, "ip address")
}

func intParam(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(key))
	return n
}
