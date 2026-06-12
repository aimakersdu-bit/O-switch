package convert

import (
	"encoding/json"
	"time"

	"baixin-switch/internal/anthropic"
)

type OpenAIChatResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []OpenAIChatChoice `json:"choices"`
	Usage   OpenAIUsage        `json:"usage"`
}

type OpenAIChatChoice struct {
	Index        int               `json:"index"`
	Message      OpenAIChatMessage `json:"message"`
	FinishReason *string           `json:"finish_reason"`
}

type OpenAIChatMessage struct {
	Role      string               `json:"role"`
	Content   *string              `json:"content"`
	ToolCalls []OpenAIChatToolCall `json:"tool_calls,omitempty"`
}

type OpenAIChatToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function OpenAIChatToolFunction `json:"function"`
}

type OpenAIChatToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIUsage struct {
	InputTokens      int `json:"input_tokens,omitempty"`
	OutputTokens     int `json:"output_tokens,omitempty"`
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens"`
}

func AnthropicToOpenAIChat(raw []byte, responseID string) (OpenAIChatResponse, error) {
	var input anthropic.MessagesResponse
	if err := json.Unmarshal(raw, &input); err != nil {
		return OpenAIChatResponse{}, err
	}
	if responseID == "" {
		responseID = input.ID
	}

	var text string
	var toolCalls []OpenAIChatToolCall
	for _, block := range input.Content {
		switch block.Type {
		case "text":
			text += block.Text
		case "tool_use":
			args, err := json.Marshal(block.Input)
			if err != nil {
				args = []byte(`{}`)
			}
			toolCalls = append(toolCalls, OpenAIChatToolCall{
				ID:   block.ID,
				Type: "function",
				Function: OpenAIChatToolFunction{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}

	var content *string
	if text != "" {
		content = &text
	}
	finish := mapStopReason(input.StopReason)

	return OpenAIChatResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   input.Model,
		Choices: []OpenAIChatChoice{{
			Index: 0,
			Message: OpenAIChatMessage{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
			},
			FinishReason: &finish,
		}},
		Usage: OpenAIUsage{
			InputTokens:      input.Usage.InputTokens,
			OutputTokens:     input.Usage.OutputTokens,
			PromptTokens:     input.Usage.InputTokens,
			CompletionTokens: input.Usage.OutputTokens,
			TotalTokens:      input.Usage.InputTokens + input.Usage.OutputTokens,
		},
	}, nil
}

func mapStopReason(reason string) string {
	switch reason {
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return "stop"
	}
}
