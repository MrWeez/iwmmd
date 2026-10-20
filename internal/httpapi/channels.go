package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mrweez/iwmmd/internal/notify"
	"github.com/mrweez/iwmmd/internal/store"
)

func (a *API) listChannels(w http.ResponseWriter, r *http.Request) {
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	views, err := a.deps.Channels.List(r.Context(), includeArchived)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (a *API) createChannel(w http.ResponseWriter, r *http.Request) {
	var input notify.CreateChannelInput
	if !decodeJSON(w, r, &input) {
		return
	}
	user, _ := userFrom(r.Context())
	channel, err := a.deps.Channels.Create(r.Context(), input, user.ID, now())
	if err != nil {
		if err == store.ErrConflict {
			writeProblem(w, http.StatusConflict, "Channel already exists", "A channel with this name already exists.")
			return
		}
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid channel", err.Error())
		return
	}
	a.audit(r, user, "channel.create", "channel", channel.ID, "", mustJSON(map[string]any{"name": channel.Name, "type": channel.Type}))
	view, err := a.deps.Channels.View(channel)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (a *API) updateChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "channelID")
	var input notify.UpdateChannelInput
	if !decodeJSON(w, r, &input) {
		return
	}
	user, _ := userFrom(r.Context())
	channel, err := a.deps.Channels.Update(r.Context(), id, input, now())
	if err != nil {
		if err == store.ErrConflict {
			writeProblem(w, http.StatusConflict, "Channel already exists", "A channel with this name already exists.")
			return
		}
		if err == store.ErrNotFound {
			writeServiceError(w, err)
			return
		}
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid channel", err.Error())
		return
	}
	a.audit(r, user, "channel.update", "channel", id, "", mustJSON(map[string]any{"name": channel.Name, "enabled": channel.Enabled}))
	view, err := a.deps.Channels.View(channel)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *API) archiveChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "channelID")
	if err := a.deps.Store.ArchiveChannel(r.Context(), id, now()); err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "channel.archive", "channel", id, "", "")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) testChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "channelID")
	if err := a.deps.Channels.Test(r.Context(), id); err != nil {
		writeProblem(w, http.StatusBadGateway, "Test delivery failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (a *API) muteChannel(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Until  *time.Time `json:"until"`
		Reason string     `json:"reason,omitempty"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	id := chi.URLParam(r, "channelID")
	if input.Until != nil && !input.Until.After(now()) {
		input.Until = nil
	}
	if err := a.deps.Store.MuteChannel(r.Context(), id, input.Until, strings.TrimSpace(input.Reason), now()); err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "channel.mute", "channel", id, "", mustJSON(map[string]any{"until": input.Until, "reason": input.Reason}))
	channel, err := a.deps.Store.GetChannel(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	view, err := a.deps.Channels.View(channel)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *API) unmuteChannel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "channelID")
	if err := a.deps.Store.MuteChannel(r.Context(), id, nil, "", now()); err != nil {
		writeServiceError(w, err)
		return
	}
	user, _ := userFrom(r.Context())
	a.audit(r, user, "channel.unmute", "channel", id, "", "")
	channel, err := a.deps.Store.GetChannel(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	view, err := a.deps.Channels.View(channel)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
