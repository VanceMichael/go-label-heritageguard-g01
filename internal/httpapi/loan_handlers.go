package httpapi

import (
	"net/http"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/loan"
)

func (a *API) createLoan(w http.ResponseWriter, r *http.Request) {
	var input loan.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	item, err := a.Loans.Create(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", item.ID)
	writeJSON(w, http.StatusCreated, DataResponse{Data: item})
}

func (a *API) submitLoan(w http.ResponseWriter, r *http.Request) {
	item, err := a.Loans.Submit(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: item})
}

func (a *API) approveLoan(w http.ResponseWriter, r *http.Request) {
	item, err := a.Loans.Approve(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: item})
}

func (a *API) recordCustody(w http.ResponseWriter, r *http.Request) {
	var input loan.CustodyInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	input.LoanID = r.PathValue("id")
	item, err := a.Loans.RecordCustody(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", item.ID)
	writeJSON(w, http.StatusCreated, DataResponse{Data: item})
}
