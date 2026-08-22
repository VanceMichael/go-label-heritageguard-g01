package httpapi

import (
	"net/http"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/auth"
	"github.com/VanceMichael/go-base-heritageguard-g01/internal/service"
)

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input auth.LoginInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	result, err := a.Auth.Login(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	writeJSON(w, http.StatusOK, DataResponse{Data: result})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	principal, err := service.PrincipalFrom(r.Context())
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	if err := a.Auth.Logout(r.Context(), principal); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var input auth.CreateUserInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	user, err := a.Auth.CreateUser(r.Context(), input)
	if err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.Header().Set("X-Resource-ID", user.ID)
	writeJSON(w, http.StatusCreated, DataResponse{Data: user})
}

func (a *API) deactivateUser(w http.ResponseWriter, r *http.Request) {
	if err := a.Auth.DeactivateUser(r.Context(), r.PathValue("id")); err != nil {
		writeError(r.Context(), a.Logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
