package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"user-service/db"
	"user-service/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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
		UserName  string `json:"user_name"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Role      string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Invalid user create request: %v", err)
		http.Error(w, "Invalid request", 400)
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
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[ERROR] Password hash error: %v", err)
		http.Error(w, "Password hash error", 500)
		return
	}
	ph := string(hash)
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
		EmailConfirmed:     false,
		PasswordHash:       &ph,
		TwoFactorEnabled:   false,
		LockoutEnabled:     false,
		AccessFailedCount:  0,
	}
	if err := db.DB.Create(&user).Error; err != nil {
		log.Printf("[ERROR] DB error on user create: %v", err)
		http.Error(w, "DB error", 500)
		return
	}
	log.Printf("[INFO] User created: id=%s, email=%s", user.ID, user.Email)
	w.WriteHeader(201)
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
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Missing email", http.StatusBadRequest)
		return
	}
	var user db.User
	if err := db.DB.Where("email = ?", email).First(&user).Error; err != nil {
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
	}
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
		http.Error(w, "Invalid user id", 400)
		return
	}
	if !authorizeUserOrAdmin(r, id.String()) {
		http.Error(w, "Forbidden", 403)
		return
	}
	if err := db.DB.Delete(&db.User{}, "id = ?", id).Error; err != nil {
		log.Printf("[ERROR] DB error on user delete: %v", err)
		http.Error(w, "DB error", 500)
		return
	}
	log.Printf("[INFO] User deleted: id=%s", id)
	w.WriteHeader(204)
}

func HandleUserPatch(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] HandleUserPatch called. Headers: %v", r.Header)
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid user id", 400)
		return
	}
	if !authorizeUserOrAdmin(r, id.String()) {
		log.Printf("[DEBUG] Forbidden: not authorized for user id %s", idStr)
		http.Error(w, "Forbidden", 403)
		return
	}
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[DEBUG] Invalid request body: %v", err)
		http.Error(w, "Invalid request", 400)
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
	}
	update := make(map[string]interface{})
	for k, v := range req {
		if allowed[k] {
			switch k {
			case "password":
				hash, err := bcrypt.GenerateFromPassword([]byte(fmt.Sprintf("%v", v)), bcrypt.DefaultCost)
				if err != nil {
					http.Error(w, "Password hash error", 500)
					return
				}
				update["password_hash"] = string(hash)
			case "role":
				// Only allow admins to change role
				claims := middleware.GetClaims(r)
				if claims == nil || claims.Role != "admin" {
					http.Error(w, "Only admins can change user role", 403)
					return
				}
				update[k] = v
			default:
				update[k] = v
			}
		}
	}
	if len(update) == 0 {
		http.Error(w, "No valid fields to update", 400)
		return
	}
	if err := db.DB.Model(&db.User{}).Where("id = ?", id).Updates(update).Error; err != nil {
		log.Printf("[ERROR] DB error on user patch: %v", err)
		http.Error(w, "DB error", 500)
		return
	}
	log.Printf("[INFO] User patched: id=%s fields=%v", id, update)
	w.WriteHeader(200)
	w.Write([]byte("User updated"))
}

func HandleUserValidate(w http.ResponseWriter, r *http.Request) {
	// Login/validate should be public, no JWT required
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	var user db.User
	if err := db.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		http.Error(w, "Invalid credentials", 401)
		return
	}
	if user.LockoutEnabled {
		http.Error(w, "Account is locked", http.StatusForbidden)
		return
	}
	if user.PasswordHash == nil {
		http.Error(w, "Invalid credentials", 401)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		http.Error(w, "Invalid credentials", 401)
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
