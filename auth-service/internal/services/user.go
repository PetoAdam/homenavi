package services

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"auth-service/internal/config"
	"auth-service/internal/models/entities"
	"auth-service/internal/models/requests"
	"auth-service/pkg/errors"
	"auth-service/pkg/roles"

	"github.com/golang-jwt/jwt/v5"
)

type UserService struct {
	config         *config.Config
	userServiceURL string
	httpClient     *http.Client
}

func NewUserService(cfg *config.Config) *UserService {
	return &UserService{
		config:         cfg,
		userServiceURL: cfg.UserServiceURL,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *UserService) CreateUser(req *requests.SignupRequest) (*entities.User, error) {
	userReq := map[string]interface{}{
		"user_name":  req.UserName,
		"email":      req.Email,
		"password":   req.Password,
		"first_name": req.FirstName,
		"last_name":  req.LastName,
	}

	body, err := json.Marshal(userReq)
	if err != nil {
		return nil, errors.InternalServerError("failed to marshal user request", err)
	}

	resp, err := s.makeRequest("POST", "/users", body, "")
	if err != nil {
		return nil, errors.InternalServerError("failed to create user", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		return nil, errors.BadRequest("invalid user data")
	}

	if resp.StatusCode == 409 {
		return nil, errors.BadRequest("user already exists with this email or username")
	}

	if resp.StatusCode != 201 {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}

	return &user, nil
}

func (s *UserService) ValidateCredentials(email, password string) (*entities.User, error) {
	body, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		return nil, errors.InternalServerError("failed to marshal credentials", err)
	}

	resp, err := s.makeRequest("POST", "/users/validate", body, "")
	if err != nil {
		return nil, errors.InternalServerError("failed to validate credentials", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, errors.Unauthorized("invalid credentials")
	}

	if resp.StatusCode != 200 {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}

	return &user, nil
}

func (s *UserService) GetUser(userID string) (*entities.User, error) {
	// Issue a short-lived token for internal service communication
	token, err := s.issueInternalToken(userID)
	if err != nil {
		return nil, errors.InternalServerError("failed to issue internal token", err)
	}

	resp, err := s.makeRequest("GET", "/users/"+userID, nil, token)
	if err != nil {
		return nil, errors.InternalServerError("failed to get user", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, errors.NotFound("user not found")
	}

	if resp.StatusCode != 200 {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}

	return &user, nil
}

func (s *UserService) GetUserByEmail(email string) (*entities.User, error) {
	resp, err := s.makeRequest("GET", "/users?email="+email, nil, "")
	if err != nil {
		return nil, errors.InternalServerError("failed to get user by email", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, errors.NotFound("user not found")
	}

	if resp.StatusCode != 200 {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}

	return &user, nil
}

func (s *UserService) UpdateUser(userID string, updates map[string]interface{}, jwtToken string) error {
	body, err := json.Marshal(updates)
	if err != nil {
		return errors.InternalServerError("failed to marshal updates", err)
	}

	resp, err := s.makeRequest("PATCH", "/users/"+userID, body, jwtToken)
	if err != nil {
		return errors.InternalServerError("failed to update user", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return errors.NotFound("user not found")
	}

	if resp.StatusCode != 200 {
		return errors.InternalServerError("user service returned unexpected status", nil)
	}

	return nil
}

func (s *UserService) UpdatePassword(userID, newPassword string) error {
	// Password hashing will be handled by user-service
	updates := map[string]interface{}{
		"password": newPassword,
	}

	// Issue internal token for this operation
	token, err := s.issueInternalToken(userID)
	if err != nil {
		return errors.InternalServerError("failed to issue internal token", err)
	}

	return s.UpdateUser(userID, updates, token)
}

func (s *UserService) DeleteUser(userID string, jwtToken string) error {
	resp, err := s.makeRequest("DELETE", "/users/"+userID, nil, jwtToken)
	if err != nil {
		return errors.InternalServerError("failed to delete user", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return errors.NotFound("user not found")
	}

	if resp.StatusCode != 204 {
		return errors.InternalServerError("user service returned unexpected status", nil)
	}

	return nil
}

func (s *UserService) GetUserByGoogleID(googleID string) (*entities.User, error) {
	resp, err := s.makeRequest("GET", "/users?google_id="+googleID, nil, "")
	if err != nil {
		return nil, errors.InternalServerError("failed to get user by google_id", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, errors.NotFound("user not found")
	}

	if resp.StatusCode != 200 {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}

	return &user, nil
}

func (s *UserService) LinkGoogleID(userID, googleID string) error {
	updates := map[string]interface{}{
		"google_id": googleID,
	}

	token, err := s.issueInternalToken(userID)
	if err != nil {
		return errors.InternalServerError("failed to issue internal token", err)
	}

	return s.UpdateUser(userID, updates, token)
}

func (s *UserService) CreateGoogleUser(userInfo *GoogleUserInfo) (*entities.User, error) {
	userReq := map[string]interface{}{
		"user_name":           userInfo.Email, // Use email as username for Google OAuth users
		"email":               userInfo.Email,
		"first_name":          userInfo.FirstName,
		"last_name":           userInfo.LastName,
		"role":                roles.User,
		"google_id":           userInfo.ID,
		"profile_picture_url": userInfo.Picture,
	}

	return s.CreateUserFromMap(userReq)
}

func (s *UserService) CreateUserFromMap(userReq map[string]interface{}) (*entities.User, error) {
	body, err := json.Marshal(userReq)
	if err != nil {
		return nil, errors.InternalServerError("failed to marshal user request", err)
	}

	resp, err := s.makeRequest("POST", "/users", body, "")
	if err != nil {
		return nil, errors.InternalServerError("failed to create user", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		return nil, errors.BadRequest("invalid user data")
	}

	if resp.StatusCode == 409 {
		return nil, errors.BadRequest("user already exists with this email or username")
	}

	if resp.StatusCode != 201 {
		return nil, errors.InternalServerError("user service returned unexpected status", nil)
	}

	var user entities.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, errors.InternalServerError("failed to decode user response", err)
	}

	return &user, nil
}

func (s *UserService) makeRequest(method, path string, body []byte, jwtToken string) (*http.Response, error) {
	url := s.userServiceURL + path

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+jwtToken)
	}

	return s.httpClient.Do(req)
}

func (s *UserService) issueInternalToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":  userID,
		"exp":  time.Now().Add(2 * time.Minute).Unix(),
		"iat":  time.Now().Unix(),
		"role": "service",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.config.JWTPrivateKey)
}
