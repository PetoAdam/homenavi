package mcpserver

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Config struct {
	Enabled            bool
	Issuer             string
	Resource           string
	PublicKey          *rsa.PublicKey
	AuthServiceURL     string
	APIGatewayURL      string
	AllowedOrigins     map[string]struct{}
	EnabledTools       map[string]struct{}
	RateLimitPerMinute int
	SessionValidator   func(context.Context, string) (bool, error)
	RateLimiter        func(context.Context, string) (bool, error)
	Logger             *slog.Logger
}

var supportedOAuthScopes = []string{"home.devices.read", "home.devices.write", "home.inventory.read", "home.inventory.write", "home.history.read", "home.automation.read", "home.automation.execute"}

type deviceClient interface {
	List(context.Context) ([]device, error)
}

type device struct {
	ID       string         `json:"id"`
	DeviceID string         `json:"device_id"`
	Type     string         `json:"type"`
	Online   bool           `json:"online"`
	State    map[string]any `json:"state"`
}

type listDevicesOutput struct {
	Devices []device `json:"devices"`
}

type deviceInput struct {
	DeviceID string `json:"device_id" jsonschema:"the canonical Homenavi device identifier"`
}

type historyInput struct {
	DeviceID string `json:"device_id" jsonschema:"the canonical Homenavi device identifier"`
	From     string `json:"from,omitempty" jsonschema:"an optional RFC3339 start timestamp"`
	To       string `json:"to,omitempty" jsonschema:"an optional RFC3339 end timestamp"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum state points to return, from 1 to 100"`
	Order    string `json:"order,omitempty" jsonschema:"asc or desc"`
}

type workflowInput struct {
	WorkflowID string `json:"workflow_id" jsonschema:"the automation workflow identifier"`
}

type controlledWrite struct {
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a stable unique key for retries of this exact write"`
}

type deviceCommandInput struct {
	DeviceID     string         `json:"device_id" jsonschema:"the canonical Homenavi device identifier"`
	State        map[string]any `json:"state" jsonschema:"the requested device state patch"`
	TransitionMs *int           `json:"transition_ms,omitempty" jsonschema:"optional transition duration in milliseconds"`
	controlledWrite
}

type runWorkflowInput struct {
	WorkflowID string `json:"workflow_id" jsonschema:"the automation workflow identifier"`
	controlledWrite
}

type groupCreateInput struct {
	Name        string   `json:"name" jsonschema:"the user-facing group name"`
	Description string   `json:"description,omitempty" jsonschema:"optional group description"`
	DeviceIDs   []string `json:"device_ids,omitempty" jsonschema:"canonical device identifiers to add to the group"`
	controlledWrite
}

type groupInput struct {
	GroupID string `json:"group_id" jsonschema:"the Homenavi device group identifier"`
}

type groupCommandInput struct {
	GroupID      string         `json:"group_id" jsonschema:"the Homenavi device group identifier"`
	State        map[string]any `json:"state" jsonschema:"the requested state patch to apply to every group member"`
	TransitionMs *int           `json:"transition_ms,omitempty" jsonschema:"optional transition duration in milliseconds"`
	controlledWrite
}

type groupDeviceCommandResult struct {
	DeviceID string `json:"device_id"`
	Result   any    `json:"result,omitempty"`
	Error    string `json:"error,omitempty"`
}

type groupCommandOutput struct {
	GroupID string                     `json:"group_id"`
	Results []groupDeviceCommandResult `json:"results"`
}

type dataOutput struct {
	Data any `json:"data"`
}

type catalogOutput struct {
	ReadTools      []string `json:"read_tools"`
	DeferredWrites []string `json:"deferred_writes"`
	EventChannels  []string `json:"event_channels"`
}

type eventChannel struct {
	Name             string   `json:"name"`
	Scope            string   `json:"scope"`
	Delivery         string   `json:"delivery"`
	SupportedFilters []string `json:"supported_filters"`
}

type writePolicyOutput struct {
	ApprovalRequired bool     `json:"approval_required"`
	IdempotencyKey   bool     `json:"idempotency_key_required"`
	CapabilityCheck  bool     `json:"capability_check_required"`
	AuditRequired    bool     `json:"audit_required"`
	DeferredWrites   []string `json:"deferred_writes"`
}

type server struct {
	config  Config
	clients *readClients
	mcp     http.Handler
	limiter *requestLimiter
}

