package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"user-service/db"
	"user-service/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm/clause"
)

// jsonError writes a standardized JSON error payload {error, code, ...extra}
func writeJSONError(w http.ResponseWriter, status int, message string, extra map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]any{"error": message, "code": status}
	for k, v := range extra {
		if k != "error" && k != "code" {
			payload[k] = v
		}
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func HandleUserCreate(w http.ResponseWriter, r *http.Request) {
	// Signup should be public, no JWT required
	if r.Method != http.MethodPost {
		slog.Warn("invalid method for /user", "method", r.Method)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
		return
	}
	var req struct {
		UserName  string `json:"user_name"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		// Role intentionally ignored for security; public signup cannot set privileged roles
		Role     string  `json:"role"`
		GoogleID *string `json:"google_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("invalid user create request", "error", err)
		writeJSONError(w, http.StatusBadRequest, "invalid request", nil)
		return
	}
	normUser := strings.ToUpper(req.UserName)
	normEmail := strings.ToUpper(req.Email)
	var existing db.User
	if err := db.DB.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		slog.Warn("duplicate email", "email", req.Email)
		writeJSONError(w, http.StatusConflict, "user with this email already exists", nil)
		return
	}
	if err := db.DB.Where("user_name = ?", req.UserName).First(&existing).Error; err == nil {
		slog.Warn("duplicate username", "user_name", req.UserName)
		writeJSONError(w, http.StatusConflict, "user with this username already exists", nil)
		return
	}

	var passwordHash *string
	// Only hash password if provided (not for Google OAuth users)
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			slog.Error("password hash error", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "password hash error", nil)
			return
		}
		ph := string(hash)
		passwordHash = &ph
	} else if req.GoogleID == nil || *req.GoogleID == "" {
		// If no password and no GoogleID, reject the request
		slog.Warn("no password or google id provided")
		writeJSONError(w, http.StatusBadRequest, "password or googleid required", nil)
		return
	}
	// Force role to standard user regardless of client-provided value to prevent privilege escalation
	if req.Role != "" && req.Role != "user" {
		slog.Warn("attempted elevated role on signup ignored", "requested_role", req.Role, "email", req.Email)
	}
	role := "user"
	user := db.User{
		ID:                 uuid.New(),
		UserName:           req.UserName,
		NormalizedUserName: normUser,
		Email:              req.Email,
		NormalizedEmail:    normEmail,
		FirstName:          req.FirstName,
		LastName:           req.LastName,
		Role:               role,
		EmailConfirmed:     req.GoogleID != nil, // Google users are email confirmed
		PasswordHash:       passwordHash,
		GoogleID:           req.GoogleID,
		TwoFactorEnabled:   false,
		LockoutEnabled:     false,
		AccessFailedCount:  0,
	}
	if err := db.DB.Create(&user).Error; err != nil {
		slog.Error("db error on user create", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}
	slog.Info("user created", "id", user.ID, "email", user.Email)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func authorizeUserOrAdmin(r *http.Request, userID string) bool {
	claims := middleware.GetClaims(r)
	if claims == nil {
		return false
	}
	// Only allow if sub matches userID or role is admin
	slog.Debug("authorizing user", "requester", claims.Sub, "role", claims.Role, "target_user", userID)
	return claims.Role == "admin" || claims.Sub == userID
}

func authorizeAnyValidJWT(r *http.Request) bool {
	claims := middleware.GetClaims(r)
	return claims != nil
}

func authorizeResidentOrAdmin(r *http.Request) bool {
	claims := middleware.GetClaims(r)
	if claims == nil {
		return false
	}
	return claims.Role == "resident" || claims.Role == "admin"
}

// HandleUsersList returns a paginated list of users (resident/admin only)
func HandleUsersList(w http.ResponseWriter, r *http.Request) {
	if !authorizeResidentOrAdmin(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden", nil)
		return
	}

	// Parse pagination params
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	pageStr := r.URL.Query().Get("page")
	sizeStr := r.URL.Query().Get("page_size")
	page := 1
	size := 20
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
		if s > 100 {
			s = 100
		}
		size = s
	}
	offset := (page - 1) * size

	query := db.DB.Model(&db.User{})
	if q != "" {
		like := "%" + strings.ToLower(escapeLike(q)) + "%"
		query = query.Where(clause.Like{Column: clause.Expr{SQL: "LOWER(email)"}, Value: like}).
			Or(clause.Like{Column: clause.Expr{SQL: "LOWER(user_name)"}, Value: like}).
			Or(clause.Like{Column: clause.Expr{SQL: "LOWER(first_name)"}, Value: like}).
			Or(clause.Like{Column: clause.Expr{SQL: "LOWER(last_name)"}, Value: like})
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		slog.Error("count users failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}

	var users []db.User
	if err := query.Order("created_at DESC").Offset(offset).Limit(size).Find(&users).Error; err != nil {
		slog.Error("list users failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}

	type userOut struct {
		ID                string    `json:"id"`
		UserName          string    `json:"user_name"`
		Email             string    `json:"email"`
		FirstName         string    `json:"first_name"`
		LastName          string    `json:"last_name"`
		Role              string    `json:"role"`
		EmailConfirmed    bool      `json:"email_confirmed"`
		TwoFactorEnabled  bool      `json:"two_factor_enabled"`
		TwoFactorType     string    `json:"two_factor_type"`
		ProfilePictureURL *string   `json:"profile_picture_url"`
		GoogleID          *string   `json:"google_id"`
		LockoutEnabled    bool      `json:"lockout_enabled"`
		CreatedAt         time.Time `json:"created_at"`
		UpdatedAt         time.Time `json:"updated_at"`
	}

	out := make([]userOut, 0, len(users))
	for _, u := range users {
		out = append(out, userOut{
			ID:                u.ID.String(),
			UserName:          u.UserName,
			Email:             u.Email,
			FirstName:         u.FirstName,
			LastName:          u.LastName,
			Role:              u.Role,
			EmailConfirmed:    u.EmailConfirmed,
			TwoFactorEnabled:  u.TwoFactorEnabled,
			TwoFactorType:     u.TwoFactorType,
			ProfilePictureURL: u.ProfilePictureURL,
			GoogleID:          u.GoogleID,
			LockoutEnabled:    u.LockoutEnabled,
			CreatedAt:         u.CreatedAt,
			UpdatedAt:         u.UpdatedAt,
		})
	}

	totalPages := (total + int64(size) - 1) / int64(size)
	resp := map[string]any{
		"users":       out,
		"page":        page,
		"page_size":   size,
		"total":       total,
		"total_pages": totalPages,
		"query":       q,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func HandleUserGet(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user id", nil)
		return
	}

	if !authorizeAnyValidJWT(r) {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	var user db.User
	if err := db.DB.Where("id = ?", id).First(&user).Error; err != nil {
		writeJSONError(w, http.StatusNotFound, "user not found", nil)
		return
	}

	slog.Debug("user profile picture url", "user_id", user.ID, "url", user.ProfilePictureURL)

	resp := struct {
		ID                string `json:"id"`
		UserName          string `json:"user_name"`
		Email             string `json:"email"`
		FirstName         string `json:"first_name"`
		LastName          string `json:"last_name"`
		Role              string `json:"role"`
		EmailConfirmed    bool   `json:"email_confirmed"`
		TwoFactorEnabled  bool   `json:"two_factor_enabled"`
		TwoFactorType     string `json:"two_factor_type"`
		ProfilePictureURL string `json:"profile_picture_url"`
	}{
		ID:               user.ID.String(),
		UserName:         user.UserName,
		Email:            user.Email,
		FirstName:        user.FirstName,
		LastName:         user.LastName,
		Role:             user.Role,
		EmailConfirmed:   user.EmailConfirmed,
		TwoFactorEnabled: user.TwoFactorEnabled,
		TwoFactorType:    user.TwoFactorType,
		ProfilePictureURL: func() string {
			if user.ProfilePictureURL != nil {
				return *user.ProfilePictureURL
			}
			return ""
		}(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func HandleUserGetByEmail(w http.ResponseWriter, r *http.Request) {
	// This endpoint now supports two modes:
	// 1) Single user lookup by email or google_id (auth required)
	// 2) Paginated list (resident/admin only) with optional q/page/page_size
	email := r.URL.Query().Get("email")
	googleID := r.URL.Query().Get("google_id")

	if email != "" || googleID != "" {
		var user db.User
		var err error
		if googleID != "" {
			err = db.DB.Where("google_id = ?", googleID).First(&user).Error
		} else {
			err = db.DB.Where("email = ?", email).First(&user).Error
		}
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "user not found", nil)
			return
		}
		resp := struct {
			ID                string  `json:"id"`
			UserName          string  `json:"user_name"`
			Email             string  `json:"email"`
			FirstName         string  `json:"first_name"`
			LastName          string  `json:"last_name"`
			Role              string  `json:"role"`
			EmailConfirmed    bool    `json:"email_confirmed"`
			TwoFactorEnabled  bool    `json:"two_factor_enabled"`
			TwoFactorType     string  `json:"two_factor_type"`
			ProfilePictureURL *string `json:"profile_picture_url"`
			GoogleID          *string `json:"google_id"`
		}{
			ID:                user.ID.String(),
			UserName:          user.UserName,
			Email:             user.Email,
			FirstName:         user.FirstName,
			LastName:          user.LastName,
			Role:              user.Role,
			EmailConfirmed:    user.EmailConfirmed,
			TwoFactorEnabled:  user.TwoFactorEnabled,
			TwoFactorType:     user.TwoFactorType,
			ProfilePictureURL: user.ProfilePictureURL,
			GoogleID:          user.GoogleID,
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Otherwise, treat as list with pagination/search; restrict to resident/admin
	if !authorizeResidentOrAdmin(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	// Reuse list logic
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	pageStr := r.URL.Query().Get("page")
	sizeStr := r.URL.Query().Get("page_size")
	page := 1
	size := 20
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
		if s > 100 {
			s = 100
		}
		size = s
	}
	offset := (page - 1) * size

	query := db.DB.Model(&db.User{})
	if q != "" {
		like := "%" + strings.ToLower(escapeLike(q)) + "%"
		query = query.Where(clause.Like{Column: clause.Expr{SQL: "LOWER(email)"}, Value: like}).
			Or(clause.Like{Column: clause.Expr{SQL: "LOWER(user_name)"}, Value: like}).
			Or(clause.Like{Column: clause.Expr{SQL: "LOWER(first_name)"}, Value: like}).
			Or(clause.Like{Column: clause.Expr{SQL: "LOWER(last_name)"}, Value: like})
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		slog.Error("count users failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}

	var users []db.User
	if err := query.Order("created_at DESC").Offset(offset).Limit(size).Find(&users).Error; err != nil {
		slog.Error("list users failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}

	type userOut struct {
		ID                string    `json:"id"`
		UserName          string    `json:"user_name"`
		Email             string    `json:"email"`
		FirstName         string    `json:"first_name"`
		LastName          string    `json:"last_name"`
		Role              string    `json:"role"`
		EmailConfirmed    bool      `json:"email_confirmed"`
		TwoFactorEnabled  bool      `json:"two_factor_enabled"`
		TwoFactorType     string    `json:"two_factor_type"`
		ProfilePictureURL *string   `json:"profile_picture_url"`
		GoogleID          *string   `json:"google_id"`
		LockoutEnabled    bool      `json:"lockout_enabled"`
		CreatedAt         time.Time `json:"created_at"`
		UpdatedAt         time.Time `json:"updated_at"`
	}

	out := make([]userOut, 0, len(users))
	for _, u := range users {
		out = append(out, userOut{
			ID:                u.ID.String(),
			UserName:          u.UserName,
			Email:             u.Email,
			FirstName:         u.FirstName,
			LastName:          u.LastName,
			Role:              u.Role,
			EmailConfirmed:    u.EmailConfirmed,
			TwoFactorEnabled:  u.TwoFactorEnabled,
			TwoFactorType:     u.TwoFactorType,
			ProfilePictureURL: u.ProfilePictureURL,
			GoogleID:          u.GoogleID,
			LockoutEnabled:    u.LockoutEnabled,
			CreatedAt:         u.CreatedAt,
			UpdatedAt:         u.UpdatedAt,
		})
	}

	totalPages := (total + int64(size) - 1) / int64(size)
	resp := map[string]any{
		"users":       out,
		"page":        page,
		"page_size":   size,
		"total":       total,
		"total_pages": totalPages,
		"query":       q,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func HandleLockout(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		slog.Error("invalid uuid for lockout", "id", idStr)
		writeJSONError(w, http.StatusBadRequest, "invalid user id", nil)
		return
	}
	claims := middleware.GetClaims(r)
	if claims == nil || claims.Role != "admin" {
		writeJSONError(w, http.StatusForbidden, "only admins can lockout accounts", nil)
		return
	}
	var req struct {
		Lock bool `json:"lock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("invalid lockout request", "error", err)
		writeJSONError(w, http.StatusBadRequest, "invalid request", nil)
		return
	}
	if err := db.DB.Model(&db.User{}).Where("id = ?", id).Update("lockout_enabled", req.Lock).Error; err != nil {
		slog.Error("db error on lockout", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}
	slog.Info("lockout updated", "user_id", idStr, "lock", req.Lock)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "lockout updated", "lock": req.Lock})
}

func HandleUserDelete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user id", nil)
		return
	}
	if !authorizeUserOrAdmin(r, id.String()) {
		writeJSONError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	if err := db.DB.Delete(&db.User{}, "id = ?", id).Error; err != nil {
		slog.Error("db error on user delete", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}
	slog.Info("user deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func HandleUserPatch(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handle user patch", "headers", r.Header)
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user id", nil)
		return
	}
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("invalid request body", "error", err)
		writeJSONError(w, http.StatusBadRequest, "invalid request", nil)
		return
	}
	// If fields other than role are being changed, enforce self-or-admin authorization
	changingNonRole := false
	for k := range req {
		if k != "role" {
			changingNonRole = true
			break
		}
	}
	if changingNonRole && !authorizeUserOrAdmin(r, id.String()) {
		slog.Debug("forbidden user patch", "target_user", idStr)
		writeJSONError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	// Only allow certain fields to be patched
	allowed := map[string]bool{
		"email_confirmed":     true,
		"two_factor_enabled":  true,
		"two_factor_type":     true,
		"two_factor_secret":   true,
		"lockout_enabled":     true,
		"access_failed_count": true,
		"password":            true, // allow password patch
		"first_name":          true,
		"last_name":           true,
		"role":                true, // will check admin below
		"profile_picture_url": true, // allow profile picture URL updates
		"google_id":           true, // allow linking Google ID
	}
	update := make(map[string]interface{})
	for k, v := range req {
		if allowed[k] {
			switch k {
			case "password":
				hash, err := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("%v", v)), bcrypt.DefaultCost)
				if err != nil {
					writeJSONError(w, http.StatusInternalServerError, "password hash error", nil)
					return
				}
				update["password_hash"] = string(hash)
			case "role":
				// Allow admins to set any role, residents can grant resident role to others
				claims := middleware.GetClaims(r)
				if claims == nil {
					writeJSONError(w, http.StatusUnauthorized, "unauthorized", nil)
					return
				}
				newRole, ok := v.(string)
				if !ok {
					writeJSONError(w, http.StatusBadRequest, "invalid role value", nil)
					return
				}
				// Normalize
				newRole = strings.ToLower(newRole)
				validRoles := map[string]bool{"user": true, "resident": true, "admin": true}
				if !validRoles[newRole] {
					writeJSONError(w, http.StatusBadRequest, "unsupported role", nil)
					return
				}
				if claims.Role == "admin" {
					update[k] = newRole
					break
				}
				if claims.Role == "resident" {
					// Residents can only grant resident role, and not modify admin accounts
					if newRole != "resident" {
						writeJSONError(w, http.StatusForbidden, "residents can only grant resident role", nil)
						return
					}
					// Fetch target user to ensure not admin
					var target db.User
					if err := db.DB.Where("id = ?", id).First(&target).Error; err != nil {
						writeJSONError(w, http.StatusNotFound, "user not found", nil)
						return
					}
					if target.Role == "admin" {
						writeJSONError(w, http.StatusForbidden, "cannot modify admin role", nil)
						return
					}
					update[k] = newRole
					break
				}
				// Others cannot change roles
				writeJSONError(w, http.StatusForbidden, "insufficient permissions to change role", nil)
				return
			default:
				update[k] = v
			}
		}
	}
	if len(update) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no valid fields to update", nil)
		return
	}
	if err := db.DB.Model(&db.User{}).Where("id = ?", id).Updates(update).Error; err != nil {
		slog.Error("db error on user patch", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "db error", nil)
		return
	}
	slog.Info("user patched", "id", id, "fields", update)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "user updated"})
}

func HandleUserValidate(w http.ResponseWriter, r *http.Request) {
	// Login/validate should be public, no JWT required
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request", nil)
		return
	}
	var user db.User
	if err := db.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid credentials", nil)
		return
	}
	if user.LockoutEnabled {
		writeJSONError(w, http.StatusLocked, "account locked", map[string]any{"reason": "admin_lock"})
		return
	}
	if user.PasswordHash == nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid credentials", nil)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid credentials", nil)
		return
	}
	resp := struct {
		ID                string `json:"id"`
		UserName          string `json:"user_name"`
		Email             string `json:"email"`
		FirstName         string `json:"first_name"`
		LastName          string `json:"last_name"`
		Role              string `json:"role"`
		TwoFactorEnabled  bool   `json:"two_factor_enabled"`
		TwoFactorType     string `json:"two_factor_type"`
		ProfilePictureURL string `json:"profile_picture_url"`
	}{
		ID:               user.ID.String(),
		UserName:         user.UserName,
		Email:            user.Email,
		FirstName:        user.FirstName,
		LastName:         user.LastName,
		Role:             user.Role,
		TwoFactorEnabled: user.TwoFactorEnabled,
		TwoFactorType:    user.TwoFactorType,
		ProfilePictureURL: func() string {
			if user.ProfilePictureURL != nil {
				return *user.ProfilePictureURL
			}
			return ""
		}(),
	}
	json.NewEncoder(w).Encode(resp)
}

// escapeLike escapes %, _ and \ for use in a LIKE/ILIKE pattern where we use ESCAPE '\\'
func escapeLike(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '%', '_', '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
