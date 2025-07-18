package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"time"

	"auth-service/internal/config"
)

type ProfilePictureService struct {
	config                   *config.Config
	profilePictureServiceURL string
	httpClient               *http.Client
}

func NewProfilePictureService(cfg *config.Config) *ProfilePictureService {
	return &ProfilePictureService{
		config:                   cfg,
		profilePictureServiceURL: cfg.ProfilePictureServiceURL,
		httpClient:               &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *ProfilePictureService) GenerateAvatar(userID string) (string, error) {
	// Create request to generate avatar
	req, err := http.NewRequest("POST", s.profilePictureServiceURL+"/generate/"+userID, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Make request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("avatar generation failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Validate response structure
	if !result["success"].(bool) {
		return "", fmt.Errorf("avatar generation failed: %v", result)
	}

	// Return the avatar URL
	if url, ok := result["url"].(string); ok {
		log.Printf("[INFO] Avatar generated for user %s: %s", userID, url)
		return url, nil
	}

	return "", fmt.Errorf("invalid response: missing or invalid 'url' field")
}

func (s *ProfilePictureService) UploadProfilePicture(userID string, file multipart.File, header *multipart.FileHeader) (string, error) {
	// Create form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add user_id field
	if err := writer.WriteField("user_id", userID); err != nil {
		return "", fmt.Errorf("failed to write user_id field: %v", err)
	}

	// Add file field
	part, err := writer.CreateFormFile("file", header.Filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %v", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("failed to copy file: %v", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close form writer: %v", err)
	}

	// Create request
	req, err := http.NewRequest("POST", s.profilePictureServiceURL+"/upload", body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Make request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body for better error handling
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	log.Printf("[DEBUG] Upload response: %+v", result)

	// Check if upload was successful
	if success, ok := result["success"].(bool); !ok || !success {
		return "", fmt.Errorf("upload failed: %v", result)
	}

	// Get the primary URL (the main profile picture URL)
	if primaryURL, ok := result["primary_url"].(string); ok {
		log.Printf("[INFO] Profile picture uploaded for user %s: %s", userID, primaryURL)
		return primaryURL, nil
	}

	return "", fmt.Errorf("invalid response: missing or invalid 'primary_url' field")
}
