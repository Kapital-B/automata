package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/auth"
)

type registerBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) register(w http.ResponseWriter, r *http.Request) {
	if h.AuthSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	var body registerBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	id, err := h.AuthSvc.Register(r.Context(), body.Email, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrEmailTaken) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email taken"})
			return
		}
		h.Log.Warn("register", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	pair, err := h.AuthSvc.IssueTokens(r.Context(), id)
	if err != nil {
		h.Log.Error("tokens after register", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id": id.String(), "access_token": pair.AccessToken, "refresh_token": pair.RefreshToken,
	})
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) loginPassword(w http.ResponseWriter, r *http.Request) {
	if h.AuthSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	var body loginBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	pair, err := h.AuthSvc.LoginPassword(r.Context(), body.Email, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}
		h.Log.Error("login", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": pair.AccessToken, "refresh_token": pair.RefreshToken,
	})
}

func (h *Handlers) authMicrosoftStart(w http.ResponseWriter, r *http.Request) {
	if h.AuthSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	u, st, err := h.AuthSvc.StartMicrosoftLogin(r.Context())
	if err != nil {
		h.Log.Warn("ms auth start", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorization_url": u, "state": st})
}

func (h *Handlers) authMicrosoftCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		h.redirectAuthError(w, r, oauthErrorCode(errParam, q.Get("error_subcode")))
		return
	}
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		h.redirectAuthError(w, r, "invalid_state")
		return
	}
	if h.AuthSvc == nil {
		h.redirectAuthError(w, r, "token_exchange_failed")
		return
	}
	pair, err := h.AuthSvc.CompleteMicrosoftLogin(r.Context(), code, state)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidOAuthState) {
			h.redirectAuthError(w, r, "invalid_state")
			return
		}
		h.Log.Error("ms auth callback", "err", err)
		h.redirectAuthError(w, r, "token_exchange_failed")
		return
	}
	h.redirectAuthSuccess(w, r, pair)
}

func (h *Handlers) authGoogleStart(w http.ResponseWriter, r *http.Request) {
	if h.AuthSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	u, st, err := h.AuthSvc.StartGoogleLogin(r.Context())
	if err != nil {
		h.Log.Warn("google auth start", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorization_url": u, "state": st})
}

func (h *Handlers) authGoogleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		h.redirectAuthError(w, r, errParam)
		return
	}
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		h.redirectAuthError(w, r, "invalid_state")
		return
	}
	if h.AuthSvc == nil {
		h.redirectAuthError(w, r, "token_exchange_failed")
		return
	}
	pair, err := h.AuthSvc.CompleteGoogleLogin(r.Context(), code, state)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidOAuthState) {
			h.redirectAuthError(w, r, "invalid_state")
			return
		}
		h.Log.Error("google auth callback", "err", err)
		h.redirectAuthError(w, r, "token_exchange_failed")
		return
	}
	h.redirectAuthSuccess(w, r, pair)
}

type refreshBody struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handlers) authRefresh(w http.ResponseWriter, r *http.Request) {
	if h.AuthSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auth not configured"})
		return
	}
	var body refreshBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	pair, err := h.AuthSvc.RefreshTokens(r.Context(), body.RefreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefreshToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
			return
		}
		h.Log.Error("refresh", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": pair.AccessToken, "refresh_token": pair.RefreshToken,
	})
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	email, err := h.Users.GetUserByID(r.Context(), uid)
	if err != nil {
		h.Log.Error("me", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	resp := map[string]any{"user_id": uid.String(), "email": email}
	if orgID, err := h.Users.GetHomeOrganisationID(r.Context(), uid); err == nil {
		resp["home_organisation_id"] = orgID.String()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) redirectAuthSuccess(w http.ResponseWriter, r *http.Request, pair auth.TokenPair) {
	base := strings.TrimRight(h.Dashboard, "/") + h.AuthSuccessPath
	u, err := url.Parse(base)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "bad redirect config"})
		return
	}
	// Fragment avoids sending tokens to the server as Referer on subsequent requests.
	u.Fragment = "access_token=" + url.QueryEscape(pair.AccessToken) + "&refresh_token=" + url.QueryEscape(pair.RefreshToken)
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (h *Handlers) redirectAuthError(w http.ResponseWriter, r *http.Request, code string) {
	target := strings.TrimRight(h.Dashboard, "/") + h.AuthErrorPath + "?code=" + url.QueryEscape(code)
	http.Redirect(w, r, target, http.StatusFound)
}
