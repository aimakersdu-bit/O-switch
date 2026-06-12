package sse

import (
	"strings"
	"testing"
)

func TestAnthropicToOpenAIChatStreamText(t *testing.T) {
	input := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","model":"deepseek-v4-pro","usage":{"input_tokens":5,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	output, err := AnthropicToOpenAIChatStream(strings.NewReader(input), ChatStreamOptions{ResponseID: "chatcmpl-1", Model: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("AnthropicToOpenAIChatStream returned error: %v", err)
	}
	if !strings.Contains(output, `"role":"assistant"`) {
		t.Fatalf("expected initial assistant role chunk:\n%s", output)
	}
	if !strings.Contains(output, `"content":"hello"`) {
		t.Fatalf("expected text delta:\n%s", output)
	}
	if !strings.Contains(output, `"finish_reason":"stop"`) {
		t.Fatalf("expected stop finish reason:\n%s", output)
	}
	if !strings.Contains(output, "data: [DONE]") {
		t.Fatalf("expected done:\n%s", output)
	}
}

func TestWriteAnthropicToOpenAIChatStreamText(t *testing.T) {
	input := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","model":"deepseek-v4-pro"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		``,
	}, "\n")

	var out strings.Builder
	err := WriteAnthropicToOpenAIChatStream(&out, strings.NewReader(input), ChatStreamOptions{ResponseID: "chatcmpl-1", Model: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("WriteAnthropicToOpenAIChatStream returned error: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, `"role":"assistant"`) {
		t.Fatalf("expected initial assistant role chunk:\n%s", output)
	}
	if !strings.Contains(output, `"content":"hello"`) {
		t.Fatalf("expected text delta:\n%s", output)
	}
	if !strings.Contains(output, "data: [DONE]") {
		t.Fatalf("expected done:\n%s", output)
	}
}

func TestAnthropicToOpenAIChatStreamToolUse(t *testing.T) {
	input := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_2","model":"deepseek-v4-pro","usage":{"input_tokens":5,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":":\"北京\"}"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":6}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	output, err := AnthropicToOpenAIChatStream(strings.NewReader(input), ChatStreamOptions{ResponseID: "chatcmpl-2", Model: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("AnthropicToOpenAIChatStream returned error: %v", err)
	}
	if !strings.Contains(output, `"id":"toolu_1"`) || !strings.Contains(output, `"name":"get_weather"`) {
		t.Fatalf("expected tool metadata chunk:\n%s", output)
	}
	if !strings.Contains(output, `{\"city\"`) || !strings.Contains(output, `:\"北京\"}`) {
		t.Fatalf("expected argument deltas:\n%s", output)
	}
	if !strings.Contains(output, `"finish_reason":"tool_calls"`) {
		t.Fatalf("expected tool_calls finish reason:\n%s", output)
	}
	if !strings.Contains(output, "data: [DONE]") {
		t.Fatalf("expected done:\n%s", output)
	}
}