type mcpPrincipal struct {
	scopes    map[string]struct{}
	sessionID string
	subject   string
	clientID  string
	token     string
}

type principalContextKey struct{}

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (w *auditResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

type rateWindow struct {
	started time.Time
	count   int
}

type requestLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	values map[string]rateWindow
}

func newRequestLimiter(limit int) *requestLimiter {
	if limit < 1 {
		limit = 60
	}
	return &requestLimiter{limit: limit, window: time.Minute, values: map[string]rateWindow{}}
}

func (l *requestLimiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	window := l.values[key]
	if window.started.IsZero() || now.Sub(window.started) >= l.window {
		l.values[key] = rateWindow{started: now, count: 1}
		return true
	}
	if window.count >= l.limit {
		return false
	}
	window.count++
	l.values[key] = window
	return true
}

func New(config Config) http.Handler {
	client := &http.Client{Timeout: 5 * time.Second}
	gatewayURL := strings.TrimRight(config.APIGatewayURL, "/")
	devices := &httpDeviceClient{readBaseURL: gatewayURL + "/api/mcp/hdp", commandReadURL: gatewayURL + "/api/mcp/hdp/device-command-targets", commandBaseURL: gatewayURL + "/api/mcp/hdp/devices", client: client}
	service := &server{config: config, clients: &readClients{devices: devices, gatewayURL: gatewayURL, authServiceURL: strings.TrimRight(config.AuthServiceURL, "/"), client: client}, limiter: newRequestLimiter(config.RateLimitPerMinute)}
	service.mcp = mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
		return service.mcpServerFor(principalFromContext(request.Context()))
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: config.Logger, MaxRequestBodyBytes: 64 << 10, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	mux.HandleFunc("/health", service.health)
	mux.HandleFunc("/.well-known/oauth-protected-resource", service.metadata)
	mux.Handle("/mcp", service.authenticate(service.mcp))
	return mux
}

