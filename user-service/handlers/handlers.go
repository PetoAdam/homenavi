package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"user-service/db"
	"user-service/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm/clause"
)

func HandleUserCreate(w http.ResponseWriter, r *http.Request) {
	// Signup should be public, no JWT required
	// TODO: Add input validation for production?
	if r.Method != http.MethodPost {
		log.Printf("[WARN] Invalid method for /user: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		UserName  string  `json:"user_name"`
		Email     string  `json:"email"`
		Password  string  `json:"password"`
		FirstName string  `json:"first_name"`
		LastName  string  `json:"last_name"`
		Role      string  `json:"role"`
		GoogleID  *string `json:"google_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid user create request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	normUser := strings.ToUpper(req.UserName)
	normEmail := strings.ToUpper(req.Email)
	var existing db.User
	if err := db.DB.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		log.Printf("[ERROR] Duplicate email: %s", req.Email)
		http.Error(w, "User with this email already exists", http.StatusConflict)
		return
	}
	if err := db.DB.Where("user_name = ?", req.UserName).First(&existing).Error; err == nil {
		log.Printf("[ERROR] Duplicate username: %s", req.UserName)
		http.Error(w, "User with this username already exists", http.StatusConflict)
		return
	}

	var passwordHash *string
	// Only hash password if provided (not for Google OAuth users)
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("[ERROR] Password hash error: %v", err)
			http.Error(w, "Password hash error", http.StatusInternalServerError)
			return
		}
		ph := string(hash)
		passwordHash = &ph
	} else if req.GoogleID == nil || *req.GoogleID == "" {
		// If no password and no GoogleID, reject the request
	log.Printf("[ERROR] No password or GoogleID provided")
	http.Error(w, "Password or GoogleID required", http.StatusBadRequest)
		return
	}
	role := req.Role
	if role == "" {
		role = "user"
	}
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
		log.Printf("[ERROR] DB error on user create: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	log.Printf("[INFO] User created: id=%s, email=%s", user.ID, user.Email)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func authorizeUserOrAdmin(r *http.Request, userID string) bool {
	claims := middleware.GetClaims(r)
	if claims == nil {
		return false
	}
	// Only allow if sub matches userID or role is admin
	log.Printf("[DEBUG] Authorizing user %s with role %s for user ID %s", claims.Sub, claims.Role, userID)
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
		http.Error(w, "Forbidden", http.StatusForbidden)
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
		log.Printf("[ERROR] Count users failed: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	var users []db.User
	if err := query.Order("created_at DESC").Offset(offset).Limit(size).Find(&users).Error; err != nil {
		log.Printf("[ERROR] List users failed: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	type userOut struct {
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
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	if !authorizeAnyValidJWT(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user db.User
	if err := db.DB.Where("id = ?", id).First(&user).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	log.Printf("[DEBUG] User %s profile picture URL: %v", user.ID, user.ProfilePictureURL)

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
			http.Error(w, "User not found", http.StatusNotFound)
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
		http.Error(w, "Forbidden", http.StatusForbidden)
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
		log.Printf("[ERROR] Count users failed: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	var users []db.User
	if err := query.Order("created_at DESC").Offset(offset).Limit(size).Find(&users).Error; err != nil {
		log.Printf("[ERROR] List users failed: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	type userOut struct {
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
		log.Printf("[ERROR] Invalid UUID for lockout: %s", idStr)
		http.Error(w, "Invalid user id", http.StatusBadRequest)
		return
	}
	claims := middleware.GetClaims(r)
	if claims == nil || claims.Role != "admin" {
		http.Error(w, "Only admins can lockout accounts", http.StatusForbidden)
		return
	}
	var req struct {
		Lock bool `json:"lock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid lockout request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if err := db.DB.Model(&db.User{}).Where("id = ?", id).Update("lockout_enabled", req.Lock).Error; err != nil {
		log.Printf("[ERROR] DB error on lockout: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	log.Printf("[INFO] Lockout updated for user_id=%s, lock=%v", idStr, req.Lock)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Lockout updated"))
}

func HandleUserDelete(w http.ResponseWriter, r *http.Request) {
	// TODO: Enforce JWT authentication and admin/self authorization for production
	// For development, allow delete without JWT check
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid user id", http.StatusBadRequest)
		return
	}
	if !authorizeUserOrAdmin(r, id.String()) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if err := db.DB.Delete(&db.User{}, "id = ?", id).Error; err != nil {
		log.Printf("[ERROR] DB error on user delete: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	log.Printf("[INFO] User deleted: id=%s", id)
	w.WriteHeader(http.StatusNoContent)
}

func HandleUserPatch(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] HandleUserPatch called. Headers: %v", r.Header)
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid user id", http.StatusBadRequest)
		return
	}
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[DEBUG] Invalid request body: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
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
		log.Printf("[DEBUG] Forbidden non-role change: not authorized for user id %s", idStr)
		http.Error(w, "Forbidden", http.StatusForbidden)
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
		    http.Error(w, "Password hash error", http.StatusInternalServerError)
					return
				}
				update["password_hash"] = string(hash)
			case "role":
				// Allow admins to set any role, residents can grant resident role to others
				claims := middleware.GetClaims(r)
				if claims == nil {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				newRole, ok := v.(string)
				if !ok {
					http.Error(w, "Invalid role value", http.StatusBadRequest)
					return
				}
				// Normalize
				newRole = strings.ToLower(newRole)
				validRoles := map[string]bool{"user": true, "resident": true, "admin": true}
				if !validRoles[newRole] {
					http.Error(w, "Unsupported role", http.StatusBadRequest)
					return
				}
				if claims.Role == "admin" {
					update[k] = newRole
					break
				}
				if claims.Role == "resident" {
					// Residents can only grant resident role, and not modify admin accounts
					if newRole != "resident" {
						http.Error(w, "Residents can only grant resident role", http.StatusForbidden)
						return
					}
					// Fetch target user to ensure not admin
					var target db.User
					if err := db.DB.Where("id = ?", id).First(&target).Error; err != nil {
						http.Error(w, "User not found", http.StatusNotFound)
						return
					}
					if target.Role == "admin" {
						http.Error(w, "Cannot modify admin role", http.StatusForbidden)
						return
					}
					update[k] = newRole
					break
				}
				// Others cannot change roles
				http.Error(w, "Insufficient permissions to change role", http.StatusForbidden)
				return
			default:
				update[k] = v
			}
		}
	}
	if len(update) == 0 {
		http.Error(w, "No valid fields to update", http.StatusBadRequest)
		return
	}
	if err := db.DB.Model(&db.User{}).Where("id = ?", id).Updates(update).Error; err != nil {
		log.Printf("[ERROR] DB error on user patch: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	log.Printf("[INFO] User patched: id=%s fields=%v", id, update)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("User updated"))
}

func HandleUserValidate(w http.ResponseWriter, r *http.Request) {
	// Login/validate should be public, no JWT required
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	var user db.User
	if err := db.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	if user.LockoutEnabled {
		http.Error(w, "Account is locked", http.StatusForbidden)
		return
	}
	if user.PasswordHash == nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
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
