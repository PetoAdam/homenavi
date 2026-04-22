package profile

import (
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"
	"strings"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/errors"
	profiletransport "github.com/PetoAdam/homenavi/auth-service/internal/http/profile/transport"
	usertransport "github.com/PetoAdam/homenavi/auth-service/internal/http/user/transport"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

type ProfileHandler struct {
	authService *authdomain.Service
	userService *clientsinfra.UserClient
}

func NewProfileHandler(authService *authdomain.Service, userService *clientsinfra.UserClient) *ProfileHandler {
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(usertransport.NewUserResponse(user))
}

type AvatarHandler struct {
	authService           *authdomain.Service
	userService           *clientsinfra.UserClient
	profilePictureService *clientsinfra.ProfilePictureClient
}

func NewAvatarHandler(authService *authdomain.Service, userService *clientsinfra.UserClient, profilePictureService *clientsinfra.ProfilePictureClient) *AvatarHandler {
	return &AvatarHandler{
		authService:           authService,
		userService:           userService,
		profilePictureService: profilePictureService,
	}
}

func (h *AvatarHandler) extractAuthenticatedUserID(r *http.Request) (string, string, *errors.AppError) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		return "", "", errors.Unauthorized("missing or invalid authorization header")
	}
	token := authHeader[7:]
	userID, err := h.authService.ExtractUserIDFromToken(token)
	if err != nil {
		return "", "", errors.Unauthorized("invalid token")
	}
	return userID, token, nil
}

func (h *AvatarHandler) HandleGenerateAvatar(w http.ResponseWriter, r *http.Request) {
	userID, token, authErr := h.extractAuthenticatedUserID(r)
	if authErr != nil {
		errors.WriteError(w, authErr)
		return
	}

	avatarURL, err := h.profilePictureService.GenerateAvatar(userID)
	if err != nil {
		slog.Error("failed to generate avatar", "user_id", userID, "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to generate avatar", err))
		return
	}

	// Update user's profile picture URL
	updates := map[string]interface{}{
		"profile_picture_url": avatarURL,
	}

	if err := h.userService.UpdateUser(userID, updates, token); err != nil {
		slog.Error("failed to update user profile picture url", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to update profile picture", err))
		return
	}

	slog.Info("avatar generated", "user_id", userID)

	response := profiletransport.ProfilePictureResponse{
		Success: true,
		URL:     avatarURL,
		Message: "Avatar generated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *AvatarHandler) HandleCreateUploadURL(w http.ResponseWriter, r *http.Request) {
	userID, _, authErr := h.extractAuthenticatedUserID(r)
	if authErr != nil {
		errors.WriteError(w, authErr)
		return
	}

	var payload struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		errors.WriteError(w, errors.BadRequest("invalid request body"))
		return
	}
	payload.Filename = strings.TrimSpace(payload.Filename)
	payload.ContentType = strings.TrimSpace(payload.ContentType)
	if payload.Filename == "" {
		errors.WriteError(w, errors.BadRequest("filename is required"))
		return
	}

	prepared, err := h.profilePictureService.CreateUploadURL(userID, payload.Filename, payload.ContentType)
	if err != nil {
		slog.Error("failed to create upload url", "user_id", userID, "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to create upload url", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profiletransport.ProfilePictureUploadURLResponse{
		Success:   true,
		UploadURL: prepared.UploadURL,
		ObjectKey: prepared.ObjectKey,
		Method:    prepared.Method,
		Headers:   prepared.Headers,
		ExpiresIn: prepared.ExpiresIn,
		Message:   "Profile picture upload URL created successfully",
	})
}

func (h *AvatarHandler) HandleCompleteProfilePictureUpload(w http.ResponseWriter, r *http.Request) {
	userID, token, authErr := h.extractAuthenticatedUserID(r)
	if authErr != nil {
		errors.WriteError(w, authErr)
		return
	}

	var payload struct {
		ObjectKey string `json:"object_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		errors.WriteError(w, errors.BadRequest("invalid request body"))
		return
	}
	payload.ObjectKey = strings.TrimSpace(payload.ObjectKey)
	if payload.ObjectKey == "" {
		errors.WriteError(w, errors.BadRequest("object_key is required"))
		return
	}

	pictureURL, err := h.profilePictureService.CompleteUpload(userID, payload.ObjectKey)
	if err != nil {
		slog.Error("failed to complete profile picture upload", "user_id", userID, "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to complete profile picture upload", err))
		return
	}

	updates := map[string]interface{}{"profile_picture_url": pictureURL}
	if err := h.userService.UpdateUser(userID, updates, token); err != nil {
		slog.Error("failed to update user profile picture url", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to update profile picture", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profiletransport.ProfilePictureResponse{
		Success: true,
		URL:     pictureURL,
		Message: "Profile picture uploaded successfully",
	})
}

func (h *AvatarHandler) HandleUploadProfilePicture(w http.ResponseWriter, r *http.Request) {
	userID, token, authErr := h.extractAuthenticatedUserID(r)
	if authErr != nil {
		errors.WriteError(w, authErr)
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
		slog.Error("failed to upload profile picture", "user_id", userID, "error", err)
		writeProfilePictureClientError(w, "failed to upload profile picture", err)
		return
	}

	// Update user's profile picture URL
	updates := map[string]interface{}{
		"profile_picture_url": pictureURL,
	}

	if err := h.userService.UpdateUser(userID, updates, token); err != nil {
		slog.Error("failed to update user profile picture url", "error", err)
		errors.WriteError(w, errors.InternalServerError("failed to update profile picture", err))
		return
	}

	slog.Info("profile picture uploaded", "user_id", userID)

	response := profiletransport.ProfilePictureResponse{
		Success: true,
		URL:     pictureURL,
		Message: "Profile picture uploaded successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func writeProfilePictureClientError(w http.ResponseWriter, fallbackMessage string, err error) {
	var downstreamErr *clientsinfra.DownstreamHTTPError
	if !stderrors.As(err, &downstreamErr) {
		errors.WriteError(w, errors.InternalServerError(fallbackMessage, err))
		return
	}

	message := extractDownstreamErrorMessage(downstreamErr.Body)
	if message == "" {
		message = fallbackMessage
	}

	switch downstreamErr.StatusCode {
	case http.StatusBadRequest:
		errors.WriteError(w, errors.BadRequest(message))
	case http.StatusUnauthorized:
		errors.WriteError(w, errors.Unauthorized(message))
	case http.StatusForbidden:
		errors.WriteError(w, errors.Forbidden(message))
	case http.StatusNotFound:
		errors.WriteError(w, errors.NotFound(message))
	default:
		errors.WriteError(w, errors.InternalServerError(fallbackMessage, err))
	}
}

func extractDownstreamErrorMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var payload struct {
		Detail string `json:"detail"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if payload.Detail != "" {
			return payload.Detail
		}
		if payload.Error != "" {
			return payload.Error
		}
	}

	return raw
}
