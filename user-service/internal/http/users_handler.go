package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/PetoAdam/homenavi/user-service/internal/auth"
	"github.com/PetoAdam/homenavi/user-service/internal/users"
	"github.com/go-chi/chi/v5"
)

// UsersHandler handles HTTP requests for user operations.
type UsersHandler struct {
	service *users.Service
}

func NewUsersHandler(service *users.Service) *UsersHandler {
	return &UsersHandler{service: service}
}

func (h *UsersHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserName          string  `json:"user_name"`
		Email             string  `json:"email"`
		Password          string  `json:"password"`
		FirstName         string  `json:"first_name"`
		LastName          string  `json:"last_name"`
		Role              string  `json:"role"`
		GoogleID          *string `json:"google_id"`
		ProfilePictureURL *string `json:"profile_picture_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request", nil)
		return
	}
	if req.Role != "" && req.Role != "user" {
		slog.Warn("attempted elevated role on signup ignored", "requested_role", req.Role, "email", req.Email)
	}
	user, err := h.service.Create(r.Context(), users.CreateInput{
		UserName:          req.UserName,
		Email:             req.Email,
		Password:          req.Password,
		FirstName:         req.FirstName,
		LastName:          req.LastName,
		GoogleID:          req.GoogleID,
		ProfilePictureURL: req.ProfilePictureURL,
	})
	if err != nil {
		switch {
		case errors.Is(err, users.ErrDuplicateEmail), errors.Is(err, users.ErrDuplicateUserName):
			writeJSONError(w, http.StatusConflict, err.Error(), nil)
		case errors.Is(err, users.ErrPasswordOrGoogleIDRequired):
			writeJSONError(w, http.StatusBadRequest, err.Error(), nil)
		default:
			writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		}
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h *UsersHandler) HandleValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request", nil)
		return
	}
	user, err := h.service.Validate(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, users.ErrInvalidCredentials):
			writeJSONError(w, http.StatusUnauthorized, "invalid credentials", nil)
		case errors.Is(err, users.ErrAccountLocked):
			writeJSONError(w, http.StatusLocked, "account locked", map[string]any{"reason": "admin_lock"})
		default:
			writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, validateResponse(user))
}

func (h *UsersHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	user, err := h.service.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if strings.Contains(err.Error(), "invalid UUID") {
			writeJSONError(w, http.StatusBadRequest, "invalid user id", nil)
			return
		}
		if errors.Is(err, users.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "user not found", nil)
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}
	writeJSON(w, http.StatusOK, publicUser(user))
}

func (h *UsersHandler) HandleQuery(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	googleID := r.URL.Query().Get("google_id")
	if email != "" || googleID != "" {
		user, err := h.service.Lookup(r.Context(), email, googleID)
		if err != nil {
			if errors.Is(err, users.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "user not found", nil)
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "db error", nil)
			return
		}
		writeJSON(w, http.StatusOK, lookupUser(user))
		return
	}

	actor := actorFromRequest(r)
	if actor.Role != "resident" && actor.Role != "admin" {
		writeJSONError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	page := 1
	size := 20
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	if s, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && s > 0 {
		size = s
	}
	items, total, page, size, err := h.service.List(r.Context(), r.URL.Query().Get("q"), page, size)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, user := range items {
		out = append(out, listUser(user))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users":       out,
		"page":        page,
		"page_size":   size,
		"total":       total,
		"total_pages": (total + int64(size) - 1) / int64(size),
		"query":       strings.TrimSpace(r.URL.Query().Get("q")),
	})
}

func (h *UsersHandler) HandleLockout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lock bool `json:"lock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request", nil)
		return
	}
	if err := h.service.SetLockout(r.Context(), actorFromRequest(r), chi.URLParam(r, "id"), req.Lock); err != nil {
		handleMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "lockout updated", "lock": req.Lock})
}

func (h *UsersHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), actorFromRequest(r), chi.URLParam(r, "id")); err != nil {
		handleMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandler) HandlePatch(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request", nil)
		return
	}
	if err := h.service.Patch(r.Context(), actorFromRequest(r), chi.URLParam(r, "id"), req); err != nil {
		handleMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "user updated"})
}

func handleMutationError(w http.ResponseWriter, err error) {
	switch {
	case strings.Contains(err.Error(), "invalid UUID"):
		writeJSONError(w, http.StatusBadRequest, "invalid user id", nil)
	case errors.Is(err, users.ErrForbidden), errors.Is(err, users.ErrCannotChangeRole), errors.Is(err, users.ErrResidentsGrantResidentOnly), errors.Is(err, users.ErrCannotModifyAdminRole):
		writeJSONError(w, http.StatusForbidden, err.Error(), nil)
	case errors.Is(err, users.ErrUnauthorized):
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", nil)
	case errors.Is(err, users.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "user not found", nil)
	case errors.Is(err, users.ErrNoValidFields), errors.Is(err, users.ErrInvalidRoleValue), errors.Is(err, users.ErrUnsupportedRole):
		writeJSONError(w, http.StatusBadRequest, err.Error(), nil)
	default:
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
	}
}

func actorFromRequest(r *http.Request) users.Actor {
	claims := auth.GetClaims(r)
	if claims == nil {
		return users.Actor{}
	}
	return users.Actor{Subject: claims.Sub, Role: claims.Role}
}

func publicUser(user users.User) map[string]any {
	return map[string]any{
		"id":                  user.ID.String(),
		"user_name":           user.UserName,
		"email":               user.Email,
		"first_name":          user.FirstName,
		"last_name":           user.LastName,
		"role":                user.Role,
		"email_confirmed":     user.EmailConfirmed,
		"two_factor_enabled":  user.TwoFactorEnabled,
		"two_factor_type":     user.TwoFactorType,
		"profile_picture_url": derefString(user.ProfilePictureURL),
	}
}

func validateResponse(user users.User) map[string]any {
	resp := publicUser(user)
	resp["profile_picture_url"] = derefString(user.ProfilePictureURL)
	return resp
}

func lookupUser(user users.User) map[string]any {
	return map[string]any{
		"id":                  user.ID.String(),
		"user_name":           user.UserName,
		"email":               user.Email,
		"first_name":          user.FirstName,
		"last_name":           user.LastName,
		"role":                user.Role,
		"email_confirmed":     user.EmailConfirmed,
		"lockout_enabled":     user.LockoutEnabled,
		"two_factor_enabled":  user.TwoFactorEnabled,
		"two_factor_type":     user.TwoFactorType,
		"profile_picture_url": user.ProfilePictureURL,
		"google_id":           user.GoogleID,
	}
}

func listUser(user users.User) map[string]any {
	return map[string]any{
		"id":                  user.ID.String(),
		"user_name":           user.UserName,
		"email":               user.Email,
		"first_name":          user.FirstName,
		"last_name":           user.LastName,
		"role":                user.Role,
		"email_confirmed":     user.EmailConfirmed,
		"two_factor_enabled":  user.TwoFactorEnabled,
		"two_factor_type":     user.TwoFactorType,
		"profile_picture_url": user.ProfilePictureURL,
		"google_id":           user.GoogleID,
		"lockout_enabled":     user.LockoutEnabled,
		"created_at":          user.CreatedAt,
		"updated_at":          user.UpdatedAt,
	}
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
