package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/exhibition"
)

func (a *API) reserveDisplayCase(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ArtifactID string `json:"artifact_id"`
		Duration   string `json:"duration"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	duration, err := parseDuration(input.Duration, 30*time.Minute)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	item, err := a.Exhibition.Reserve(r.Context(), exhibition.ReservationInput{
		ArtifactID: input.ArtifactID, DisplayCaseID: r.PathValue("id"), Duration: duration,
	})
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: item})
}

func (a *API) activateInstallation(w http.ResponseWriter, r *http.Request) {
	var input exhibition.InstallationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	input.DisplayCaseID = r.PathValue("id")
	item, err := a.Exhibition.Activate(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", item.ID)
	writeJSON(w, http.StatusCreated, DataResponse{Data: item})
}

func (a *API) recordReading(w http.ResponseWriter, r *http.Request) {
	var input exhibition.ReadingInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	item, err := a.Exhibition.RecordReading(r.Context(), strings.TrimSpace(r.Header.Get("X-Tenant-ID")), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", item.ID)
	writeJSON(w, http.StatusAccepted, DataResponse{Data: item})
}

func (a *API) assessEnvironment(w http.ResponseWriter, r *http.Request) {
	window, err := parseDuration(r.URL.Query().Get("window"), 15*time.Minute)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	item, err := a.Exhibition.Assess(r.Context(), r.PathValue("id"), window)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: item})
}

func (a *API) advanceIncident(w http.ResponseWriter, r *http.Request) {
	var input struct {
		To         domain.IncidentStatus `json:"to"`
		Remediated bool                  `json:"remediated"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	item, err := a.Exhibition.AdvanceIncident(r.Context(), r.PathValue("id"), input.To, input.Remediated)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: item})
}
