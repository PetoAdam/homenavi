package profile

import (
	"encoding/json"
	"log"
	"net/http"

	"auth-service/internal/models/responses"
	"auth-service/internal/services"
	"auth-service/pkg/errors"
)

type ProfileHandler struct {
	authService *services.AuthService
	userService *services.UserService
}

func NewProfileHandler(authService *services.AuthService, userService *services.UserService) *ProfileHandler {
	return &ProfileHandler{
		authService: authService,
		userService: userService,
	}
}

func (h *ProfileHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		errors.WriteError(w, errors.Unauthorized("missing or invalid authorization header"))
		return
	}

	token := authHeader[7:]
	userID, err := h.authService.ExtractUserIDFromToken(token)
	if err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid token"))
		return
	}

	// Get user
	user, err := h.userService.GetUser(userID)
	if err != nil {
		errors.WriteError(w, errors.NotFound("user not found"))
		return
	}

	response := responses.UserResponse{
		ID:                user.ID,
		UserName:          user.UserName,
		Email:             user.Email,
		FirstName:         user.FirstName,
		LastName:          user.LastName,
		Role:              user.Role,
		EmailConfirmed:    user.EmailConfirmed,
		TwoFactorEnabled:  user.TwoFactorEnabled,
		TwoFactorType:     user.TwoFactorType,
		ProfilePictureURL: user.ProfilePictureURL,
		CreatedAt:         user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:         user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type AvatarHandler struct {
	authService            *services.AuthService
	userService            *services.UserService
	profilePictureService  *services.ProfilePictureService
}

func NewAvatarHandler(authService *services.AuthService, userService *services.UserService, profilePictureService *services.ProfilePictureService) *AvatarHandler {
	return &AvatarHandler{
		authService:           authService,
		userService:           userService,
		profilePictureService: profilePictureService,
	}
}

func (h *AvatarHandler) HandleGenerateAvatar(w http.ResponseWriter, r *http.Request) {
	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		errors.WriteError(w, errors.Unauthorized("missing or invalid authorization header"))
		return
	}

	token := authHeader[7:]
	userID, err := h.authService.ExtractUserIDFromToken(token)
	if err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid token"))
		return
	}

	// Call profile picture service to generate avatar
	avatarURL, err := h.profilePictureService.GenerateAvatar(userID)
	if err != nil {
		log.Printf("[ERROR] Failed to generate avatar for user %s: %v", userID, err)
		errors.WriteError(w, errors.InternalServerError("failed to generate avatar", err))
		return
	}

	// Update user's profile picture URL
	updates := map[string]interface{}{
		"profile_picture_url": avatarURL,
	}

	if err := h.userService.UpdateUser(userID, updates, token); err != nil {
		log.Printf("[ERROR] Failed to update user profile picture URL: %v", err)
		errors.WriteError(w, errors.InternalServerError("failed to update profile picture", err))
		return
	}

	log.Printf("[INFO] Avatar generated for user: %s", userID)

	response := responses.ProfilePictureResponse{
		Success: true,
		URL:     avatarURL,
		Message: "Avatar generated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *AvatarHandler) HandleUploadProfilePicture(w http.ResponseWriter, r *http.Request) {
	// Extract JWT token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		errors.WriteError(w, errors.Unauthorized("missing or invalid authorization header"))
		return
	}

	token := authHeader[7:]
	userID, err := h.authService.ExtractUserIDFromToken(token)
	if err != nil {
		errors.WriteError(w, errors.Unauthorized("invalid token"))
		return
	}

	// Parse multipart form (10MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		errors.WriteError(w, errors.BadRequest("invalid multipart form"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		errors.WriteError(w, errors.BadRequest("missing file in upload"))
		return
	}
	defer file.Close()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/gif" && contentType != "image/webp" {
		errors.WriteError(w, errors.BadRequest("invalid file type. Only JPEG, PNG, GIF, and WebP are allowed"))
		return
	}

	// Validate file size (5MB max)
	if header.Size > 5<<20 {
		errors.WriteError(w, errors.BadRequest("file size too large. Maximum 5MB allowed"))
		return
	}

	// Call profile picture service to upload and process the image
	pictureURL, err := h.profilePictureService.UploadProfilePicture(userID, file, header)
	if err != nil {
		log.Printf("[ERROR] Failed to upload profile picture for user %s: %v", userID, err)
		errors.WriteError(w, errors.InternalServerError("failed to upload profile picture", err))
		return
	}

	// Update user's profile picture URL
	updates := map[string]interface{}{
		"profile_picture_url": pictureURL,
	}

	if err := h.userService.UpdateUser(userID, updates, token); err != nil {
		log.Printf("[ERROR] Failed to update user profile picture URL: %v", err)
		errors.WriteError(w, errors.InternalServerError("failed to update profile picture", err))
		return
	}

	log.Printf("[INFO] Profile picture uploaded for user: %s", userID)

	response := responses.ProfilePictureResponse{
		Success: true,
		URL:     pictureURL,
		Message: "Profile picture uploaded successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