func (s *server) mcpServerFor(principal *mcpPrincipal) *mcp.Server {
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "homenavi-mcp", Version: "0.1.0"}, nil)
	if principal == nil {
		return mcpServer
	}
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "describe_homenavi_api", Description: "Describe Homenavi's MCP read tools, deferred writes, and event channels."}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, catalogOutput, error) {
		return nil, catalogOutput{
			ReadTools:      []string{"list_devices", "get_device", "get_device_state", "list_device_groups", "get_device_group", "get_device_group_state", "list_device_integrations", "list_pairings", "list_rooms", "list_automation_workflows", "get_automation_workflow", "query_state_history"},
			DeferredWrites: []string{"pairing", "automation changes", "dashboard changes", "user/account changes"},
			EventChannels:  []string{"/ws/automation/runs/{run_id}", "/ws/ers", "/ws/hdp (MQTT-over-WebSocket)"},
		}, nil
	})
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_event_channels", Description: "List typed, scope-gated Homenavi event subscription capabilities. Raw WebSocket and MQTT transports are not exposed."}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, []eventChannel, error) {
		channels := []eventChannel{}
		if principal.hasScope("home.automation.read") {
			channels = append(channels, eventChannel{Name: "automation_run_status", Scope: "home.automation.read", Delivery: "deferred_typed_subscription", SupportedFilters: []string{"run_id"}})
		}
		if principal.hasScope("home.inventory.read") {
			channels = append(channels, eventChannel{Name: "entity_registry_changes", Scope: "home.inventory.read", Delivery: "deferred_typed_subscription", SupportedFilters: []string{"entity_type", "entity_id"}})
		}
		if principal.hasScope("home.devices.read") {
			channels = append(channels, eventChannel{Name: "device_state_changes", Scope: "home.devices.read", Delivery: "deferred_typed_subscription", SupportedFilters: []string{"device_id"}})
		}
		return nil, channels, nil
	})
	mcp.AddTool(mcpServer, &mcp.Tool{Name: "get_write_policy", Description: "Describe requirements for future controlled Homenavi write capabilities."}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, writePolicyOutput, error) {
		return nil, writePolicyOutput{ApprovalRequired: false, IdempotencyKey: true, CapabilityCheck: true, AuditRequired: true, DeferredWrites: []string{"pairing", "automation changes", "dashboard changes", "user/account changes"}}, nil
	})
	if principal.hasScope("home.devices.read") && s.toolEnabled("list_devices") {
		mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_devices", Description: "List the devices available in the current Homenavi deployment."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listDevicesOutput, error) {
			token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.devices.read")
			if err != nil {
				return nil, listDevicesOutput{}, err
			}
			devices, err := s.clients.devices.List(ctx, token)
			if err != nil {
				return nil, listDevicesOutput{}, err
			}
			return nil, listDevicesOutput{Devices: devices}, nil
		})
		if s.toolEnabled("get_device") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "get_device", Description: "Get one device by its canonical Homenavi device identifier."}, func(ctx context.Context, _ *mcp.CallToolRequest, input deviceInput) (*mcp.CallToolResult, device, error) {
				token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.devices.read")
				if err != nil {
					return nil, device{}, err
				}
				device, err := s.clients.GetDevice(ctx, input.DeviceID, token)
				return nil, device, err
			})
		}
		if s.toolEnabled("get_device_state") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "get_device_state", Description: "Get the current state for one Homenavi device."}, func(ctx context.Context, _ *mcp.CallToolRequest, input deviceInput) (*mcp.CallToolResult, device, error) {
				token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.devices.read")
				if err != nil {
					return nil, device{}, err
				}
				device, err := s.clients.GetDevice(ctx, input.DeviceID, token)
				return nil, device, err
			})
		}
		if s.toolEnabled("list_device_integrations") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_device_integrations", Description: "List configured device integrations without credentials."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataOutput, error) {
				token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.devices.read")
				if err != nil {
					return nil, dataOutput{}, err
				}
				result, err := s.clients.ListIntegrations(ctx, token)
				return nil, dataOutput{Data: result}, err
			})
		}
		if s.toolEnabled("list_pairings") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_pairings", Description: "List active and recent device pairing sessions."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataOutput, error) {
				token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.devices.read")
				if err != nil {
					return nil, dataOutput{}, err
				}
				result, err := s.clients.ListPairings(ctx, token)
				return nil, dataOutput{Data: result}, err
			})
		}
	}
	if principal.hasScope("home.inventory.read") && s.toolEnabled("list_rooms") {
		mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_rooms", Description: "List rooms in the Homenavi entity registry."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataOutput, error) {
			token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.inventory.read")
			if err != nil {
				return nil, dataOutput{}, err
			}
			result, err := s.clients.ListRooms(ctx, token)
			return nil, dataOutput{Data: result}, err
		})
	}
	if principal.hasScope("home.inventory.read") {
		if s.toolEnabled("list_device_groups") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_device_groups", Description: "List Homenavi device groups and their member devices."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataOutput, error) {
				token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.inventory.read")
				if err != nil {
					return nil, dataOutput{}, err
				}
				result, err := s.clients.ListGroups(ctx, token)
				return nil, dataOutput{Data: result}, err
			})
		}
		if s.toolEnabled("get_device_group") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "get_device_group", Description: "Get a Homenavi device group and its member devices."}, func(ctx context.Context, _ *mcp.CallToolRequest, input groupInput) (*mcp.CallToolResult, dataOutput, error) {
				token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.inventory.read")
				if err != nil {
					return nil, dataOutput{}, err
				}
				result, err := s.clients.GetGroup(ctx, input.GroupID, token)
				return nil, dataOutput{Data: result}, err
			})
		}
		if principal.hasScope("home.devices.read") && s.toolEnabled("get_device_group_state") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "get_device_group_state", Description: "Get the current state of every device in a Homenavi group."}, func(ctx context.Context, _ *mcp.CallToolRequest, input groupInput) (*mcp.CallToolResult, dataOutput, error) {
				inventoryToken, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.inventory.read")
				if err != nil {
					return nil, dataOutput{}, err
				}
				deviceToken, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.devices.read")
				if err != nil {
					return nil, dataOutput{}, err
				}
				group, err := s.clients.GetGroup(ctx, input.GroupID, inventoryToken)
				if err != nil {
					return nil, dataOutput{}, err
				}
				devices := make([]device, 0, len(group.HDPExternalIDs))
				for _, deviceID := range group.HDPExternalIDs {
					member, err := s.clients.GetDevice(ctx, deviceID, deviceToken)
					if err != nil {
						return nil, dataOutput{}, fmt.Errorf("load group member %q: %w", deviceID, err)
					}
					devices = append(devices, member)
				}
				return nil, dataOutput{Data: map[string]any{"group": group, "devices": devices}}, nil
			})
		}
	}
	if principal.hasScope("home.automation.read") {
		if s.toolEnabled("list_automation_workflows") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "list_automation_workflows", Description: "List configured automation workflows without changing them."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataOutput, error) {
				token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.automation.read")
				if err != nil {
					return nil, dataOutput{}, err
				}
				result, err := s.clients.ListAutomationWorkflows(ctx, token)
				return nil, dataOutput{Data: result}, err
			})
		}
		if s.toolEnabled("get_automation_workflow") {
			mcp.AddTool(mcpServer, &mcp.Tool{Name: "get_automation_workflow", Description: "Get one automation workflow by identifier without changing it."}, func(ctx context.Context, _ *mcp.CallToolRequest, input workflowInput) (*mcp.CallToolResult, dataOutput, error) {
				token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.automation.read")
				if err != nil {
					return nil, dataOutput{}, err
				}
				result, err := s.clients.GetAutomationWorkflow(ctx, input.WorkflowID, token)
				return nil, dataOutput{Data: result}, err
			})
		}
	}
	if principal.hasScope("home.devices.write") && s.toolEnabled("send_device_command") {
		mcp.AddTool(mcpServer, &mcp.Tool{Name: "send_device_command", Description: "Send an idempotent state command to an online device."}, func(ctx context.Context, _ *mcp.CallToolRequest, input deviceCommandInput) (*mcp.CallToolResult, dataOutput, error) {
			if err := validateWritePolicy(input.controlledWrite); err != nil {
				return nil, dataOutput{}, err
			}
			token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.devices.write")
			if err != nil {
				return nil, dataOutput{}, err
			}
			device, err := s.clients.GetCommandTarget(ctx, input.DeviceID, token)
			if err != nil {
				return nil, dataOutput{}, err
			}
			if !device.Online {
				return nil, dataOutput{}, fmt.Errorf("device is offline")
			}
			result, err := s.clients.devices.Command(ctx, input.DeviceID, input.State, input.TransitionMs, input.IdempotencyKey, token)
			return nil, dataOutput{Data: result}, err
		})
	}
	if principal.hasScope("home.devices.write") && principal.hasScope("home.inventory.read") && s.toolEnabled("send_device_group_command") {
		mcp.AddTool(mcpServer, &mcp.Tool{Name: "send_device_group_command", Description: "Apply an idempotent state command to every device in a Homenavi group."}, func(ctx context.Context, _ *mcp.CallToolRequest, input groupCommandInput) (*mcp.CallToolResult, groupCommandOutput, error) {
			if err := validateWritePolicy(input.controlledWrite); err != nil {
				return nil, groupCommandOutput{}, err
			}
			if len(input.State) == 0 {
				return nil, groupCommandOutput{}, fmt.Errorf("state is required")
			}
			inventoryToken, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.inventory.read")
			if err != nil {
				return nil, groupCommandOutput{}, err
			}
			group, err := s.clients.GetGroup(ctx, input.GroupID, inventoryToken)
			if err != nil {
				return nil, groupCommandOutput{}, err
			}
			if len(group.HDPExternalIDs) == 0 {
				return nil, groupCommandOutput{}, fmt.Errorf("group has no commandable devices")
			}
			token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.devices.write")
			if err != nil {
				return nil, groupCommandOutput{}, err
			}
			members := make([]device, 0, len(group.HDPExternalIDs))
			for _, deviceID := range group.HDPExternalIDs {
				member, err := s.clients.GetCommandTarget(ctx, deviceID, token)
				if err != nil {
					return nil, groupCommandOutput{}, fmt.Errorf("load group member %q: %w", deviceID, err)
				}
				if !member.Online {
					return nil, groupCommandOutput{}, fmt.Errorf("group member %q is offline", deviceID)
				}
				members = append(members, member)
			}
			result := groupCommandOutput{GroupID: group.ID, Results: make([]groupDeviceCommandResult, 0, len(members))}
			for _, member := range members {
				memberResult, err := s.clients.devices.Command(ctx, member.DeviceID, input.State, input.TransitionMs, input.IdempotencyKey+":"+member.DeviceID, token)
				entry := groupDeviceCommandResult{DeviceID: member.DeviceID, Result: memberResult}
				if err != nil {
					entry.Error = err.Error()
				}
				result.Results = append(result.Results, entry)
			}
			return nil, result, nil
		})
	}
	if principal.hasScope("home.automation.execute") && s.toolEnabled("run_automation_workflow") {
		mcp.AddTool(mcpServer, &mcp.Tool{Name: "run_automation_workflow", Description: "Start one idempotent Homenavi automation workflow."}, func(ctx context.Context, _ *mcp.CallToolRequest, input runWorkflowInput) (*mcp.CallToolResult, dataOutput, error) {
			if err := validateWritePolicy(input.controlledWrite); err != nil {
				return nil, dataOutput{}, err
			}
			token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.automation.execute")
			if err != nil {
				return nil, dataOutput{}, err
			}
			result, err := s.clients.RunAutomationWorkflow(ctx, input.WorkflowID, token, input.IdempotencyKey)
			return nil, dataOutput{Data: result}, err
		})
	}
	if principal.hasScope("home.inventory.write") && s.toolEnabled("create_device_group") {
		mcp.AddTool(mcpServer, &mcp.Tool{Name: "create_device_group", Description: "Create an idempotent Homenavi device group."}, func(ctx context.Context, _ *mcp.CallToolRequest, input groupCreateInput) (*mcp.CallToolResult, dataOutput, error) {
			if err := validateWritePolicy(input.controlledWrite); err != nil {
				return nil, dataOutput{}, err
			}
			token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.inventory.write")
			if err != nil {
				return nil, dataOutput{}, err
			}
			result, err := s.clients.CreateGroup(ctx, input, input.IdempotencyKey, token)
			return nil, dataOutput{Data: result}, err
		})
	}
	if principal.hasScope("home.history.read") && s.toolEnabled("query_state_history") {
		mcp.AddTool(mcpServer, &mcp.Tool{Name: "query_state_history", Description: "Query a bounded history of device state observations."}, func(ctx context.Context, _ *mcp.CallToolRequest, input historyInput) (*mcp.CallToolResult, dataOutput, error) {
			token, err := s.clients.exchangeDelegatedToken(ctx, principal.token, s.config.Resource, "home.history.read")
			if err != nil {
				return nil, dataOutput{}, err
			}
			result, err := s.clients.QueryStateHistory(ctx, input, token)
			return nil, dataOutput{Data: result}, err
		})
	}
	return mcpServer
}

