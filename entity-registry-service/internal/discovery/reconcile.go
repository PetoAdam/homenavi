package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/google/uuid"
)

type snapshotStore interface {
	ApplyDiscoverySnapshot(context.Context, int64, []dbinfra.DiscoveryMetadata) error
}

func Reconcile(ctx context.Context, store snapshotStore, baseURL string, client *http.Client) error {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/hdp/device-metadata")
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return errors.New("invalid device-hub URL")
	}
	var items []dbinfra.DiscoveryMetadata
	seenSourceIDs := map[uuid.UUID]bool{}
	var revision int64
	cursor := ""
	limit := 100
	for page := 0; page < 1000; page++ {
		query := endpoint.Query()
		query.Set("limit", strconv.Itoa(limit))
		if cursor != "" {
			query.Set("cursor", cursor)
			query.Set("revision", strconv.FormatInt(revision, 10))
		}
		endpoint.RawQuery = query.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		_ = response.Body.Close()
		if readErr != nil || len(body) > 1<<20 {
			return errors.New("invalid metadata response size")
		}
		if response.StatusCode == http.StatusRequestEntityTooLarge && limit > 1 {
			limit /= 2
			page--
			continue
		}
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("metadata request returned %d", response.StatusCode)
		}
		var result struct {
			Devices    []dbinfra.DiscoveryMetadata `json:"devices"`
			NextCursor string                      `json:"next_cursor"`
			Revision   *int64                      `json:"revision"`
		}
		if err := json.Unmarshal(body, &result); err != nil || result.Revision == nil || *result.Revision < 0 || result.Devices == nil || len(result.Devices) > limit {
			return errors.New("invalid metadata response")
		}
		if page == 0 {
			revision = *result.Revision
		} else if revision != *result.Revision {
			return errors.New("metadata revision changed")
		}
		for _, device := range result.Devices {
			if device.SourceID == nil || *device.SourceID == uuid.Nil || seenSourceIDs[*device.SourceID] {
				return errors.New("invalid or duplicate source device ID")
			}
			seenSourceIDs[*device.SourceID] = true
			items = append(items, device)
		}
		if result.NextCursor == "" {
			return store.ApplyDiscoverySnapshot(ctx, revision, items)
		}
		if result.NextCursor == cursor || len(result.Devices) == 0 {
			return errors.New("invalid metadata cursor")
		}
		cursor = result.NextCursor
	}
	return errors.New("metadata page limit exceeded")
}

func Start(ctx context.Context, store snapshotStore, baseURL string, client *http.Client, config mqttx.Config, sharedGroup string) {
	updates := make(chan struct{}, 1)
	go func() {
		broker, err := mqttinfra.Connect(config, "entity-registry-discovery")
		if err != nil {
			slog.Warn("discovery broker unavailable", "error", err)
			return
		}
		defer broker.Close()
		handler := func(message mqttinfra.Message) {
			if strings.HasPrefix(message.Topic(), hdp.EventPrefix) {
				var event struct {
					Event string `json:"event"`
				}
				if json.Unmarshal(message.Payload(), &event) != nil || event.Event != "device_removed" {
					return
				}
			}
			select {
			case updates <- struct{}{}:
			default:
			}
		}
		for _, topic := range []string{hdp.MetadataPrefix + "#", hdp.EventPrefix + "#"} {
			if sharedGroup == "" {
				err = broker.Subscribe(topic, handler)
			} else {
				err = broker.SubscribeShared(sharedGroup+"-discovery", topic, handler)
			}
			if err != nil {
				slog.Warn("discovery subscription failed", "topic", topic, "error", err)
			}
		}
		<-ctx.Done()
	}()
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := Reconcile(runCtx, store, baseURL, client)
			cancel()
			if err != nil && ctx.Err() == nil {
				slog.Warn("discovery reconciliation failed", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-updates:
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				select {
				case <-updates:
				default:
				}
			}
		}
	}()
}
