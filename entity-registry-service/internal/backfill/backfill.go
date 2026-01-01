package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"entity-registry-service/internal/store"
)

type hdpDeviceListItem struct {
	DeviceID     string `json:"device_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Protocol     string `json:"protocol"`
	ExternalID   string `json:"external_id"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
}

// RunOnce queries device-hub for discovered devices and ensures each one has a
// canonical ERS device with an HDP binding.
//
// Returns how many devices were newly created.
func RunOnce(ctx context.Context, repo *store.Repo, deviceHubURL string, httpClient *http.Client) (int, error) {
	if repo == nil {
		return 0, errors.New("repo is required")
	}
	base := strings.TrimRight(strings.TrimSpace(deviceHubURL), "/")
	if base == "" {
		base = "http://device-hub:8090"
	}
	hc := httpClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}

	url := base + "/api/hdp/devices"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return 0, fmt.Errorf("device-hub list failed: %s (%s)", resp.Status, strings.TrimSpace(string(b)))
	}

	dec := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024))
	var items []hdpDeviceListItem
	if err := dec.Decode(&items); err != nil {
		return 0, err
	}

	created := 0
	for _, it := range items {
		hdpID := strings.TrimSpace(it.DeviceID)
		if hdpID == "" {
			// Fallback if device-hub ever changes shape; keep it best-effort.
			proto := strings.TrimSpace(it.Protocol)
			ext := strings.TrimSpace(it.ExternalID)
			if proto != "" && ext != "" {
				hdpID = proto + "/" + ext
			}
		}
		if hdpID == "" {
			continue
		}

		name := strings.TrimSpace(it.Name)
		if name == "" {
			if it.Manufacturer != "" || it.Model != "" {
				name = strings.TrimSpace(strings.TrimSpace(it.Manufacturer) + " " + strings.TrimSpace(it.Model))
			}
		}
		desc := strings.TrimSpace(it.Description)

		_, wasCreated, err := repo.EnsureDeviceForHDP(ctx, hdpID, name, desc)
		if err != nil {
			return created, err
		}
		if wasCreated {
			created++
		}
	}

	return created, nil
}

// Start runs a best-effort backfill loop until a successful backfill occurs or
// ctx is cancelled.
func Start(ctx context.Context, repo *store.Repo, deviceHubURL string, httpClient *http.Client) {
	go func() {
		delay := 2 * time.Second
		for {
			created, err := RunOnce(ctx, repo, deviceHubURL, httpClient)
			if err == nil {
				slog.Info("ers backfill complete", "created", created)
				return
			}
			slog.Warn("ers backfill failed; will retry", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if delay < 30*time.Second {
				delay *= 2
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
			}
		}
	}()
}
