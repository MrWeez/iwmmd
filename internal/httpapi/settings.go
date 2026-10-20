package httpapi

import (
	"net/http"
	"net/mail"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/notify"
	"github.com/mrweez/iwmmd/internal/settings"
)

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	snap, err := a.deps.Settings.Get(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"smtp": map[string]any{
			"host":           snap.SMTP.Host,
			"port":           snap.SMTP.Port,
			"tls_mode":       snap.SMTP.TLSMode,
			"username":       snap.SMTP.Username,
			"from":           snap.SMTP.From,
			"password_set":   snap.SMTP.PasswordSet,
			"managed_by_env": snap.SMTP.ManagedByEnv,
		},
		"alerts": map[string]any{
			"timezone":       snap.Alerts.Timezone,
			"default_time":   snap.Alerts.DefaultTime,
			"paused_until":   snap.Alerts.PausedUntil,
			"managed_by_env": snap.Alerts.ManagedByEnv,
		},
		"retention": map[string]any{
			"raw_days":           snap.Retention.RawDays,
			"checks_days":        snap.Retention.ChecksDays,
			"notifications_days": snap.Retention.NotificationsDays,
			"managed_by_env":     snap.Retention.ManagedByEnv,
		},
		"check": map[string]any{
			"default_interval":  snap.Check.DefaultIntervalSeconds,
			"manual_rate_limit": snap.Check.ManualRateLimitSeconds,
			"managed_by_env":    snap.Check.ManagedByEnv,
		},
	})
}

func (a *API) saveSMTP(w http.ResponseWriter, r *http.Request) {
	snap, err := a.deps.Settings.Get(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if snap.SMTP.ManagedByEnv {
		writeProblem(w, http.StatusConflict, "Managed by environment", "SMTP settings are controlled by environment variables.")
		return
	}
	var input settings.SMTPInput
	if !decodeJSON(w, r, &input) {
		return
	}
	user, _ := userFrom(r.Context())
	now := now()
	if err := a.deps.Settings.SaveSMTP(r.Context(), input, user.ID, now); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid SMTP settings", err.Error())
		return
	}
	a.audit(r, user, "settings.smtp", "settings", "smtp", "", mustJSON(map[string]any{"host": input.Host, "port": input.Port, "from": input.From}))
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (a *API) testSMTP(w http.ResponseWriter, r *http.Request) {
	var input struct {
		To []string `json:"to"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if a.deps.Demo != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
		return
	}
	snap, err := a.deps.Settings.Get(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if snap.SMTP.Host == "" || snap.SMTP.From == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "SMTP is not configured", "Save SMTP settings before sending a test.")
		return
	}
	recipients := input.To
	if len(recipients) == 0 {
		recipients = []string{snap.SMTP.From}
	}
	for i, recipient := range recipients {
		address, err := mail.ParseAddress(recipient)
		if err != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "Invalid recipient", "One or more recipients is invalid.")
			return
		}
		recipients[i] = address.Address
	}
	payload := alert.Payload{
		Event:   "test",
		Kind:    "system",
		Domain:  "example.com",
		Message: "This is a test email from IWMMD.",
	}
	if err := notify.SendTestEmail(r.Context(), snap.SMTP, recipients, payload); err != nil {
		writeProblem(w, http.StatusBadGateway, "Test email failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (a *API) saveAlerts(w http.ResponseWriter, r *http.Request) {
	snap, err := a.deps.Settings.Get(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if snap.Alerts.ManagedByEnv {
		writeProblem(w, http.StatusConflict, "Managed by environment", "Alert settings are controlled by environment variables.")
		return
	}
	var input struct {
		Timezone    string     `json:"timezone"`
		DefaultTime string     `json:"default_time"`
		PausedUntil *time.Time `json:"paused_until"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user, _ := userFrom(r.Context())
	now := now()
	if err := a.deps.Settings.SaveAlerts(r.Context(), settings.AlertsInput{
		Timezone: input.Timezone, DefaultTime: input.DefaultTime, PausedUntil: input.PausedUntil,
	}, user.ID, now); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid alert settings", err.Error())
		return
	}
	a.audit(r, user, "settings.alerts", "settings", "alerts", "", mustJSON(input))
	if err := a.deps.Alerts.ReconcileSettings(r.Context(), now); err != nil {
		a.logger().Error("reconcile alert settings", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (a *API) saveRetention(w http.ResponseWriter, r *http.Request) {
	snap, err := a.deps.Settings.Get(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if snap.Retention.ManagedByEnv {
		writeProblem(w, http.StatusConflict, "Managed by environment", "Retention settings are controlled by environment variables.")
		return
	}
	var input settings.Retention
	if !decodeJSON(w, r, &input) {
		return
	}
	user, _ := userFrom(r.Context())
	if err := a.deps.Settings.SaveRetention(r.Context(), input, user.ID, now()); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid retention settings", err.Error())
		return
	}
	a.audit(r, user, "settings.retention", "settings", "retention", "", mustJSON(input))
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (a *API) saveCheck(w http.ResponseWriter, r *http.Request) {
	snap, err := a.deps.Settings.Get(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if snap.Check.ManagedByEnv {
		writeProblem(w, http.StatusConflict, "Managed by environment", "Check settings are controlled by environment variables.")
		return
	}
	var input settings.Check
	if !decodeJSON(w, r, &input) {
		return
	}
	user, _ := userFrom(r.Context())
	if err := a.deps.Settings.SaveCheck(r.Context(), input, user.ID, now()); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid check settings", err.Error())
		return
	}
	a.audit(r, user, "settings.check", "settings", "check", "", mustJSON(input))
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
