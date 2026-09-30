package app

import (
	"encoding/json"
	"fmt"

	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
	dbinfra "github.com/PetoAdam/homenavi/automation-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/mockdemo"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

func demoWorkflows() ([]dbinfra.Workflow, error) {
	definitions := []struct {
		id        string
		name      string
		sortOrder int
	}{
		{id: "c358553a-7174-49b6-bcaa-44c30293ecf2", name: "Good Morning", sortOrder: 300},
		{id: "0f3fbc02-1e54-4b30-a40f-b871d39cff8f", name: "Movie Night", sortOrder: 200},
		{id: "8b40db6f-c7b3-4fe4-a716-95f57642d533", name: "Leave Home Checklist", sortOrder: 100},
	}
	workflows := make([]dbinfra.Workflow, 0, len(definitions))
	for _, item := range definitions {
		definition, err := demoWorkflowDefinition(item.name)
		if err != nil {
			return nil, fmt.Errorf("workflow %s: %w", item.name, err)
		}
		workflows = append(workflows, dbinfra.Workflow{
			ID:             mustDemoWorkflowUUID(item.id),
			Name:           item.name,
			SortOrder:      item.sortOrder,
			Enabled:        true,
			Definition:     definition,
			SourceKind:     "graph",
			SourceFormat:   "graph-json",
			SourceCode:     "",
			SourceRevision: 1,
			CreatedBy:      "demo-seed",
		})
	}
	return workflows, nil
}

func demoWorkflowDefinition(name string) (datatypes.JSON, error) {
	var definition engine.Definition
	switch name {
	case "Good Morning":
		definition = engine.Definition{
			Version: "automation",
			Nodes: []engine.NodeDef{
				{ID: "manual-trigger", Kind: "trigger.manual", X: 120, Y: 120, Data: json.RawMessage(`{}`)},
				{ID: "blind-open", Kind: "action.send_command", X: 410, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "device", "ids": []string{mockdemo.BedroomBlindHDPDeviceID}}, "command": "set_state", "args": map[string]any{"position": 18}})},
				{ID: "lights-on", Kind: "action.send_command", X: 750, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "device", "ids": []string{mockdemo.SofaLampHDPDeviceID, mockdemo.TVBacklightHDPDeviceID, mockdemo.KitchenPendantHDPDeviceID, mockdemo.DeskLampHDPDeviceID}}, "command": "set_state", "args": map[string]any{"on": true, "brightness": 58}})},
				{ID: "purifier-auto", Kind: "action.send_command", X: 1090, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "device", "ids": []string{mockdemo.AirPurifierHDPDeviceID}}, "command": "set_state", "args": map[string]any{"on": true, "fan_speed": "auto"}})},
			},
			Edges: []engine.EdgeDef{{From: "manual-trigger", To: "blind-open"}, {From: "blind-open", To: "lights-on"}, {From: "lights-on", To: "purifier-auto"}},
		}
	case "Movie Night":
		definition = engine.Definition{
			Version: "automation",
			Nodes: []engine.NodeDef{
				{ID: "manual-trigger", Kind: "trigger.manual", X: 120, Y: 120, Data: json.RawMessage(`{}`)},
				{ID: "main-lights-dim", Kind: "action.send_command", X: 410, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "device", "ids": []string{mockdemo.SofaLampHDPDeviceID, mockdemo.TVBacklightHDPDeviceID, mockdemo.KitchenPendantHDPDeviceID, mockdemo.DeskLampHDPDeviceID}}, "command": "set_state", "args": map[string]any{"on": true, "brightness": 22}})},
				{ID: "tv-accent", Kind: "action.send_command", X: 750, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "device", "ids": []string{mockdemo.TVBacklightHDPDeviceID}}, "command": "set_state", "args": map[string]any{"on": true, "brightness": 34}})},
				{ID: "blind-close", Kind: "action.send_command", X: 1090, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "device", "ids": []string{mockdemo.BedroomBlindHDPDeviceID}}, "command": "set_state", "args": map[string]any{"position": 86}})},
			},
			Edges: []engine.EdgeDef{{From: "manual-trigger", To: "main-lights-dim"}, {From: "main-lights-dim", To: "tv-accent"}, {From: "tv-accent", To: "blind-close"}},
		}
	case "Leave Home Checklist":
		definition = engine.Definition{
			Version: "automation",
			Nodes: []engine.NodeDef{
				{ID: "manual-trigger", Kind: "trigger.manual", X: 120, Y: 120, Data: json.RawMessage(`{}`)},
				{ID: "lights-off", Kind: "action.send_command", X: 410, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "selector", "selector": "group:evening-lights"}, "command": "set_state", "args": map[string]any{"on": false, "brightness": 0}})},
				{ID: "coffee-off", Kind: "action.send_command", X: 750, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "device", "ids": []string{mockdemo.CoffeeMakerHDPDeviceID}}, "command": "set_state", "args": map[string]any{"power": false}})},
				{ID: "purifier-low", Kind: "action.send_command", X: 1090, Y: 80, Data: mustJSON(map[string]any{"targets": map[string]any{"type": "device", "ids": []string{mockdemo.AirPurifierHDPDeviceID}}, "command": "set_state", "args": map[string]any{"on": true, "fan_speed": "low"}})},
			},
			Edges: []engine.EdgeDef{{From: "manual-trigger", To: "lights-off"}, {From: "lights-off", To: "coffee-off"}, {From: "coffee-off", To: "purifier-low"}},
		}
	default:
		definition = engine.Definition{
			Version: "automation",
			Nodes: []engine.NodeDef{{ID: "manual-trigger", Kind: "trigger.manual", X: 120, Y: 120, Data: json.RawMessage(`{}`)}},
			Edges:   []engine.EdgeDef{},
		}
	}
	if err := definition.NormalizeAndValidate(); err != nil {
		return nil, fmt.Errorf("normalize %s: %w", name, err)
	}
	b, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(b), nil
}

func mustJSON(value any) json.RawMessage {
	b, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(b)
}

func mustDemoWorkflowUUID(raw string) uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil {
		panic(err)
	}
	return id
}