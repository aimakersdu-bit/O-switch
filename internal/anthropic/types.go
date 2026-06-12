package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type MessagesRequest struct {
	Model         string         `json:"model"`
	MaxTokens     int            `json:"max_tokens"`
	Messages      []Message      `json:"messages"`
	System        *string        `json:"system,omitempty"`
	Tools         []Tool         `json:"tools,omitempty"`
	ToolChoice    map[string]any `json:"tool_choice,omitempty"`
	Temperature   *float64       `json:"temperature,omitempty"`
	TopP          *float64       `json:"top_p,omitempty"`
	StopSequences []string       `json:"stop_sequences,omitempty"`
	Stream        *bool          `json:"stream,omitempty"`
}

type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// UnmarshalJSON handles both standard ContentBlock arrays and plain strings for Message Content.
func (m *Message) UnmarshalJSON(data []byte) error {
	var aux struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.Role = aux.Role
	contentBytes := bytes.TrimSpace(aux.Content)
	if len(contentBytes) == 0 {
		m.Content = nil
		return nil
	}
	if contentBytes[0] == '"' {
		var s string
		if err := json.Unmarshal(contentBytes, &s); err != nil {
			return err
		}
		m.Content = []ContentBlock{{
			Type: "text",
			Text: s,
		}}
		return nil
	}
	if contentBytes[0] == '[' {
		var blocks []ContentBlock
		if err := json.Unmarshal(contentBytes, &blocks); err != nil {
			return err
		}
		m.Content = blocks
		return nil
	}
	return fmt.Errorf("invalid content type in Message: must be string or array")
}

type ContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"`
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

type MessagesResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        Usage          `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
