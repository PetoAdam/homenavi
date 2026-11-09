package zigbee

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	mqttinfra "github.com/PetoAdam/homenavi/zigbee-adapter/internal/infra/mqtt"
)

type reportingTarget struct {
	device   string
	endpoint int
	bound    bool
}

type reportingResponse struct {
	Transaction string         `json:"transaction"`
	Status      string         `json:"status"`
	Error       string         `json:"error"`
	Data        map[string]any `json:"data"`
}

func nativeReportingTarget(raw map[string]any, external, coordinator string, endpointID int) (reportingTarget, bool) {
	target := reportingTarget{device: external, endpoint: endpointID}
	endpoints, ok := raw["endpoints"].(map[string]any)
	if !ok || external == "" || coordinator == "" {
		return target, false
	}
	endpoint, ok := endpoints[strconv.Itoa(endpointID)].(map[string]any)
	if !ok {
		return target, false
	}
	bindings, _ := endpoint["bindings"].([]any)
	for _, value := range bindings {
		binding, _ := value.(map[string]any)
		destination, _ := binding["target"].(map[string]any)
		cluster, _ := binding["cluster"].(string)
		address, _ := destination["ieee_address"].(string)
		if strings.EqualFold(cluster, "genOnOff") && strings.EqualFold(address, coordinator) {
			target.bound = true
			break
		}
	}
	reporting, _ := endpoint["configured_reportings"].([]any)
	for _, value := range reporting {
		config, _ := value.(map[string]any)
		cluster, _ := config["cluster"].(string)
		attribute, _ := config["attribute"].(string)
		maximum, _ := config["maximum_report_interval"].(float64)
		if target.bound && strings.EqualFold(cluster, "genOnOff") && strings.EqualFold(attribute, "onOff") && maximum > 0 && maximum <= 60 {
			return target, false
		}
	}
	return target, true
}

func (z *ZigbeeAdapter) enqueueReportingSetup(target reportingTarget) {
	key := fmt.Sprintf("%s/%d", target.device, target.endpoint)
	z.reportingMu.Lock()
	defer z.reportingMu.Unlock()
	if z.reportingQueue == nil {
		return
	}
	if last, ok := z.reportingLast[key]; ok && (last.IsZero() || time.Since(last) < 15*time.Minute) {
		return
	}
	select {
	case z.reportingQueue <- target:
		z.reportingLast[key] = time.Time{}
	default:
		slog.Warn("zigbee reporting queue full", "device", target.device, "endpoint", target.endpoint)
	}
}

func (z *ZigbeeAdapter) runReportingSetup() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-z.ctx.Done():
			return
		case <-ticker.C:
			z.requestBridgeDevicesThrottled("reporting-retry")
		case target := <-z.reportingQueue:
			err := configureNativeReporting(target, z.reportingRequest)
			key := fmt.Sprintf("%s/%d", target.device, target.endpoint)
			z.reportingMu.Lock()
			z.reportingLast[key] = time.Now()
			z.reportingMu.Unlock()
			if err != nil {
				slog.Warn("zigbee native reporting setup failed", "device", target.device, "endpoint", target.endpoint, "error", err)
			} else {
				slog.Info("zigbee native reporting configured", "device", target.device, "endpoint", target.endpoint)
			}
		}
	}
}

func configureNativeReporting(target reportingTarget, request func(string, map[string]any) (reportingResponse, error)) error {
	if !target.bound {
		_, err := request("bind", map[string]any{
			"from": target.device, "from_endpoint": target.endpoint, "to": "Coordinator", "clusters": []string{"genOnOff"},
		})
		if err != nil {
			return err
		}
	}
	_, err := request("reporting/configure", map[string]any{
		"id": target.device, "endpoint": target.endpoint, "cluster": "genOnOff", "attribute": "onOff",
		"minimum_report_interval": 0, "maximum_report_interval": 60,
	})
	return err
}

func (z *ZigbeeAdapter) reportingRequest(action string, payload map[string]any) (reportingResponse, error) {
	return z.sendReportingRequest(z.ctx, action, payload, z.client.Publish)
}

func (z *ZigbeeAdapter) sendReportingRequest(parent context.Context, action string, payload map[string]any, publish func(string, []byte) error) (reportingResponse, error) {
	if err := parent.Err(); err != nil {
		return reportingResponse{}, err
	}
	transaction := uuid.NewString()
	payload["transaction"] = transaction
	encoded, err := json.Marshal(payload)
	if err != nil {
		return reportingResponse{}, err
	}
	responses := make(chan reportingResponse, 1)
	z.reportingMu.Lock()
	z.reportingWait[transaction] = responses
	z.reportingMu.Unlock()
	defer func() {
		z.reportingMu.Lock()
		delete(z.reportingWait, transaction)
		z.reportingMu.Unlock()
	}()
	if err := publish("zigbee2mqtt/bridge/request/device/"+action, encoded); err != nil {
		return reportingResponse{}, err
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		return reportingResponse{}, ctx.Err()
	case response := <-responses:
		if response.Status != "ok" {
			return response, fmt.Errorf("%s: %s", action, response.Error)
		}
		if failed, ok := response.Data["failed"].([]any); ok && len(failed) > 0 {
			return response, fmt.Errorf("%s failed for clusters: %v", action, failed)
		}
		return response, nil
	}
}

func (z *ZigbeeAdapter) handleReportingResponse(message mqttinfra.Message) {
	var response reportingResponse
	if err := json.Unmarshal(message.Payload(), &response); err != nil || response.Transaction == "" {
		return
	}
	z.reportingMu.Lock()
	defer z.reportingMu.Unlock()
	if waiter := z.reportingWait[response.Transaction]; waiter != nil {
		select {
		case waiter <- response:
		default:
		}
	}
}
