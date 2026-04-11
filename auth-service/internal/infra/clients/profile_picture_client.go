package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"time"
)

// ProfilePictureClient wraps profile-picture-service HTTP calls.
type ProfilePictureClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewProfilePictureClient(baseURL string) *ProfilePictureClient {
	return &ProfilePictureClient{baseURL: baseURL, httpClient: &http.Client{Timeout: 30 * time.Second}}
}

func (c *ProfilePictureClient) GenerateAvatar(userID string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/generate/"+userID, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("avatar generation failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}
	if success, ok := result["success"].(bool); !ok || !success {
		return "", fmt.Errorf("avatar generation failed: %v", result)
	}
	if avatarURL, ok := result["url"].(string); ok {
		slog.Info("avatar generated", "user_id", userID, "url", avatarURL)
		return avatarURL, nil
	}
	return "", fmt.Errorf("invalid response: missing or invalid 'url' field")
}

func (c *ProfilePictureClient) UploadProfilePicture(userID string, file multipart.File, header *multipart.FileHeader) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("user_id", userID); err != nil {
		return "", fmt.Errorf("failed to write user_id field: %v", err)
	}
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

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/upload", body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}
	slog.Debug("upload response", "result", result)

	if success, ok := result["success"].(bool); !ok || !success {
		return "", fmt.Errorf("upload failed: %v", result)
	}
	if primaryURL, ok := result["primary_url"].(string); ok {
		slog.Info("profile picture uploaded", "user_id", userID, "url", primaryURL)
		return primaryURL, nil
	}
	return "", fmt.Errorf("invalid response: missing or invalid 'primary_url' field")
}
