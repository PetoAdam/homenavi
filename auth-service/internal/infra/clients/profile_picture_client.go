package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

// ProfilePictureClient wraps profile-picture-service HTTP calls.
type ProfilePictureClient struct {
	baseURL    string
	httpClient *http.Client
}

type ProfilePictureUploadURL struct {
	ObjectKey string            `json:"object_key"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresIn int               `json:"expires_in"`
}

type DownstreamHTTPError struct {
	StatusCode int
	Body       string
}

func (e *DownstreamHTTPError) Error() string {
	return fmt.Sprintf("downstream request failed with status %d: %s", e.StatusCode, e.Body)
}

func newDownstreamHTTPError(statusCode int, body []byte) error {
	return &DownstreamHTTPError{StatusCode: statusCode, Body: string(body)}
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

func (c *ProfilePictureClient) CreateUploadURL(userID, filename, contentType string) (ProfilePictureUploadURL, error) {
	payload := map[string]string{
		"user_id":      userID,
		"filename":     filename,
		"content_type": contentType,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ProfilePictureUploadURL{}, fmt.Errorf("marshal upload-url request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/profile-pictures/upload-url", bytes.NewReader(body))
	if err != nil {
		return ProfilePictureUploadURL{}, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ProfilePictureUploadURL{}, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProfilePictureUploadURL{}, fmt.Errorf("failed to read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ProfilePictureUploadURL{}, newDownstreamHTTPError(resp.StatusCode, responseBody)
	}

	var result struct {
		Success   bool              `json:"success"`
		ObjectKey string            `json:"object_key"`
		UploadURL string            `json:"upload_url"`
		Method    string            `json:"method"`
		Headers   map[string]string `json:"headers"`
		ExpiresIn int               `json:"expires_in"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return ProfilePictureUploadURL{}, fmt.Errorf("failed to parse response: %v", err)
	}
	if !result.Success || result.ObjectKey == "" || result.UploadURL == "" {
		return ProfilePictureUploadURL{}, fmt.Errorf("invalid upload-url response")
	}
	if result.Method == "" {
		result.Method = http.MethodPut
	}
	return ProfilePictureUploadURL{
		ObjectKey: result.ObjectKey,
		UploadURL: result.UploadURL,
		Method:    result.Method,
		Headers:   result.Headers,
		ExpiresIn: result.ExpiresIn,
	}, nil
}

func (c *ProfilePictureClient) CompleteUpload(userID, objectKey string) (string, error) {
	payload := map[string]string{
		"user_id":    userID,
		"object_key": objectKey,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal upload completion request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/profile-pictures/complete", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

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
		return "", newDownstreamHTTPError(resp.StatusCode, responseBody)
	}

	var result map[string]any
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}
	if success, ok := result["success"].(bool); !ok || !success {
		return "", fmt.Errorf("upload completion failed: %v", result)
	}
	if pictureURL, ok := result["url"].(string); ok {
		return pictureURL, nil
	}
	return "", fmt.Errorf("invalid response: missing or invalid 'url' field")
}

func (c *ProfilePictureClient) UploadProfilePicture(userID string, file multipart.File, header *multipart.FileHeader) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("user_id", userID); err != nil {
		return "", fmt.Errorf("failed to write user_id field: %v", err)
	}
	partHeaders := textproto.MIMEHeader{}
	partHeaders.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, header.Filename))
	if contentType := header.Header.Get("Content-Type"); contentType != "" {
		partHeaders.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(partHeaders)
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
		return "", newDownstreamHTTPError(resp.StatusCode, responseBody)
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
