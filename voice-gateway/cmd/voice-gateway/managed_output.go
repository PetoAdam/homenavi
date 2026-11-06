package main

import (
	"encoding/json"
	"errors"
	"strings"
)

type managedOutput struct {
	order []string
	items map[string]string
}

func newManagedOutput() *managedOutput {
	return &managedOutput{items: make(map[string]string)}
}

func (output *managedOutput) apply(data []byte) (string, bool, error) {
	var event struct {
		Type   string          `json:"type"`
		ItemID string          `json:"item_id"`
		Delta  json.RawMessage `json:"delta"`
		Done   bool            `json:"done"`
		Error  json.RawMessage `json:"error"`
		Output json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return "", false, err
	}
	if len(event.Error) > 0 && string(event.Error) != "null" {
		return "", false, errors.New("Open WebUI managed chat reported an error")
	}
	if len(event.Output) > 0 && string(event.Output) != "null" {
		var items []struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(event.Output, &items); err != nil {
			return "", false, err
		}
		output.order = nil
		output.items = make(map[string]string)
		for _, item := range items {
			if item.Type != "message" || item.Role != "assistant" || item.ID == "" {
				continue
			}
			var text strings.Builder
			for _, content := range item.Content {
				if content.Type == "output_text" {
					text.WriteString(content.Text)
				}
			}
			output.order = append(output.order, item.ID)
			output.items[item.ID] = text.String()
		}
	} else if event.Type == "response.output_text.delta" {
		var delta string
		if err := json.Unmarshal(event.Delta, &delta); err != nil {
			return "", false, errors.New("invalid Open WebUI text delta")
		}
		if delta == "" {
			return output.text(), event.Done, nil
		}
		if event.ItemID == "" {
			return "", false, errors.New("Open WebUI text delta has no item ID")
		}
		if _, ok := output.items[event.ItemID]; !ok {
			output.order = append(output.order, event.ItemID)
		}
		output.items[event.ItemID] += delta
	}
	return output.text(), event.Done, nil
}

func (output *managedOutput) text() string {
	var text strings.Builder
	for _, id := range output.order {
		text.WriteString(output.items[id])
	}
	return text.String()
}
