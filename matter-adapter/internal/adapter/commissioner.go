package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type commissionerRequest struct {
	Protocol string         `json:"protocol"`
	Mode     string         `json:"mode"`
	FlowID   string         `json:"flow_id,omitempty"`
	Inputs   map[string]any `json:"inputs,omitempty"`
	Controller struct {
		OTBRBaseURL          string `json:"otbr_base_url,omitempty"`
		OTBRExpectedState    string `json:"otbr_expected_state,omitempty"`
		ThreadBorderRouter   string `json:"thread_border_router,omitempty"`
		ThreadBorderRouterPort int  `json:"thread_border_router_port,omitempty"`
		CommissioningInterface string `json:"commissioning_interface,omitempty"`
	} `json:"controller,omitempty"`
}

type commissionerCommandRequest struct {
	Protocol string         `json:"protocol"`
	DeviceID string         `json:"device_id"`
	Command  string         `json:"command"`
	Args     map[string]any `json:"args,omitempty"`
}

type commissionerResponse struct {
	ExternalID string         `json:"external_id,omitempty"`
	DeviceID   string         `json:"device_id,omitempty"`
	Message    string         `json:"message,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	State      map[string]any `json:"state,omitempty"`
}

func (s *Service) commissionerAvailable() bool {
	return s.commissionerEnabled && strings.TrimSpace(s.commissionerCommand) != ""
}

func (s *Service) cancelActivePairing() {
	s.pairingMu.Lock()
	defer s.pairingMu.Unlock()
	if s.pairingCancel != nil {
		s.pairingCancel()
		s.pairingCancel = nil
	}
}

func (s *Service) setActivePairingCancel(cancel context.CancelFunc) {
	s.pairingMu.Lock()
	defer s.pairingMu.Unlock()
	s.pairingCancel = cancel
}

func (s *Service) runCommissioner(ctx context.Context, mode, flowID string, inputs map[string]any) (*commissionerResponse, error) {
	if !s.commissionerAvailable() {
		return nil, fmt.Errorf("commissioner not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := s.commissionerTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.commissionerCommand, append(append([]string(nil), s.commissionerArgs...), "pair")...)
	request := commissionerRequest{
		Protocol: "matter",
		Mode:     mode,
		FlowID:   flowID,
		Inputs:   inputs,
	}
	request.Controller.OTBRBaseURL = s.otbrBaseURL
	request.Controller.OTBRExpectedState = s.otbrExpectedState
	request.Controller.ThreadBorderRouter = s.threadBorderRouterHost
	request.Controller.ThreadBorderRouterPort = s.threadBorderRouterPort
	request.Controller.CommissioningInterface = s.commissioningInterface
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("commissioner execution failed: %s", msg)
	}
	var response commissionerResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("commissioner output decode failed: %w", err)
	}
	if response.ExternalID == "" {
		response.ExternalID = "matter-device-001"
	}
	if len(response.Metadata) == 0 {
		response.Metadata = map[string]any{
			"type":         "light",
			"manufacturer": "Matter",
			"model":        "Commissioned Device",
			"icon":         "lightbulb",
		}
	}
	if len(response.State) == 0 {
		response.State = map[string]any{"on": false}
	}
	if strings.TrimSpace(response.Message) == "" {
		response.Message = "Matter commissioning complete"
	}
	return &response, nil
}

func (s *Service) runCommissionerCommand(ctx context.Context, deviceID, command string, args map[string]any) (*commissionerResponse, error) {
	if !s.commissionerAvailable() {
		return nil, fmt.Errorf("commissioner not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := s.commissionerTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.commissionerCommand, append(append([]string(nil), s.commissionerArgs...), "command")...)
	request := commissionerCommandRequest{
		Protocol: "matter",
		DeviceID: strings.TrimSpace(deviceID),
		Command:  strings.TrimSpace(command),
		Args:     args,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("commissioner command failed: %s", msg)
	}
	var response commissionerResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("commissioner command output decode failed: %w", err)
	}
	return &response, nil
}
