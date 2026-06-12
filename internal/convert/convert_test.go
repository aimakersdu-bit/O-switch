package convert

import (
	"encoding/json"
	"testing"
)

func TestOpenAIChatToAnthropicMapsMessagesToolsAndToolResults(t *testing.T) {
	raw := []byte(`{
		"model": "gpt-4.1",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "developer", "content": "Prefer concise answers."},
			{"role": "user", "content": "Weather?"},
			{"role": "assistant", "content": "I will check.", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"北京\"}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "{\"temp\":20}"}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get weather",
				"parameters": {
					"type": "object",
					"properties": {"city": {"type": "string"}},
					"required": ["city"]
				}
			}
		}],
		"tool_choice": "auto",
		"max_tokens": 1024,
		"temperature": 0.2,
		"top_p": 0.9,
		"stream": true
	}`)

	req, err := OpenAIChatToAnthropic(raw, Options{DefaultModel: "deepseek-v4-pro", ModelMap: map[string]string{"gpt-4.1": "deepseek-v4-pro"}})
	if err != nil {
		t.Fatalf("OpenAIChatToAnthropic returned error: %v", err)
	}

	if req.Model != "deepseek-v4-pro" {
		t.Fatalf("model mismatch: %s", req.Model)
	}
	if req.System == nil || *req.System != "You are helpful.\n\nPrefer concise answers." {
		t.Fatalf("system mismatch: %#v", req.System)
	}
	if req.MaxTokens != 1024 {
		t.Fatalf("max_tokens mismatch: %d", req.MaxTokens)
	}
	if req.Stream == nil || !*req.Stream {
		t.Fatalf("stream should be true")
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "get_weather" {
		t.Fatalf("tool mismatch: %#v", req.Tools)
	}
	if req.ToolChoice == nil || req.ToolChoice["type"] != "auto" {
		t.Fatalf("tool_choice mismatch: %#v", req.ToolChoice)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 Anthropic messages, got %#v", req.Messages)
	}
	if req.Messages[1].Role != "assistant" {
		t.Fatalf("expected assistant message, got %#v", req.Messages[1])
	}
	if got := req.Messages[1].Content[1].Type; got != "tool_use" {
		t.Fatalf("expected tool_use block, got %s", got)
	}
	if req.Messages[1].Content[1].ID != "call_1" || req.Messages[1].Content[1].Name != "get_weather" {
		t.Fatalf("tool_use mismatch: %#v", req.Messages[1].Content[1])
	}
	if req.Messages[2].Content[0].Type != "tool_result" || req.Messages[2].Content[0].ToolUseID != "call_1" {
		t.Fatalf("tool_result mismatch: %#v", req.Messages[2].Content[0])
	}

	if _, err := json.Marshal(req); err != nil {
		t.Fatalf("anthropic request should marshal: %v", err)
	}
}

func TestAnthropicToOpenAIChatMapsTextAndToolUse(t *testing.T) {
	raw := []byte(`{
		"id": "msg_1",
		"type": "message",
		"role": "assistant",
		"model": "deepseek-v4-pro",
		"content": [
			{"type": "text", "text": "I will call a tool."},
			{"type": "tool_use", "id": "call_1", "name": "get_weather", "input": {"city": "北京"}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 12, "output_tokens": 8}
	}`)

	resp, err := AnthropicToOpenAIChat(raw, "chatcmpl-test")
	if err != nil {
		t.Fatalf("AnthropicToOpenAIChat returned error: %v", err)
	}

	if resp.ID != "chatcmpl-test" {
		t.Fatalf("id mismatch: %s", resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected one choice: %#v", resp.Choices)
	}
	choice := resp.Choices[0]
	if choice.FinishReason == nil || *choice.FinishReason != "tool_calls" {
		t.Fatalf("finish reason mismatch: %#v", choice.FinishReason)
	}
	if choice.Message.Content == nil || *choice.Message.Content != "I will call a tool." {
		t.Fatalf("content mismatch: %#v", choice.Message.Content)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("expected one tool call: %#v", choice.Message.ToolCalls)
	}
	if choice.Message.ToolCalls[0].ID != "call_1" || choice.Message.ToolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("tool call mismatch: %#v", choice.Message.ToolCalls[0])
	}
	if choice.Message.ToolCalls[0].Function.Arguments != `{"city":"北京"}` {
		t.Fatalf("tool arguments mismatch: %s", choice.Message.ToolCalls[0].Function.Arguments)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 8 || resp.Usage.TotalTokens != 20 {
		t.Fatalf("usage mismatch: %#v", resp.Usage)
	}
}
