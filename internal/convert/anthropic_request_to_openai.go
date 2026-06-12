package convert

import (
	"encoding/json"
	"strings"

	"baixin-switch/internal/anthropic"
)

type OpenAIChatRequest struct {
	Model               string                   `json:"model"`
	Messages            []OpenAIChatMessage      `json:"messages"`
	Tools               []OpenAIChatTool         `json:"tools,omitempty"`
	ToolChoice          any                      `json:"tool_choice,omitempty"`
	MaxTokens           int                      `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                      `json:"max_completion_tokens,omitempty"`
	Temperature         *float64                 `json:"temperature,omitempty"`
	TopP                *float64                 `json:"top_p,omitempty"`
	Stop                any                      `json:"stop,omitempty"`
	Stream              bool                     `json:"stream,omitempty"`
	StreamOptions       *OpenAIChatStreamOptions `json:"stream_options,omitempty"`
}

type OpenAIChatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type OpenAIChatTool struct {
	Type     string                   `json:"type"`
	Function OpenAIChatToolDefinition `json:"function"`
}

type OpenAIChatToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

func AnthropicToOpenAIChatRequest(anthropicReq anthropic.MessagesRequest) (OpenAIChatRequest, error) {
	out := OpenAIChatRequest{
		Model:       anthropicReq.Model,
		Temperature: anthropicReq.Temperature,
		TopP:        anthropicReq.TopP,
	}

	if anthropicReq.MaxTokens > 0 {
		out.MaxTokens = anthropicReq.MaxTokens
	}
	if anthropicReq.Stream != nil {
		out.Stream = *anthropicReq.Stream
		if out.Stream {
			out.StreamOptions = &OpenAIChatStreamOptions{
				IncludeUsage: true,
			}
		}
	}
	if len(anthropicReq.StopSequences) > 0 {
		out.Stop = anthropicReq.StopSequences
	}

	// 1. Convert System instructions
	if anthropicReq.System != nil && *anthropicReq.System != "" {
		out.Messages = append(out.Messages, OpenAIChatMessage{
			Role:    "system",
			Content: anthropicReq.System,
		})
	}

	// 2. Convert Messages
	for _, msg := range anthropicReq.Messages {
		if msg.Role == "user" {
			// In Anthropic, a user message can contain tool_result blocks.
			// In OpenAI, tool results must be separate messages with role "tool".
			var userTextBlocks []string
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					userTextBlocks = append(userTextBlocks, block.Text)
				case "tool_result":
					// If we had accumulated user text, flush it as a user message first
					if len(userTextBlocks) > 0 {
						joinedText := strings.Join(userTextBlocks, "\n")
						out.Messages = append(out.Messages, OpenAIChatMessage{
							Role:    "user",
							Content: &joinedText,
						})
						userTextBlocks = nil
					}
					// Add the tool result message
					toolContent := contentToString(block.Content)
					out.Messages = append(out.Messages, OpenAIChatMessage{
						Role:       "tool",
						Content:    &toolContent,
						ToolCallID: block.ToolUseID,
					})
				}
			}
			// Flush remaining user text if any
			if len(userTextBlocks) > 0 {
				joinedText := strings.Join(userTextBlocks, "\n")
				out.Messages = append(out.Messages, OpenAIChatMessage{
					Role:    "user",
					Content: &joinedText,
				})
			}
		} else if msg.Role == "assistant" {
			var assistantTextBlocks []string
			var toolCalls []OpenAIChatToolCall
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					assistantTextBlocks = append(assistantTextBlocks, block.Text)
				case "tool_use":
					argsBytes, err := json.Marshal(block.Input)
					if err != nil {
						argsBytes = []byte("{}")
					}
					toolCalls = append(toolCalls, OpenAIChatToolCall{
						ID:   block.ID,
						Type: "function",
						Function: OpenAIChatToolFunction{
							Name:      block.Name,
							Arguments: string(argsBytes),
						},
					})
				}
			}
			var content *string
			if len(assistantTextBlocks) > 0 {
				joined := strings.Join(assistantTextBlocks, "\n")
				content = &joined
			}
			out.Messages = append(out.Messages, OpenAIChatMessage{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
			})
		}
	}

	// 3. Convert Tools
	for _, tool := range anthropicReq.Tools {
		out.Tools = append(out.Tools, OpenAIChatTool{
			Type: "function",
			Function: OpenAIChatToolDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}

	// 4. Convert ToolChoice
	if anthropicReq.ToolChoice != nil {
		switch t := anthropicReq.ToolChoice["type"].(type) {
		case string:
			if t == "auto" {
				out.ToolChoice = "auto"
			} else if t == "any" {
				out.ToolChoice = "required"
			} else if t == "tool" {
				if name, ok := anthropicReq.ToolChoice["name"].(string); ok {
					out.ToolChoice = map[string]any{
						"type": "function",
						"function": map[string]string{
							"name": name,
						},
					}
				}
			}
		}
	}

	return out, nil
}

func OpenAIChatToAnthropicResponse(openaiResp OpenAIChatResponse) (anthropic.MessagesResponse, error) {
	out := anthropic.MessagesResponse{
		ID:    openaiResp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: openaiResp.Model,
	}

	if len(openaiResp.Choices) > 0 {
		choice := openaiResp.Choices[0]
		if choice.Message.Content != nil && *choice.Message.Content != "" {
			out.Content = append(out.Content, anthropic.ContentBlock{
				Type: "text",
				Text: *choice.Message.Content,
			})
		}
		for _, tc := range choice.Message.ToolCalls {
			var input map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				input = map[string]any{"arguments": tc.Function.Arguments}
			}
			out.Content = append(out.Content, anthropic.ContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
		if choice.FinishReason != nil {
			out.StopReason = mapOpenAIFinishReason(*choice.FinishReason)
		}
	}

	out.Usage = anthropic.Usage{
		InputTokens:  openaiResp.Usage.PromptTokens,
		OutputTokens: openaiResp.Usage.CompletionTokens,
	}
	if out.Usage.InputTokens == 0 && openaiResp.Usage.InputTokens > 0 {
		out.Usage.InputTokens = openaiResp.Usage.InputTokens
	}
	if out.Usage.OutputTokens == 0 && openaiResp.Usage.OutputTokens > 0 {
		out.Usage.OutputTokens = openaiResp.Usage.OutputTokens
	}

	return out, nil
}

func mapOpenAIFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}

func contentToString(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}
