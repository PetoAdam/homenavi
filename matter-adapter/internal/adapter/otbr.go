package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type otbrDiagnostics struct {
	State   string
	Dataset string
}

var (
	otbrStateRegex   = regexp.MustCompile(`(?is)OTBR\s*state\s*:\s*(?:<[^>]+>\s*)*([a-z_]+)`)
	otbrDatasetRegex = regexp.MustCompile(`(?is)HEX-encoded\s+TLVs\s*([0-9a-f]+)`)
)

func parseOTBRDiagnostics(body string) otbrDiagnostics {
	content := strings.TrimSpace(body)
	out := otbrDiagnostics{}
	if match := otbrStateRegex.FindStringSubmatch(content); len(match) > 1 {
		out.State = strings.TrimSpace(strings.ToLower(match[1]))
	}
	if match := otbrDatasetRegex.FindStringSubmatch(content); len(match) > 1 {
		out.Dataset = "hex:" + strings.TrimSpace(strings.ToLower(match[1]))
	}
	return out
}

func (s *Service) fetchOTBRDiagnostics(ctx context.Context) (otbrDiagnostics, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.otbrBaseURL), "/")
	if baseURL == "" {
		return otbrDiagnostics{}, fmt.Errorf("otbr base url not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	apiBaseURL, err := s.otbrNodeAPIBaseURL(baseURL)
	if err == nil {
		if diagnostics, apiErr := s.fetchOTBRDiagnosticsFromNodeAPI(ctx, apiBaseURL); apiErr == nil {
			return diagnostics, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/otbr", nil)
	if err != nil {
		return otbrDiagnostics{}, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return otbrDiagnostics{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return otbrDiagnostics{}, fmt.Errorf("otbr request failed with status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return otbrDiagnostics{}, err
	}
	parsed := parseOTBRDiagnostics(string(body))
	if parsed.State == "" && parsed.Dataset == "" {
		return otbrDiagnostics{}, fmt.Errorf("otbr diagnostics page missing expected markers")
	}
	return parsed, nil
}

func (s *Service) fetchOTBRDiagnosticsFromNodeAPI(ctx context.Context, apiBaseURL string) (otbrDiagnostics, error) {
	stateBody, err := s.fetchOTBRNodeAPI(ctx, apiBaseURL+"/node/state", "")
	if err != nil {
		return otbrDiagnostics{}, err
	}
	datasetBody, err := s.fetchOTBRNodeAPI(ctx, apiBaseURL+"/node/dataset/active", "text/plain")
	if err != nil {
		return otbrDiagnostics{}, err
	}
	state := parseOTBRNodeState(stateBody)
	dataset := strings.TrimSpace(strings.ToLower(datasetBody))
	if dataset != "" && !strings.HasPrefix(dataset, "hex:") {
		dataset = "hex:" + dataset
	}
	if state == "" && dataset == "" {
		return otbrDiagnostics{}, fmt.Errorf("otbr node api missing expected state and dataset")
	}
	return otbrDiagnostics{State: state, Dataset: dataset}, nil
}

func (s *Service) fetchOTBRNodeAPI(ctx context.Context, requestURL, accept string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(accept) != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("otbr node api request failed with status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func parseOTBRNodeState(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	var parsed string
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return strings.ToLower(strings.TrimSpace(parsed))
	}
	return strings.ToLower(strings.Trim(trimmed, `"`))
}

func (s *Service) otbrNodeAPIBaseURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		host = strings.TrimSpace(s.threadBorderRouterHost)
	}
	if host == "" {
		return "", fmt.Errorf("otbr host not configured")
	}
	port := s.threadBorderRouterPort
	if port <= 0 {
		if parsedPort := parsed.Port(); parsedPort != "" {
			if value, convErr := strconv.Atoi(parsedPort); convErr == nil {
				port = value
			}
		}
	}
	if port <= 0 {
		port = 8080
	}
	return fmt.Sprintf("http://%s:%d", host, port), nil
}
