package clients

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	"github.com/PetoAdam/homenavi/auth-service/internal/models/entities"
	"github.com/PetoAdam/homenavi/auth-service/internal/models/requests"
	"github.com/PetoAdam/homenavi/auth-service/pkg/errors"
	"github.com/golang-jwt/jwt/v5"
)

type UserConfig struct {
	BaseURL       string
	JWTPrivateKey *rsa.PrivateKey
}

// UserClient wraps user-service HTTP calls.
type UserClient struct {
	baseURL       string
	jwtPrivateKey *rsa.PrivateKey
	httpClient    *http.Client
}

func NewUserClient(cfg UserConfig) *UserClient {
	return &UserClient{baseURL: cfg.BaseURL, jwtPrivateKey: cfg.JWTPrivateKey, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *UserClient) CreateUser(req *requests.SignupRequest) (*entities.User, error) {
	userReq := map[string]any{"user_name": req.UserName, "email": req.Email, "password": req.Password, "first_name": req.FirstName, "last_name": req.LastName}
	return c.createUserFromMap(userReq)
}

func (c *UserClient) ValidateCredentials(email, password string) (*entities.User, error) {
	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		return nil, errors.InternalServerError("failed to marshal credentials", err)
	}

	resp, err := c.makeRequest(http.MethodPost, "/users/validate", body, "")
	if err != nil {
		return nil, errors.InternalServerError("failed to validate credentials", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, errors.Unauthorized("invalid credentials")
	case http.StatusLocked:
		return nil, errors.Forbidden("account is locked")
	case http.StatusOK:
		var user entities.User
		if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
			return nil, errors.InternalServerError("failed to decode user response", err)
		}
		return &user, nil
	default:
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}
}

func (c *UserClient) GetUser(userID string) (*entities.User, error) {
	token, err := c.issueInternalToken(userID)
	if err != nil {
		return nil, errors.InternalServerError("failed to issue internal token", err)
	}

	resp, err := c.makeRequest(http.MethodGet, "/users/"+userID, nil, token)
	if err != nil {
		return nil, errors.InternalServerError("failed to get user", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFound("user not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}
	return &user, nil
}

func (c *UserClient) GetUserByEmail(email string) (*entities.User, error) {
	token, _ := c.issueInternalToken("")
	resp, err := c.makeRequest(http.MethodGet, "/users?email="+url.QueryEscape(email), nil, token)
	if err != nil {
		return nil, errors.InternalServerError("failed to get user by email", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFound("user not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}
	return &user, nil
}

func (c *UserClient) UpdateUser(userID string, updates map[string]interface{}, jwtToken string) error {
	body, err := json.Marshal(updates)
	if err != nil {
		return errors.InternalServerError("failed to marshal updates", err)
	}

	resp, err := c.makeRequest(http.MethodPatch, "/users/"+userID, body, jwtToken)
	if err != nil {
		return errors.InternalServerError("failed to update user", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return errors.NotFound("user not found")
	}
	if resp.StatusCode != http.StatusOK {
		return errors.InternalServerError("user service returned unexpected status", nil)
	}
	return nil
}

func (c *UserClient) UpdatePassword(userID, newPassword string) error {
	token, err := c.issueInternalToken(userID)
	if err != nil {
		return errors.InternalServerError("failed to issue internal token", err)
	}
	return c.UpdateUser(userID, map[string]interface{}{"password": newPassword}, token)
}

func (c *UserClient) DeleteUser(userID string, jwtToken string) error {
	resp, err := c.makeRequest(http.MethodDelete, "/users/"+userID, nil, jwtToken)
	if err != nil {
		return errors.InternalServerError("failed to delete user", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return errors.NotFound("user not found")
	}
	if resp.StatusCode != http.StatusNoContent {
		return errors.InternalServerError("user service returned unexpected status", nil)
	}
	return nil
}

func (c *UserClient) GetUserByGoogleID(googleID string) (*entities.User, error) {
	token, _ := c.issueInternalToken("")
	resp, err := c.makeRequest(http.MethodGet, "/users?google_id="+url.QueryEscape(googleID), nil, token)
	if err != nil {
		return nil, errors.InternalServerError("failed to get user by google_id", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFound("user not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}
	return &user, nil
}

func (c *UserClient) LinkGoogleID(userID, googleID string) error {
	token, err := c.issueInternalToken(userID)
	if err != nil {
		return errors.InternalServerError("failed to issue internal token", err)
	}
	return c.UpdateUser(userID, map[string]interface{}{"google_id": googleID}, token)
}

func (c *UserClient) CreateGoogleUser(userInfo *authdomain.GoogleUserInfo) (*entities.User, error) {
	return c.createUserFromMap(map[string]interface{}{
		"user_name":           userInfo.Email,
		"email":               userInfo.Email,
		"first_name":          userInfo.FirstName,
		"last_name":           userInfo.LastName,
		"role":                "user",
		"google_id":           userInfo.ID,
		"profile_picture_url": userInfo.Picture,
	})
}

func (c *UserClient) ListUsers(values url.Values, bearer string) ([]entities.User, map[string]interface{}, error) {
	token := strings.TrimPrefix(bearer, "Bearer ")
	resp, err := c.makeRequest(http.MethodGet, "/users?"+values.Encode(), nil, token)
	if err != nil {
		return nil, nil, errors.InternalServerError("failed list users", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, errors.InternalServerError("unexpected status", nil)
	}

	var raw struct {
		Users      []entities.User `json:"users"`
		Page       int             `json:"page"`
		PageSize   int             `json:"page_size"`
		Total      int64           `json:"total"`
		TotalPages int64           `json:"total_pages"`
		Query      string          `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, nil, errors.InternalServerError("decode error", err)
	}
	meta := map[string]interface{}{"page": raw.Page, "page_size": raw.PageSize, "total": raw.Total, "total_pages": raw.TotalPages, "query": raw.Query}
	return raw.Users, meta, nil
}

func (c *UserClient) createUserFromMap(userReq map[string]interface{}) (*entities.User, error) {
	body, err := json.Marshal(userReq)
	if err != nil {
		return nil, errors.InternalServerError("failed to marshal user request", err)
	}

	resp, err := c.makeRequest(http.MethodPost, "/users", body, "")
	if err != nil {
		return nil, errors.InternalServerError("failed to create user", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusBadRequest:
		return nil, errors.BadRequest("invalid user data")
	case http.StatusConflict:
		return nil, errors.BadRequest("user already exists with this email or username")
	case http.StatusCreated:
		var user entities.User
		if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
			return nil, errors.InternalServerError("failed to decode user response", err)
		}
		return &user, nil
	default:
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}
}

func (c *UserClient) makeRequest(method, path string, body []byte, jwtToken string) (*http.Response, error) {
	endpoint := c.baseURL + path
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, endpoint, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+jwtToken)
	}
	return c.httpClient.Do(req)
}

func (c *UserClient) issueInternalToken(userID string) (string, error) {
	claims := jwt.MapClaims{"sub": userID, "exp": time.Now().Add(2 * time.Minute).Unix(), "iat": time.Now().Unix(), "role": "service"}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if c.jwtPrivateKey == nil {
		return "", fmt.Errorf("jwt private key not configured")
	}
	return token.SignedString(c.jwtPrivateKey)
}
