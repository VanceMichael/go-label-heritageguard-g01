package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/conservation"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

func (a *API) registerArtifact(w http.ResponseWriter, r *http.Request) {
	var input conservation.RegisterArtifactInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	result, err := a.Conservation.Register(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", result.Artifact.ID)
	writeJSON(w, http.StatusCreated, DataResponse{Data: result})
}

func (a *API) listArtifacts(w http.ResponseWriter, r *http.Request) {
	principal, err := service.PrincipalFrom(r.Context())
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	page, err := parsePage(r)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	items, total, err := a.Artifacts.ListArtifacts(r.Context(), principal.TenantID, page, r.URL.Query().Get("status"))
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	page = page.Normalize()
	writeJSON(w, http.StatusOK, ListResponse{Data: items, Limit: page.Limit, Offset: page.Offset, Total: total})
}

func (a *API) getArtifact(w http.ResponseWriter, r *http.Request) {
	principal, err := service.PrincipalFrom(r.Context())
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	artifact, err := a.Artifacts.GetArtifact(r.Context(), principal.TenantID, r.PathValue("id"))
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: artifact})
}

func (a *API) recordCondition(w http.ResponseWriter, r *http.Request) {
	var input conservation.ConditionInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	input.ArtifactID = r.PathValue("id")
	report, err := a.Conservation.RecordCondition(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", report.ID)
	writeJSON(w, http.StatusCreated, DataResponse{Data: report})
}

func (a *API) releaseAssessment(w http.ResponseWriter, r *http.Request) {
	artifact, err := a.Conservation.ReleaseAssessment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: artifact})
}

func (a *API) openQuarantine(w http.ResponseWriter, r *http.Request) {
	var input conservation.QuarantineInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	input.ArtifactID = r.PathValue("id")
	item, err := a.Conservation.OpenQuarantine(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", item.ID)
	writeJSON(w, http.StatusCreated, DataResponse{Data: item})
}

func (a *API) draftTreatment(w http.ResponseWriter, r *http.Request) {
	var input conservation.TreatmentInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	item, err := a.Conservation.DraftTreatment(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", item.ID)
	writeJSON(w, http.StatusCreated, DataResponse{Data: item})
}

func (a *API) advanceTreatment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		To          domain.TreatmentStatus `json:"to"`
		EvidenceURI string                 `json:"evidence_uri"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	item, err := a.Conservation.AdvanceTreatment(r.Context(), r.PathValue("id"), input.To, input.EvidenceURI)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: item})
}

func parsePage(r *http.Request) (domain.Page, error) {
	page := domain.Page{}
	for name, target := range map[string]*int{"limit": &page.Limit, "offset": &page.Offset} {
		raw := strings.TrimSpace(r.URL.Query().Get(name))
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return domain.Page{}, domain.FieldError{Field: name, Message: "must be a non-negative integer"}
		}
		*target = value
	}
	return page, nil
}