func validateWritePolicy(input controlledWrite) error {
	if len(strings.TrimSpace(input.IdempotencyKey)) < 8 {
		return fmt.Errorf("idempotency_key must be at least 8 characters")
	}
	return nil
}

func (s *server) toolEnabled(name string) bool {
	if len(s.config.EnabledTools) == 0 {
		return true
	}
	_, enabled := s.config.EnabledTools[name]
	return enabled
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *server) metadata(w http.ResponseWriter, request *http.Request) {
	if !s.config.Enabled {
		http.NotFound(w, nil)
		return
	}
	resource, err := s.resourceForRequest(request)
	if err != nil {
		http.Error(w, "invalid public resource", http.StatusBadRequest)
		return
	}
	issuer, err := s.authorizationServerIssuer(resource)
	if err != nil {
		http.Error(w, "invalid public resource", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"resource": resource, "authorization_servers": []string{issuer}, "scopes_supported": supportedOAuthScopes})
}

func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.config.Enabled {
			http.NotFound(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if _, allowed := s.config.AllowedOrigins[origin]; !allowed {
				http.Error(w, "origin is not allowed", http.StatusForbidden)
				return
			}
		}
		resource, err := s.resourceForRequest(r)
		if err != nil {
			http.Error(w, "invalid public resource", http.StatusBadRequest)
			return
		}
		bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		principal, err := s.parseToken(bearer, resource)
		if err != nil {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s", scope="%s"`, resourceMetadataURL(resource), strings.Join(supportedOAuthScopes, " ")))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if s.config.SessionValidator != nil {
			active, err := s.config.SessionValidator(r.Context(), principal.sessionID)
			if err != nil || !active {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		allowed, err := s.allowRequest(r.Context(), principal.sessionID)
		if err != nil {
			http.Error(w, "rate limiter unavailable", http.StatusServiceUnavailable)
			return
		}
		if !allowed {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		method, tool := mcpAuditDetails(r)
		auditedWriter := &auditResponseWriter{ResponseWriter: w}
		next.ServeHTTP(auditedWriter, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
		if s.config.Logger != nil {
			s.config.Logger.Info("mcp request completed", "subject", principal.subject, "client_id", principal.clientID, "session_id", principal.sessionID, "request_id", r.Header.Get("X-Request-ID"), "method", method, "tool", tool, "status", auditedWriter.status)
		}
	})
}

func mcpAuditDetails(r *http.Request) (string, string) {
	if r.Body == nil {
		return "", ""
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return "", ""
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var request struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return "", ""
	}
	return request.Method, request.Params.Name
}

func (s *server) allowRequest(ctx context.Context, sessionID string) (bool, error) {
	if s.config.RateLimiter != nil {
		return s.config.RateLimiter(ctx, sessionID)
	}
	return s.limiter.Allow(sessionID), nil
}

func (s *server) resourceForRequest(request *http.Request) (string, error) {
	resource := strings.TrimSpace(s.config.Resource)
	if resource == "" {
		resource = publicMCPResource(request)
	}
	return validMCPResource(resource)
}

func publicMCPResource(request *http.Request) string {
	host := forwardedValue(request.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(request.Host)
	}
	if host == "" {
		return ""
	}
	scheme := forwardedValue(request.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if request.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + host + "/mcp"
}

func forwardedValue(value string) string {
	value, _, _ = strings.Cut(value, ",")
	return strings.TrimSpace(value)
}

func principalFromContext(ctx context.Context) *mcpPrincipal {
	principal, _ := ctx.Value(principalContextKey{}).(*mcpPrincipal)
	return principal
}

func (p *mcpPrincipal) hasScope(scope string) bool {
	if p == nil {
		return false
	}
	_, ok := p.scopes[scope]
	return ok
}

func resourceMetadataURL(resource string) string {
	parsed, err := url.Parse(resource)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "/.well-known/oauth-protected-resource"
	}
	parsed.Path = "/.well-known/oauth-protected-resource"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (s *server) parseToken(tokenString, resource string) (*mcpPrincipal, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("missing bearer token")
	}
	issuer, err := s.authorizationServerIssuer(resource)
	if err != nil {
		return nil, err
	}
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing algorithm")
		}
		expectedKeyID, err := authx.KeyIDForRSAPublicKey(s.config.PublicKey)
		if err != nil || token.Header["kid"] != expectedKeyID {
			return nil, fmt.Errorf("unexpected key ID")
		}
		return s.config.PublicKey, nil
	}, jwt.WithIssuer(issuer), jwt.WithAudience(resource), jwt.WithExpirationRequired(), jwt.WithLeeway(30*time.Second))
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("validate MCP token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims[authx.ClaimTokenType] != authx.TokenTypeMCP {
		return nil, fmt.Errorf("unexpected MCP token type")
	}
	sessionID, _ := claims[authx.ClaimSessionID].(string)
	if sessionID == "" {
		return nil, fmt.Errorf("MCP token has no session ID")
	}
	scopes := authx.ParseScopes(fmt.Sprint(claims[authx.ClaimScope]))
	if len(scopes) == 0 {
		return nil, fmt.Errorf("MCP token has no scopes")
	}
	subject, _ := claims.GetSubject()
	clientID, _ := claims[authx.ClaimAuthorizedParty].(string)
	role, _ := claims["role"].(string)
	if subject == "" || clientID == "" || (role != authx.RoleResident && role != authx.RoleAdmin) {
		return nil, fmt.Errorf("MCP token is missing principal claims")
	}
	return &mcpPrincipal{scopes: scopes, sessionID: sessionID, subject: subject, clientID: clientID, token: tokenString}, nil
}

func (s *server) authorizationServerIssuer(resource string) (string, error) {
	if issuer := strings.TrimSpace(s.config.Issuer); issuer != "" {
		return issuer, nil
	}
	parsed, err := url.Parse(resource)
	if err != nil {
		return "", fmt.Errorf("parse MCP resource: %w", err)
	}
	parsed.Path = "/api/auth"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func validMCPResource(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "/mcp" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("resource must be an absolute /mcp URI")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")) {
		return "", fmt.Errorf("resource must use HTTPS outside local development")
	}
	return parsed.String(), nil
}
