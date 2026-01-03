package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/homenavi/assistant-service/internal/config"
)

// ErrInsufficientRole indicates the user doesn't have permission for a tool
var ErrInsufficientRole = errors.New("insufficient role for this action")

// Tool represents a function the assistant can call
type Tool struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Parameters   map[string]interface{} `json:"parameters"`
	RequiredRole string                 `json:"-"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ToolHandler is a function that executes a tool
type ToolHandler func(ctx context.Context, params map[string]interface{}, userRole string, userID string) (*ToolResult, error)

// Registry manages available tools
type Registry struct {
	tools    map[string]Tool
	handlers map[string]ToolHandler
	config   *config.Config
	client   *http.Client

	cacheMu        sync.Mutex
	cacheTTL       time.Duration
	cachedMerged   []map[string]interface{}
	cachedMergedAt time.Time
	cachedRooms    []map[string]interface{}
	cachedRoomsAt  time.Time
}

// NewRegistry creates a new tool registry
func NewRegistry(cfg *config.Config) *Registry {
	r := &Registry{
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
		config:   cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cacheTTL: 2 * time.Second,
	}
	r.registerBuiltinTools()
	return r
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool, handler ToolHandler) {
	r.tools[tool.Name] = tool
	r.handlers[tool.Name] = handler
}

// roleHierarchy defines role levels
var roleHierarchy = map[string]int{
	"user":     1,
	"resident": 2,
	"admin":    3,
	"service":  4,
}

// FilterByRole returns tools available to a role
func (r *Registry) FilterByRole(userRole string) []Tool {
	userLevel := roleHierarchy[userRole]
	var allowed []Tool

	for _, tool := range r.tools {
		requiredLevel := roleHierarchy[tool.RequiredRole]
		if userLevel >= requiredLevel {
			allowed = append(allowed, tool)
		}
	}
	return allowed
}

// CanUse checks if a role can use a tool
func (r *Registry) CanUse(toolName, userRole string) bool {
	tool, exists := r.tools[toolName]
	if !exists {
		return false
	}
	return roleHierarchy[userRole] >= roleHierarchy[tool.RequiredRole]
}

// Execute runs a tool with the given parameters
func (r *Registry) Execute(ctx context.Context, toolName string, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	tool, exists := r.tools[toolName]
	if !exists {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
	userLevel := roleHierarchy[userRole]
	requiredLevel := roleHierarchy[tool.RequiredRole]
	if userLevel < requiredLevel {
		return nil, fmt.Errorf("%w: requires role %s (you are %s)", ErrInsufficientRole, tool.RequiredRole, userRole)
	}

	handler, exists := r.handlers[toolName]
	if !exists {
		return nil, fmt.Errorf("unknown tool handler: %s", toolName)
	}

	return handler(ctx, params, userRole, userID)
}

// GetToolsForPrompt returns a formatted list of tools for the system prompt
func (r *Registry) GetToolsForPrompt(userRole string) string {
	tools := r.FilterByRole(userRole)
	var lines []string
	for _, tool := range tools {
		lines = append(lines, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
	}
	return strings.Join(lines, "\n")
}

// GetToolDefinitions returns Ollama-compatible tool definitions for function calling
func (r *Registry) GetToolDefinitions(userRole string) []map[string]interface{} {
	tools := r.FilterByRole(userRole)
	var defs []map[string]interface{}
	for _, tool := range tools {
		defs = append(defs, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		})
	}
	return defs
}

// GetAllToolDefinitions returns tool definitions without role filtering.
// Authorization is enforced when executing tools.
func (r *Registry) GetAllToolDefinitions() []map[string]interface{} {
	defs := make([]map[string]interface{}, 0, len(r.tools))
	for _, tool := range r.tools {
		defs = append(defs, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		})
	}
	return defs
}

func (r *Registry) registerBuiltinTools() {
	// List devices
	r.Register(Tool{
		Name:         "list_devices",
		Description:  "List devices with merged ERS+HDP data (room/name from ERS + state/capabilities from HDP). Use this before get_device_state/control_device.",
		RequiredRole: "resident",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"type":  map[string]string{"type": "string", "description": "Optional filter by device type"},
				"room":  map[string]string{"type": "string", "description": "Optional filter by room name/slug (e.g., 'Bedroom', 'living-room')"},
				"state": map[string]string{"type": "string", "description": "Optional filter by state (best-effort, e.g. 'ON'/'OFF'/'true'/'false')"},
			},
		},
	}, r.handleListDevices)

	// Find devices (natural-language search)
	r.Register(Tool{
		Name:         "find_devices",
		Description:  "Find devices by a natural-language query (name/description/model/etc) against the merged ERS+HDP view. Returns best matches with device_id.",
		RequiredRole: "resident",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]string{"type": "string", "description": "Search phrase (e.g., 'rgb lamp', 'bedroom sensor')"},
				"room":  map[string]string{"type": "string", "description": "Optional room name/slug to narrow results"},
				"limit": map[string]string{"type": "integer", "description": "Max matches to return (default 5, max 20)"},
			},
			"required": []string{"query"},
		},
	}, r.handleFindDevices)

	// Control device
	r.Register(Tool{
		Name:         "control_device",
		Description:  "Send a state patch to a device via HDP. Preferred params: device_id + state (object). Example: {device_id:'zigbee/0x..', state:{state:'ON'}}. Use list_devices/get_device_state to see capabilities/inputs.",
		RequiredRole: "resident",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"device_id":     map[string]string{"type": "string", "description": "Exact device_id from list_devices (e.g., 'zigbee/0x...')"},
				"state":         map[string]interface{}{"type": "object", "description": "State patch to apply (e.g., {state:'ON', brightness:128})"},
				"transition_ms": map[string]string{"type": "integer", "description": "Optional transition time in ms"},
				"capability_id": map[string]string{"type": "string", "description": "(Compatibility) Single state key to set, e.g. 'state' or 'brightness'"},
				"action":        map[string]string{"type": "string", "description": "(Legacy) on/off/toggle/set"},
				"value":         map[string]string{"type": "any", "description": "(Legacy/Compatibility) value for set (or for capability_id)"},
			},
			"required": []string{"device_id"},
		},
	}, r.handleControlDevice)

	// Get device state
	r.Register(Tool{
		Name:         "get_device_state",
		Description:  "Get a single device with merged ERS+HDP data (room/name + current state + capabilities/inputs).",
		RequiredRole: "resident",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"device_id": map[string]string{"type": "string", "description": "Exact device_id from list_devices"},
			},
			"required": []string{"device_id"},
		},
	}, r.handleGetDeviceState)

	// Room metric helper
	r.Register(Tool{
		Name:         "get_room_metric",
		Description:  "Get temperature or humidity for a room using merged ERS+HDP data. Returns readings + which device(s) they came from.",
		RequiredRole: "resident",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"room":   map[string]string{"type": "string", "description": "Room name or slug (e.g., 'Bedroom', 'living-room')"},
				"metric": map[string]string{"type": "string", "description": "Metric: 'temperature' or 'humidity'"},
			},
			"required": []string{"room", "metric"},
		},
	}, r.handleGetRoomMetric)

	// List rooms
	r.Register(Tool{
		Name:         "list_rooms",
		Description:  "List rooms from ERS (id, name, slug). Use this to resolve room names/slugs before querying room metrics.",
		RequiredRole: "resident",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, r.handleListRooms)

	// List automations
	r.Register(Tool{
		Name:         "list_automations",
		Description:  "List all automation rules configured in the system.",
		RequiredRole: "resident",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, r.handleListAutomations)

	// Get current time
	r.Register(Tool{
		Name:         "get_current_time",
		Description:  "Get the current date and time.",
		RequiredRole: "user",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, r.handleGetCurrentTime)

	// Get system status
	r.Register(Tool{
		Name:         "get_system_status",
		Description:  "Get overall system status including number of online devices, active automations, and alerts.",
		RequiredRole: "resident",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, r.handleGetSystemStatus)

}

func (r *Registry) listRoomsCached(ctx context.Context) ([]map[string]interface{}, map[string]string, map[string]string, error) {
	r.cacheMu.Lock()
	if len(r.cachedRooms) > 0 && time.Since(r.cachedRoomsAt) < r.cacheTTL {
		rooms := r.cachedRooms
		r.cacheMu.Unlock()
		nameByID := make(map[string]string)
		slugByID := make(map[string]string)
		for _, room := range rooms {
			id := toString(room["id"])
			if id == "" {
				continue
			}
			nameByID[id] = toString(room["name"])
			slugByID[id] = toString(room["slug"])
		}
		return rooms, nameByID, slugByID, nil
	}
	r.cacheMu.Unlock()

	roomsReq, _ := http.NewRequestWithContext(ctx, "GET", r.config.ERSURL+"/api/ers/rooms", nil)
	roomsResp, err := r.client.Do(roomsReq)
	if err != nil {
		return nil, nil, nil, err
	}
	defer roomsResp.Body.Close()
	roomsBody, _ := io.ReadAll(roomsResp.Body)
	if roomsResp.StatusCode != http.StatusOK {
		return nil, nil, nil, fmt.Errorf("ERS rooms error: %s", string(roomsBody))
	}

	var rooms []map[string]interface{}
	if err := json.Unmarshal(roomsBody, &rooms); err != nil {
		return nil, nil, nil, err
	}

	// Normalize to a stable minimal schema
	out := make([]map[string]interface{}, 0, len(rooms))
	nameByID := make(map[string]string)
	slugByID := make(map[string]string)
	for _, room := range rooms {
		id := toString(room["id"])
		name := toString(room["name"])
		slug := toString(room["slug"])
		out = append(out, map[string]interface{}{
			"id":   id,
			"name": name,
			"slug": slug,
		})
		if id != "" {
			nameByID[id] = name
			slugByID[id] = slug
		}
	}

	r.cacheMu.Lock()
	r.cachedRooms = out
	r.cachedRoomsAt = time.Now()
	r.cacheMu.Unlock()

	return out, nameByID, slugByID, nil
}

func (r *Registry) handleListRooms(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	out, _, _, err := r.listRoomsCached(ctx)
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to fetch rooms: " + err.Error()}, nil
	}
	return &ToolResult{Success: true, Data: out}, nil
}

func normalizeKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toBool(v any) (bool, bool) {
	if v == nil {
		return false, false
	}
	if b, ok := v.(bool); ok {
		return b, true
	}
	return false, false
}

func stateMatchesFilter(state any, want string) bool {
	want = strings.TrimSpace(strings.ToLower(want))
	if want == "" || want == "any" {
		return true
	}
	// Best-effort: look for state["state"] or boolean-ish state["state"]
	st, ok := state.(map[string]interface{})
	if !ok {
		return false
	}
	if raw, ok := st["state"]; ok {
		if b, okb := toBool(raw); okb {
			return (want == "on" || want == "true") == b
		}
		if s, oks := raw.(string); oks {
			s = strings.ToLower(strings.TrimSpace(s))
			return s == want
		}
	}
	return false
}

// mergedDevices returns a cached merged ERS+HDP view keyed by device_id.
// Intended for prompt/snapshot building and list-like queries.
func (r *Registry) mergedDevices(ctx context.Context) ([]map[string]interface{}, error) {
	r.cacheMu.Lock()
	if len(r.cachedMerged) > 0 && time.Since(r.cachedMergedAt) < r.cacheTTL {
		cached := r.cachedMerged
		r.cacheMu.Unlock()
		return cached, nil
	}
	r.cacheMu.Unlock()

	fresh, err := r.mergedDevicesFresh(ctx)
	if err != nil {
		return nil, err
	}
	r.cacheMu.Lock()
	r.cachedMerged = fresh
	r.cachedMergedAt = time.Now()
	r.cacheMu.Unlock()
	return fresh, nil
}

// mergedDevicesFresh always fetches from upstream services (HDP+ERS).
// Use this for confirmations and precise state checks.
func (r *Registry) mergedDevicesFresh(ctx context.Context) ([]map[string]interface{}, error) {
	// HDP devices (state/capabilities/inputs)
	hdpReq, _ := http.NewRequestWithContext(ctx, "GET", r.config.DeviceHubURL+"/api/hdp/devices", nil)
	hdpResp, err := r.client.Do(hdpReq)
	if err != nil {
		return nil, err
	}
	defer hdpResp.Body.Close()
	hdpBody, _ := io.ReadAll(hdpResp.Body)
	if hdpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device hub error: %s", string(hdpBody))
	}
	var hdpDevices []map[string]interface{}
	if err := json.Unmarshal(hdpBody, &hdpDevices); err != nil {
		return nil, err
	}

	// ERS devices (room/name mapping)
	ersReq, _ := http.NewRequestWithContext(ctx, "GET", r.config.ERSURL+"/api/ers/devices", nil)
	ersResp, err := r.client.Do(ersReq)
	var ersDevices []map[string]interface{}
	if err == nil {
		defer ersResp.Body.Close()
		if ersResp.StatusCode == http.StatusOK {
			ersBody, _ := io.ReadAll(ersResp.Body)
			_ = json.Unmarshal(ersBody, &ersDevices)
		}
	}

	// ERS rooms (id -> name/slug)
	_, roomNameByID, roomSlugByID, _ := r.listRoomsCached(ctx)

	// ERS lookup by hdp_external_id
	ersByHDPID := make(map[string]map[string]interface{})
	for _, ersDev := range ersDevices {
		if ids, ok := ersDev["hdp_external_ids"].([]interface{}); ok {
			for _, raw := range ids {
				if id, ok := raw.(string); ok && id != "" {
					ersByHDPID[id] = ersDev
				}
			}
		}
	}

	mergedByID := make(map[string]map[string]interface{}, len(hdpDevices))
	for _, hdpDev := range hdpDevices {
		deviceID := toString(hdpDev["device_id"])
		if strings.TrimSpace(deviceID) == "" {
			continue
		}
		item := map[string]interface{}{
			"device_id":    deviceID,
			"description":  hdpDev["description"],
			"type":         hdpDev["type"],
			"manufacturer": hdpDev["manufacturer"],
			"model":        hdpDev["model"],
			"online":       hdpDev["online"],
			"state":        hdpDev["state"],
			"capabilities": hdpDev["capabilities"],
			"inputs":       hdpDev["inputs"],
		}
		if ersDev, ok := ersByHDPID[deviceID]; ok {
			item["ers_id"] = ersDev["id"]
			if name := toString(ersDev["name"]); name != "" {
				item["name"] = name
			}
			roomID := toString(ersDev["room_id"])
			if roomID != "" {
				item["room_id"] = roomID
				if rn := roomNameByID[roomID]; rn != "" {
					item["room"] = rn
				}
				if rs := roomSlugByID[roomID]; rs != "" {
					item["room_slug"] = rs
				}
			}
		}
		mergedByID[deviceID] = item
	}

	merged := make([]map[string]interface{}, 0, len(mergedByID))
	for _, item := range mergedByID {
		merged = append(merged, item)
	}
	return merged, nil
}

func (r *Registry) handleListDevices(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	devices, err := r.mergedDevices(ctx)
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to fetch devices: " + err.Error()}, nil
	}

	roomFilter := normalizeKey(toString(params["room"]))
	typeFilter := normalizeKey(toString(params["type"]))
	stateFilter := strings.TrimSpace(toString(params["state"]))

	if roomFilter == "" && typeFilter == "" && stateFilter == "" {
		return &ToolResult{Success: true, Data: devices}, nil
	}

	filtered := make([]map[string]interface{}, 0, len(devices))
	for _, d := range devices {
		if typeFilter != "" {
			if normalizeKey(toString(d["type"])) != typeFilter {
				continue
			}
		}
		if roomFilter != "" {
			rn := normalizeKey(toString(d["room"]))
			rs := normalizeKey(toString(d["room_slug"]))
			if rn != roomFilter && rs != roomFilter {
				continue
			}
		}
		if stateFilter != "" {
			if !stateMatchesFilter(d["state"], stateFilter) {
				continue
			}
		}
		filtered = append(filtered, d)
	}
	return &ToolResult{Success: true, Data: filtered}, nil
}

func (r *Registry) handleFindDevices(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	query := strings.TrimSpace(toString(params["query"]))
	if query == "" {
		return &ToolResult{Success: false, Error: "query is required"}, nil
	}
	roomFilter := normalizeKey(toString(params["room"]))
	limit := 5
	if lf, ok := params["limit"].(float64); ok && int(lf) > 0 {
		limit = int(lf)
	}
	if limit > 20 {
		limit = 20
	}

	devices, err := r.mergedDevices(ctx)
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to fetch devices: " + err.Error()}, nil
	}

	normalize := func(in string) string {
		in = strings.ToLower(strings.TrimSpace(in))
		in = strings.ReplaceAll(in, "_", " ")
		in = strings.ReplaceAll(in, "-", " ")
		in = strings.Join(strings.Fields(in), " ")
		return in
	}
	stop := map[string]struct{}{
		"the": {}, "a": {}, "an": {}, "please": {}, "my": {}, "to": {}, "in": {},
		"on": {}, "off": {}, "turn": {}, "switch": {}, "set": {}, "make": {}, "device": {},
	}
	qNorm := normalize(query)
	var tokens []string
	for _, t := range strings.Fields(qNorm) {
		if _, ok := stop[t]; ok {
			continue
		}
		tokens = append(tokens, t)
	}

	type scored struct {
		score int
		dev   map[string]interface{}
	}
	var cands []scored
	for _, d := range devices {
		rn := normalizeKey(toString(d["room"]))
		rs := normalizeKey(toString(d["room_slug"]))
		if roomFilter != "" && rn != roomFilter && rs != roomFilter {
			continue
		}
		fields := []string{
			toString(d["name"]),
			toString(d["description"]),
			toString(d["device_id"]),
			toString(d["model"]),
			toString(d["manufacturer"]),
		}
		score := 0
		for _, f := range fields {
			fn := normalize(f)
			if fn == "" {
				continue
			}
			if qNorm != "" && strings.Contains(fn, qNorm) {
				score += 6
			}
			for _, tok := range tokens {
				if tok != "" && strings.Contains(fn, tok) {
					score += 2
				}
			}
		}
		if score <= 0 {
			continue
		}
		dev := map[string]interface{}{
			"device_id":   d["device_id"],
			"name":        d["name"],
			"description": d["description"],
			"room":        d["room"],
			"room_slug":   d["room_slug"],
			"type":        d["type"],
			"online":      d["online"],
		}
		cands = append(cands, scored{score: score, dev: dev})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > limit {
		cands = cands[:limit]
	}
	out := make([]map[string]interface{}, 0, len(cands))
	for _, c := range cands {
		c.dev["score"] = c.score
		out = append(out, c.dev)
	}

	return &ToolResult{Success: true, Data: map[string]interface{}{
		"query":   query,
		"room":    params["room"],
		"matches": out,
		"count":   len(out),
	}}, nil
}

func (r *Registry) handleControlDevice(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	deviceID, ok := params["device_id"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "device_id is required"}, nil
	}

	// Preferred: explicit state patch
	state := make(map[string]interface{})
	if sm, ok := params["state"].(map[string]interface{}); ok {
		for k, v := range sm {
			if strings.TrimSpace(k) != "" {
				state[k] = v
			}
		}
	}

	// Back-compat: action/value
	if len(state) == 0 {
		action, _ := params["action"].(string)
		value := params["value"]
		switch strings.ToLower(strings.TrimSpace(action)) {
		case "on":
			state["state"] = "ON"
		case "off":
			state["state"] = "OFF"
		case "toggle":
			state["state"] = "TOGGLE"
		case "set":
			if valueMap, ok := value.(map[string]interface{}); ok {
				for k, v := range valueMap {
					state[k] = v
				}
			} else if value != nil {
				state["brightness"] = value
			}
		default:
			if action != "" && value != nil {
				state[action] = value
			} else if action != "" {
				state["state"] = strings.ToUpper(action)
			}
		}
	}

	// Compatibility: capability_id + value
	if len(state) == 0 {
		capID := strings.TrimSpace(toString(params["capability_id"]))
		val := params["value"]
		if capID != "" {
			if strings.EqualFold(capID, "state") {
				if b, ok := toBool(val); ok {
					if b {
						state["state"] = "ON"
					} else {
						state["state"] = "OFF"
					}
				} else {
					state["state"] = strings.ToUpper(strings.TrimSpace(toString(val)))
				}
			} else {
				state[capID] = val
			}
		}
	}

	// Compatibility: property + value (some clients/models use this shape)
	if len(state) == 0 {
		prop := strings.TrimSpace(toString(params["property"]))
		val := params["value"]
		if prop != "" {
			if strings.EqualFold(prop, "state") {
				if b, ok := toBool(val); ok {
					if b {
						state["state"] = "ON"
					} else {
						state["state"] = "OFF"
					}
				} else {
					state["state"] = strings.ToUpper(strings.TrimSpace(toString(val)))
				}
			} else {
				state[prop] = val
			}
		}
	}

	if len(state) == 0 {
		return &ToolResult{Success: false, Error: "state is required (e.g. {state:{state:'ON'}})"}, nil
	}

	payload := map[string]interface{}{"state": state}
	if tm, ok := params["transition_ms"].(float64); ok { // JSON numbers decode as float64
		payload["transition_ms"] = int(tm)
	}

	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/hdp/devices/%s/commands", r.config.DeviceHubURL, deviceID),
		strings.NewReader(string(payloadBytes)),
	)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to send command: " + err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return &ToolResult{Success: false, Error: "Command failed: " + string(body)}, nil
	}

	// Best-effort confirmation: if we're setting on/off state, poll until HDP reflects it.
	wantRaw, hasWant := state["state"]
	if hasWant {
		wantStr := ""
		wantBool, wantIsBool := toBool(wantRaw)
		if s, ok := wantRaw.(string); ok {
			wantStr = strings.ToUpper(strings.TrimSpace(s))
			if wantStr == "ON" {
				wantBool, wantIsBool = true, true
			}
			if wantStr == "OFF" {
				wantBool, wantIsBool = false, true
			}
		}

		if wantIsBool || wantStr == "ON" || wantStr == "OFF" {
			deadline := time.Now().Add(3 * time.Second)
			for {
				devices, err := r.mergedDevicesFresh(ctx)
				if err == nil {
					for _, d := range devices {
						if toString(d["device_id"]) != deviceID {
							continue
						}
						st, _ := d["state"].(map[string]interface{})
						cur, ok := st["state"]
						if ok {
							if b, okb := toBool(cur); okb && wantIsBool {
								if b == wantBool {
									return &ToolResult{Success: true, Data: fmt.Sprintf("Confirmed state=%v for device %s", wantBool, deviceID)}, nil
								}
							}
							if cs, oks := cur.(string); oks && wantStr != "" {
								if strings.ToUpper(strings.TrimSpace(cs)) == wantStr {
									return &ToolResult{Success: true, Data: fmt.Sprintf("Confirmed state=%s for device %s", wantStr, deviceID)}, nil
								}
							}
						}
					}
				}

				if time.Now().After(deadline) {
					return &ToolResult{Success: false, Error: "Command accepted but device state did not confirm within 3s"}, nil
				}
				time.Sleep(400 * time.Millisecond)
			}
		}
	}

	return &ToolResult{
		Success: true,
		Data:    fmt.Sprintf("Successfully sent command to device %s: %v", deviceID, state),
	}, nil
}

func (r *Registry) handleGetDeviceState(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	deviceID, ok := params["device_id"].(string)
	if !ok {
		return &ToolResult{Success: false, Error: "device_id is required"}, nil
	}

	devices, err := r.mergedDevicesFresh(ctx)
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to fetch devices: " + err.Error()}, nil
	}
	for _, d := range devices {
		if toString(d["device_id"]) == deviceID {
			return &ToolResult{Success: true, Data: d}, nil
		}
	}
	return &ToolResult{Success: false, Error: "Device not found"}, nil
}

func (r *Registry) handleGetRoomMetric(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	room := normalizeKey(toString(params["room"]))
	metric := normalizeKey(toString(params["metric"]))
	if room == "" || metric == "" {
		return &ToolResult{Success: false, Error: "room and metric are required"}, nil
	}
	if metric == "temp" {
		metric = "temperature"
	}
	if metric != "temperature" && metric != "humidity" {
		return &ToolResult{Success: false, Error: "metric must be 'temperature' or 'humidity'"}, nil
	}

	devices, err := r.mergedDevicesFresh(ctx)
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to fetch devices: " + err.Error()}, nil
	}

	readings := make([]map[string]interface{}, 0)
	for _, d := range devices {
		rn := normalizeKey(toString(d["room"]))
		rs := normalizeKey(toString(d["room_slug"]))
		if rn != room && rs != room {
			continue
		}
		st, ok := d["state"].(map[string]interface{})
		if !ok {
			continue
		}
		if val, ok := st[metric]; ok {
			readings = append(readings, map[string]interface{}{
				"device_id":   d["device_id"],
				"name":        d["name"],
				"description": d["description"],
				"value":       val,
				"unit":        map[string]string{"temperature": "°C", "humidity": "%"}[metric],
			})
		}
	}

	return &ToolResult{Success: true, Data: map[string]interface{}{
		"room":     params["room"],
		"metric":   metric,
		"readings": readings,
	}}, nil
}

func (r *Registry) handleListAutomations(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	resp, err := r.client.Get(r.config.AutomationURL + "/api/automation/rules")
	if err != nil {
		return &ToolResult{Success: false, Error: "Failed to fetch automations: " + err.Error()}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return &ToolResult{Success: false, Error: "Automation service error: " + string(body)}, nil
	}

	var automations interface{}
	json.Unmarshal(body, &automations)

	return &ToolResult{Success: true, Data: automations}, nil
}

func (r *Registry) handleGetCurrentTime(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	now := time.Now().Local()
	return &ToolResult{
		Success: true,
		Data: map[string]string{
			"time":     now.Format("15:04:05"),
			"date":     now.Format("2006-01-02"),
			"datetime": now.Format(time.RFC3339),
			"weekday":  now.Weekday().String(),
			"timezone": now.Location().String(),
		},
	}, nil
}

func (r *Registry) handleGetSystemStatus(ctx context.Context, params map[string]interface{}, userRole, userID string) (*ToolResult, error) {
	// This would aggregate status from multiple services
	// For now, return a basic status
	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"status":     "operational",
			"assistant":  "online",
			"checked_at": time.Now().Format(time.RFC3339),
		},
	}, nil
}
