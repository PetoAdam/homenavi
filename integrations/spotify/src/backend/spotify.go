package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const spotifyAPIBase = "https://api.spotify.com/v1"
const spotifyTokenURL = "https://accounts.spotify.com/api/token"

type SpotifyClient struct {
	clientID     string
	clientSecret string
	refreshToken string

	httpClient *http.Client
	mu         sync.Mutex
	accessTok  string
	expiresAt  time.Time
}

func NewSpotifyClientFromEnv() (*SpotifyClient, error) {
	clientID := strings.TrimSpace(getenv("SPOTIFY_CLIENT_ID", ""))
	clientSecret := strings.TrimSpace(getenv("SPOTIFY_CLIENT_SECRET", ""))
	refreshToken := strings.TrimSpace(getenv("SPOTIFY_REFRESH_TOKEN", ""))
	if clientID == "" || clientSecret == "" || refreshToken == "" {
		secrets := loadSecretsFromFile(selectSecretsPath(), "spotify")
		if clientID == "" {
			clientID = strings.TrimSpace(secrets["SPOTIFY_CLIENT_ID"])
		}
		if clientSecret == "" {
			clientSecret = strings.TrimSpace(secrets["SPOTIFY_CLIENT_SECRET"])
		}
		if refreshToken == "" {
			refreshToken = strings.TrimSpace(secrets["SPOTIFY_REFRESH_TOKEN"])
		}
	}

	if clientID == "" || clientSecret == "" || refreshToken == "" {
		return nil, errors.New("missing SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, or SPOTIFY_REFRESH_TOKEN")
	}

	return &SpotifyClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func loadSecretsFromFile(path, integrationID string) map[string]string {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(integrationID) == "" {
		return map[string]string{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	var byIntegration map[string]map[string]string
	if err := json.Unmarshal(data, &byIntegration); err == nil {
		return byIntegration[integrationID]
	}
	var flat map[string]string
	if err := json.Unmarshal(data, &flat); err == nil {
		return flat
	}
	return map[string]string{}
}

func selectSecretsPath() string {
	if path := strings.TrimSpace(os.Getenv("INTEGRATION_SECRETS_PATH")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("INTEGRATIONS_SECRETS_PATH")); path != "" {
		return path
	}
	return filepath.Join("config", "integration.secrets.json")
}

func (c *SpotifyClient) Do(ctx context.Context, method, path string, query url.Values, body any) (int, []byte, error) {
	if c == nil {
		return 0, nil, errors.New("spotify client is nil")
	}
	token, err := c.ensureToken(ctx)
	if err != nil {
		return 0, nil, err
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	endpoint := spotifyAPIBase + path
	if len(query) > 0 {
		endpoint = endpoint + "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = strings.NewReader(string(payload))
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, data, fmt.Errorf("spotify api error: %s", strings.TrimSpace(string(data)))
	}
	return resp.StatusCode, data, nil
}

func (c *SpotifyClient) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessTok != "" && time.Now().Before(c.expiresAt.Add(-30*time.Second)) {
		tok := c.accessTok
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	if err := c.refreshAccessToken(ctx); err != nil {
		return "", err
	}

	c.mu.Lock()
	tok := c.accessTok
	c.mu.Unlock()
	return tok, nil
}

func (c *SpotifyClient) refreshAccessToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", c.refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spotifyTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	auth := base64.StdEncoding.EncodeToString([]byte(c.clientID + ":" + c.clientSecret))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("refresh token error: %s", strings.TrimSpace(string(data)))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	if parsed.AccessToken == "" {
		return errors.New("missing access_token in refresh response")
	}
	c.accessTok = parsed.AccessToken
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 3600
	}
	c.expiresAt = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	return nil
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
