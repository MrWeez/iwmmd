package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mrweez/iwmmd/internal/calendar"
	"github.com/mrweez/iwmmd/internal/store"
)

type ruleInput struct {
	Name          string   `json:"name"`
	Scope         string   `json:"scope,omitempty"`
	DomainID      string   `json:"domain_id,omitempty"`
	OffsetValue   int      `json:"offset_value"`
	OffsetUnit    string   `json:"offset_unit"`
	Direction     string   `json:"direction"`
	SendTimeLocal string   `json:"send_time_local"`
	Enabled       bool     `json:"enabled"`
	ChannelIDs    []string `json:"channel_ids"`
}

func (a *API) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := a.deps.Store.ListAlertRules(r.Context(), r.URL.Query().Get("scope"), r.URL.Query().Get("domain_id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	view := make([]ruleDTO, 0, len(rules))
	for _, rule := range rules {
		view = append(view, toRuleDTO(rule))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": view})
}

func (a *API) createRule(w http.ResponseWriter, r *http.Request) {
	var input ruleInput
	if !decodeJSON(w, r, &input) {
		return
	}
	user, _ := userFrom(r.Context())
	if input.Scope == "global" && user.Role != "admin" {
		writeProblem(w, http.StatusForbidden, "Forbidden", "Only admins can create global alert rules.")
		return
	}
	if err := a.validateRuleInput(r, &input); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid alert rule", err.Error())
		return
	}
	now := now()
	rule := store.AlertRule{
		ID:            store.NewAlertRuleID(),
		Name:          strings.TrimSpace(input.Name),
		Scope:         input.Scope,
		DomainID:      input.DomainID,
		OffsetValue:   input.OffsetValue,
		OffsetUnit:    input.OffsetUnit,
		Direction:     input.Direction,
		SendTimeLocal: input.SendTimeLocal,
		Enabled:       input.Enabled,
		CreatedBy:     user.ID,
		CreatedAt:     now,
		UpdatedAt:     now,
		ChannelIDs:    dedupeStrings(input.ChannelIDs),
	}
	if err := a.deps.Store.CreateAlertRule(r.Context(), rule); err != nil {
		writeServiceError(w, err)
		return
	}
	a.audit(r, user, "alert_rule.create", "alert_rule", rule.ID, "", mustJSON(toRuleDTO(rule)))
	if err := a.deps.Alerts.ReconcileRule(r.Context(), rule, now); err != nil {
		a.logger().Error("reconcile alert rule", "rule_id", rule.ID, "error", err)
	}
	writeJSON(w, http.StatusCreated, toRuleDTO(rule))
}

func (a *API) updateRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ruleID")
	existing, err := a.deps.Store.GetAlertRule(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	if existing.Scope == "global" && user.Role != "admin" {
		writeProblem(w, http.StatusForbidden, "Forbidden", "Only admins can edit global alert rules.")
		return
	}
	var input ruleInput
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Scope = existing.Scope
	input.DomainID = existing.DomainID
	if err := a.validateRuleInput(r, &input); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid alert rule", err.Error())
		return
	}
	existing.Name = strings.TrimSpace(input.Name)
	existing.OffsetValue = input.OffsetValue
	existing.OffsetUnit = input.OffsetUnit
	existing.Direction = input.Direction
	existing.SendTimeLocal = input.SendTimeLocal
	existing.Enabled = input.Enabled
	existing.ChannelIDs = dedupeStrings(input.ChannelIDs)
	existing.UpdatedAt = now()
	if err := a.deps.Store.UpdateAlertRule(r.Context(), existing); err != nil {
		writeServiceError(w, err)
		return
	}
	a.audit(r, user, "alert_rule.update", "alert_rule", id, "", mustJSON(toRuleDTO(existing)))
	if err := a.deps.Alerts.ReconcileRule(r.Context(), existing, existing.UpdatedAt); err != nil {
		a.logger().Error("reconcile alert rule", "rule_id", id, "error", err)
	}
	writeJSON(w, http.StatusOK, toRuleDTO(existing))
}

func (a *API) archiveRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "ruleID")
	existing, err := a.deps.Store.GetAlertRule(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	if existing.Scope == "global" && user.Role != "admin" {
		writeProblem(w, http.StatusForbidden, "Forbidden", "Only admins can delete global alert rules.")
		return
	}
	now := now()
	if err := a.deps.Store.ArchiveAlertRule(r.Context(), id, now); err != nil {
		writeServiceError(w, err)
		return
	}
	existing.ArchivedAt = &now
	existing.Enabled = false
	a.audit(r, user, "alert_rule.archive", "alert_rule", id, "", "")
	if err := a.deps.Alerts.ReconcileRule(r.Context(), existing, now); err != nil {
		a.logger().Error("reconcile archived rule", "rule_id", id, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) validateRuleInput(r *http.Request, input *ruleInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 120 {
		return errors.New("name must be between 1 and 120 characters")
	}
	if input.Scope != "global" && input.Scope != "domain" {
		return errors.New("scope must be global or domain")
	}
	if input.Scope == "domain" {
		if input.DomainID == "" {
			return errors.New("domain_id is required for a domain rule")
		}
		d, err := a.deps.Store.GetDomain(r.Context(), input.DomainID)
		if err != nil {
			return errors.New("domain does not exist")
		}
		if d.State == "archived" {
			return errors.New("domain is archived")
		}
	} else {
		input.DomainID = ""
	}
	if input.OffsetValue < 0 {
		return errors.New("offset must be zero or greater")
	}
	if input.OffsetValue == 0 && input.Direction != "before" {
		return errors.New("zero offset is only allowed before expiry")
	}
	if input.OffsetUnit != "days" && input.OffsetUnit != "months" {
		return errors.New("offset unit must be days or months")
	}
	if input.Direction != "before" && input.Direction != "after" {
		return errors.New("direction must be before or after")
	}
	if input.OffsetUnit == "days" && input.OffsetValue > 3650 {
		return errors.New("day offset is too large")
	}
	if input.OffsetUnit == "months" && input.OffsetValue > 120 {
		return errors.New("month offset is too large")
	}
	if input.SendTimeLocal == "" {
		snap, err := a.deps.Settings.Get(r.Context())
		if err != nil {
			return err
		}
		input.SendTimeLocal = snap.Alerts.DefaultTime
	}
	if _, _, err := calendar.ParseSendTime(input.SendTimeLocal); err != nil {
		return err
	}
	if len(input.ChannelIDs) == 0 {
		return errors.New("at least one channel is required")
	}
	seen := map[string]bool{}
	for _, channelID := range input.ChannelIDs {
		if seen[channelID] {
			continue
		}
		seen[channelID] = true
		channel, err := a.deps.Store.GetChannel(r.Context(), channelID)
		if err != nil {
			return errors.New("one or more channels do not exist")
		}
		if channel.ArchivedAt != nil {
			return errors.New("one or more channels are archived")
		}
	}
	return nil
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
