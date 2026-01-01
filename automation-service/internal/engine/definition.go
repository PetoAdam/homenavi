package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Definition is the only supported workflow definition format.
//
// automation is a graph-based workflow:
// - multiple triggers per workflow
// - multiple outgoing edges per node
// - node positions persisted via x/y
//
// The engine treats the graph as a DAG (no cycles). Loop semantics are provided
// via explicit loop nodes (logic.for).
//
// Schema shape:
// {
//   "version": "automation",
//   "nodes": [{"id":"...","kind":"trigger.device_state", "x": 0, "y": 0, "data": {...}}],
//   "edges": [{"from":"a","to":"b"}]
// }
//
// Node kinds:
// - trigger.manual
// - trigger.device_state
// - trigger.schedule
// - action.send_command
// - action.notify_email
// - logic.if
// - logic.sleep
// - logic.for
//
// Edge ordering matters for certain nodes:
// - logic.if: outgoing[0] = then, outgoing[1] = else (optional)
// - logic.for: outgoing[0] = body, outgoing[1] = after (optional)
// - other nodes: all outgoing edges are executed sequentially.
//
// Triggers are entry points; they do not produce WorkflowRunStep records.
// All non-trigger nodes do.
//
// The UI is responsible for generating stable node IDs.
//
// This is stored in store.Workflow.Definition as JSONB.

type Definition struct {
	Version string    `json:"version"`
	Nodes   []NodeDef `json:"nodes"`
	Edges   []EdgeDef `json:"edges"`
}

type NodeDef struct {
	ID   string          `json:"id"`
	Kind string          `json:"kind"`
	X    float64         `json:"x"`
	Y    float64         `json:"y"`
	Data json.RawMessage `json:"data,omitempty"`
}

type EdgeDef struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// --- Node payloads (typed decoding) ---

type TriggerManual struct{}

type TriggerDeviceState struct {
	Targets        NodeTargets     `json:"targets"`
	Key            string          `json:"key,omitempty"`
	Op             string          `json:"op,omitempty"`    // exists|eq|neq|gt|gte|lt|lte
	Value          json.RawMessage `json:"value,omitempty"` // for comparisons
	CooldownSec    int             `json:"cooldown_sec,omitempty"`
	IgnoreRetained bool            `json:"ignore_retained,omitempty"`
}

type TriggerSchedule struct {
	Cron        string `json:"cron"`
	CooldownSec int    `json:"cooldown_sec,omitempty"`
}

type ActionSendCommand struct {
	Targets          NodeTargets    `json:"targets"`
	Command          string         `json:"command"`
	Args             map[string]any `json:"args,omitempty"`
	WaitForResult    bool           `json:"wait_for_result,omitempty"`
	ResultTimeoutSec int            `json:"result_timeout_sec,omitempty"`
}

// NodeTargets defines the runtime target selection for trigger/action nodes.
// v1 supports:
// - {"type":"device","ids":["zigbee/0x...", ...]}
// - {"type":"selector","selector":"tag:kitchen"}
type NodeTargets struct {
	Type     string   `json:"type"`
	IDs      []string `json:"ids,omitempty"`
	Selector string   `json:"selector,omitempty"`
}

type ActionNotifyEmail struct {
	UserIDs     []string               `json:"user_ids"`
	TargetRoles []string               `json:"target_roles,omitempty"`
	Subject     string                 `json:"subject"`
	Message     string                 `json:"message"`
	Recipients  []NotifyEmailRecipient `json:"recipients,omitempty"`
}

type NotifyEmailRecipient struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	UserName string `json:"user_name"`
}

type LogicSleep struct {
	DurationSec int `json:"duration_sec"`
}

type LogicIf struct {
	Path  string          `json:"path"`
	Op    string          `json:"op,omitempty"`    // exists|eq|neq|gt|gte|lt|lte
	Value json.RawMessage `json:"value,omitempty"` // for comparisons
}

type LogicFor struct {
	Count int `json:"count"` // number of iterations; must be >= 0
}

