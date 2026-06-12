package convert

import (
	"encoding/json"
	"fmt"
	"strings"

	"baixin-switch/internal/anthropic"
	"baixin-switch/internal/ir"
)

type Options struct {
	DefaultModel string
	ModelMap     map[string]string
}

type openAIChatRequest struct {
	Model               string          `json:"model"`
	Messages            []openAIMessage `json:"messages"`
	Tools               []openAITool    `json:"tools"`
	ToolChoice          any             `json:"tool_choice"`
	MaxTokens           int             `json:"max_tokens"`
	MaxCompletionTokens int             `json:"max_completion_tokens"`
	Temperature         *float64        `json:"temperature"`
	TopP                *float64        `json:"top_p"`
	Stop                any             `json:"stop"`
	Stream              bool            `json:"stream"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls"`
	ToolCallID string           `json:"tool_call_id"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
	Arguments   string `json:"arguments"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

func OpenAIChatToAnthropic(raw []byte, opts Options) (anthropic.MessagesRequest, error) {
	var input openAIChatRequest
	if err := json.Unmarshal(raw, &input); err != nil {
		return anthropic.MessagesRequest{}, err
	}
	req, err := openAIChatToIR(input, opts)
	if err != nil {
		return anthropic.MessagesRequest{}, err
	}
	return irToAnthropic(req), nil
}

func openAIChatToIR(input openAIChatRequest, opts Options) (ir.Request, error) {
	model := input.Model
	if mapped := opts.ModelMap[model]; mapped != "" {
		model = mapped
	}
	if model == "" {
		model = opts.DefaultModel
	}

	maxTokens := input.MaxTokens
	if maxTokens == 0 {
		maxTokens = input.MaxCompletionTokens
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}

	out := ir.Request{
		SourceAPI:   "openai_chat",
		Model:       model,
		MaxTokens:   maxTokens,
		Temperature: input.Temperature,
		TopP:        input.TopP,
		Stream:      input.Stream,
	}
	out.StopSequences = parseStop(input.Stop)
	out.ToolChoice = parseToolChoice(input.ToolChoice)

	for _, tool := range input.Tools {
		if tool.Type != "function" || tool.Function.Name == "" {
			continue
		}
		out.Tools = append(out.Tools, ir.Tool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.ParametersOrEmpty(),
		})
	}

	for _, msg := range input.Messages {
		switch msg.Role {
		case "system", "developer":
			text := contentText(msg.Content)
			if text != "" {
				out.System = append(out.System, text)
			}
		case "user":
			text := contentText(msg.Content)
			if text != "" {
				out.Messages = append(out.Messages, ir.Message{Role: "user", Content: []ir.ContentBlock{{Type: "text", Text: text}}})
			}
		case "assistant":
			blocks := []ir.ContentBlock{}
			if text := contentText(msg.Content); text != "" {
				blocks = append(blocks, ir.ContentBlock{Type: "text", Text: text})
			}
			for _, call := range msg.ToolCalls {
				if call.Function.Name == "" {
					continue
				}
				var args any = map[string]any{}
				if strings.TrimSpace(call.Function.Arguments) != "" {
					if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
						args = map[string]any{"arguments": call.Function.Arguments}
					}
				}
				blocks = append(blocks, ir.ContentBlock{Type: "tool_use", ToolUse: &ir.ToolUse{ID: call.ID, Name: call.Function.Name, Input: args}})
			}
			if len(blocks) > 0 {
				out.Messages = append(out.Messages, ir.Message{Role: "assistant", Content: blocks})
			}
		case "tool":
			out.Messages = append(out.Messages, ir.Message{Role: "user", Content: []ir.ContentBlock{{
				Type:       "tool_result",
				ToolResult: &ir.ToolResult{ToolUseID: msg.ToolCallID, Content: contentText(msg.Content)},
			}}})
		default:
			return ir.Request{}, fmt.Errorf("unsupported role %q", msg.Role)
		}
	}
	return out, nil
}

func irToAnthropic(req ir.Request) anthropic.MessagesRequest {
	out := anthropic.MessagesRequest{
		Model:         req.Model,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: req.StopSequences,
	}
	if len(req.System) > 0 {
		system := strings.Join(req.System, "\n\n")
		out.System = &system
	}
	stream := req.Stream
	out.Stream = &stream
	for _, tool := range req.Tools {
		schema := tool.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out.Tools = append(out.Tools, anthropic.Tool{Name: tool.Name, Description: tool.Description, InputSchema: schema})
	}
	if req.ToolChoice.Type != "" {
		switch req.ToolChoice.Type {
		case "auto", "any", "none":
			out.ToolChoice = map[string]any{"type": req.ToolChoice.Type}
		case "tool":
			out.ToolChoice = map[string]any{"type": "tool", "name": req.ToolChoice.Name}
		}
	}
	for _, msg := range req.Messages {
		am := anthropic.Message{Role: msg.Role}
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				am.Content = append(am.Content, anthropic.ContentBlock{Type: "text", Text: block.Text})
			case "tool_use":
				am.Content = append(am.Content, anthropic.ContentBlock{Type: "tool_use", ID: block.ToolUse.ID, Name: block.ToolUse.Name, Input: block.ToolUse.Input})
			case "tool_result":
				am.Content = append(am.Content, anthropic.ContentBlock{Type: "tool_result", ToolUseID: block.ToolResult.ToolUseID, Content: block.ToolResult.Content})
			}
		}
		if len(am.Content) > 0 {
			out.Messages = append(out.Messages, am)
		}
	}
	return out
}

func contentText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if obj["type"] == "text" || obj["type"] == "input_text" {
				if text, ok := obj["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	case nil:
		return ""
	default:
		raw, _ := json.Marshal(v)
		return string(raw)
	}
}

func parseStop(stop any) []string {
	switch v := stop.(type) {
	case string:
		return []string{v}
	case []any:
		out := []string{}
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func parseToolChoice(raw any) ir.ToolChoice {
	switch v := raw.(type) {
	case string:
		switch v {
		case "auto":
			return ir.ToolChoice{Type: "auto"}
		case "required":
			return ir.ToolChoice{Type: "any"}
		case "none":
			return ir.ToolChoice{Type: "none"}
		}
	case map[string]any:
		if v["type"] == "function" {
			if fn, ok := v["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok {
					return ir.ToolChoice{Type: "tool", Name: name}
				}
			}
		}
	}
	return ir.ToolChoice{}
}

func (f openAIToolFunction) ParametersOrEmpty() any {
	if f.Parameters != nil {
		return f.Parameters
	}
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