func (d *Definition) NormalizeAndValidate() error {
	if strings.TrimSpace(d.Version) == "" {
		return errors.New("definition.version is required")
	}
	if d.Version != "automation" {
		return errors.New("unsupported definition version")
	}
	if len(d.Nodes) == 0 {
		return errors.New("definition.nodes is required")
	}
	// Build node map.
	nodeByID := map[string]NodeDef{}
	for i := range d.Nodes {
		n := d.Nodes[i]
		n.ID = strings.TrimSpace(n.ID)
		n.Kind = strings.TrimSpace(n.Kind)
		if n.ID == "" {
			return errors.New("definition.nodes[].id is required")
		}
		if n.Kind == "" {
			return fmt.Errorf("definition.nodes[%d].kind is required", i)
		}
		if _, exists := nodeByID[n.ID]; exists {
			return fmt.Errorf("duplicate node id: %s", n.ID)
		}
		nodeByID[n.ID] = n
		// Per-kind validation.
		if err := validateNode(n); err != nil {
			return err
		}
	}
	// Must have at least one trigger.
	triggers := 0
	for _, n := range nodeByID {
		if strings.HasPrefix(strings.ToLower(n.Kind), "trigger.") {
			triggers++
		}
	}
	if triggers == 0 {
		return errors.New("at least one trigger node is required")
	}

	// Validate edges.
	for i := range d.Edges {
		e := d.Edges[i]
		e.From = strings.TrimSpace(e.From)
		e.To = strings.TrimSpace(e.To)
		if e.From == "" || e.To == "" {
			return errors.New("definition.edges[].from and .to are required")
		}
		if e.From == e.To {
			return errors.New("self edges are not allowed")
		}
		if _, ok := nodeByID[e.From]; !ok {
			return fmt.Errorf("edge.from references unknown node: %s", e.From)
		}
		if _, ok := nodeByID[e.To]; !ok {
			return fmt.Errorf("edge.to references unknown node: %s", e.To)
		}
		d.Edges[i] = e
	}

	// Disallow cycles (DAG), with explicit loops handled by logic.for nodes.
	if err := validateAcyclic(nodeByID, d.Edges); err != nil {
		return err
	}

	// Enforce wait_for_result only on leaf action nodes.
	outgoing := buildOutgoing(d.Edges)
	for _, n := range nodeByID {
		if strings.ToLower(n.Kind) != "action.send_command" {
			continue
		}
		var a ActionSendCommand
		_ = json.Unmarshal(n.Data, &a)
		if !a.WaitForResult {
			continue
		}
		// wait_for_result is only supported for single-device targets.
		if err := validateTargets(a.Targets, true); err != nil {
			return fmt.Errorf("action.send_command wait_for_result: %w", err)
		}
		if len(outgoing[n.ID]) != 0 {
			return fmt.Errorf("action.send_command wait_for_result is only supported on leaf nodes (node %s)", n.ID)
		}
	}

	// Write back normalized nodes (trimmed fields).
	norm := make([]NodeDef, 0, len(nodeByID))
	for _, n := range d.Nodes {
		n.ID = strings.TrimSpace(n.ID)
		n.Kind = strings.TrimSpace(n.Kind)
		norm = append(norm, n)
	}
	d.Nodes = norm
	return nil
}

func validateNode(n NodeDef) error {
	kind := strings.ToLower(strings.TrimSpace(n.Kind))
	switch kind {
	case "trigger.manual":
		return nil
	case "trigger.device_state":
		var t TriggerDeviceState
		if err := json.Unmarshal(n.Data, &t); err != nil {
			return fmt.Errorf("trigger.device_state data must be valid json object")
		}
		if err := validateTargets(t.Targets, false); err != nil {
			return fmt.Errorf("trigger.device_state.targets: %w", err)
		}
		t.Op = strings.ToLower(strings.TrimSpace(t.Op))
		if t.Op == "" {
			t.Op = "exists"
		}
		switch t.Op {
		case "exists", "eq", "neq", "gt", "gte", "lt", "lte":
		default:
			return errors.New("unsupported trigger.device_state op")
		}
		return nil
	case "trigger.schedule":
		var t TriggerSchedule
		if err := json.Unmarshal(n.Data, &t); err != nil {
			return fmt.Errorf("trigger.schedule data must be valid json object")
		}
		if strings.TrimSpace(t.Cron) == "" {
			return errors.New("trigger.schedule.cron is required")
		}
		return nil
	case "action.send_command":
		var a ActionSendCommand
		if err := json.Unmarshal(n.Data, &a); err != nil {
			return fmt.Errorf("action.send_command data must be valid json object")
		}
		if err := validateTargets(a.Targets, a.WaitForResult); err != nil {
			return fmt.Errorf("action.send_command.targets: %w", err)
		}
		if strings.TrimSpace(a.Command) == "" {
			a.Command = "set_state"
		}
		if a.ResultTimeoutSec <= 0 {
			a.ResultTimeoutSec = 15
		}
		return nil
	case "action.notify_email":
		var a ActionNotifyEmail
		if err := json.Unmarshal(n.Data, &a); err != nil {
			return fmt.Errorf("action.notify_email data must be valid json object")
		}
		userCount := 0
		for _, id := range a.UserIDs {
			if strings.TrimSpace(id) == "" {
				return errors.New("action.notify_email.user_ids must not contain empty values")
			}
			userCount++
		}
		roleCount := 0
		for _, r := range a.TargetRoles {
			r = strings.ToLower(strings.TrimSpace(r))
			if r == "" {
				continue
			}
			if r != "admin" && r != "resident" {
				return errors.New("action.notify_email.target_roles supports: admin,resident")
			}
			roleCount++
		}
		if userCount == 0 && roleCount == 0 {
			return errors.New("action.notify_email requires at least one user_id or target role")
		}
		if strings.TrimSpace(a.Subject) == "" {
			return errors.New("action.notify_email.subject is required")
		}
		if strings.TrimSpace(a.Message) == "" {
			return errors.New("action.notify_email.message is required")
		}
		return nil
	case "logic.sleep":
		var s LogicSleep
		if err := json.Unmarshal(n.Data, &s); err != nil {
			return fmt.Errorf("logic.sleep data must be valid json object")
		}
		if s.DurationSec < 0 {
			return errors.New("logic.sleep.duration_sec must be >= 0")
		}
		return nil
	case "logic.if":
		var c LogicIf
		if err := json.Unmarshal(n.Data, &c); err != nil {
			return fmt.Errorf("logic.if data must be valid json object")
		}
		if strings.TrimSpace(c.Path) == "" {
			return errors.New("logic.if.path is required")
		}
		c.Op = strings.ToLower(strings.TrimSpace(c.Op))
		if c.Op == "" {
			c.Op = "exists"
		}
		switch c.Op {
		case "exists", "eq", "neq", "gt", "gte", "lt", "lte":
		default:
			return errors.New("unsupported logic.if op")
		}
		return nil
	case "logic.for":
		var f LogicFor
		if err := json.Unmarshal(n.Data, &f); err != nil {
			return fmt.Errorf("logic.for data must be valid json object")
		}
		if f.Count < 0 {
			return errors.New("logic.for.count must be >= 0")
		}
		return nil
	default:
		return fmt.Errorf("unsupported node kind: %s", n.Kind)
	}
}

func buildOutgoing(edges []EdgeDef) map[string][]string {
	out := map[string][]string{}
	for _, e := range edges {
		out[e.From] = append(out[e.From], e.To)
	}
	return out
}

func validateAcyclic(nodes map[string]NodeDef, edges []EdgeDef) error {
	out := buildOutgoing(edges)
	state := map[string]int{} // 0=unvisited,1=visiting,2=done
	var visit func(id string) error
	visit = func(id string) error {
		s := state[id]
		if s == 1 {
			return fmt.Errorf("workflow contains a cycle involving %s", id)
		}
		if s == 2 {
			return nil
		}
		state[id] = 1
		for _, to := range out[id] {
			if _, ok := nodes[to]; !ok {
				continue
			}
			if err := visit(to); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range nodes {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func validateTargets(t NodeTargets, requireSingleDevice bool) error {
	typ := strings.ToLower(strings.TrimSpace(t.Type))
	switch typ {
	case "device":
		ids := make([]string, 0, len(t.IDs))
		seen := map[string]struct{}{}
		for _, raw := range t.IDs {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			return errors.New("targets.ids is required")
		}
		if requireSingleDevice && len(ids) != 1 {
			return errors.New("targets.ids must contain exactly 1 device when wait_for_result is enabled")
		}
		return nil
	case "selector":
		sel := strings.TrimSpace(t.Selector)
		if sel == "" {
			return errors.New("targets.selector is required")
		}
		if requireSingleDevice {
			return errors.New("wait_for_result is not supported for selector targets")
		}
		return nil
	default:
		if typ == "" {
			return errors.New("targets.type is required")
		}
		return errors.New("unsupported targets.type")
	}
}
