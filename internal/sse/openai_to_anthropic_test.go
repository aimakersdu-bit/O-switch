package sse

import (
	"strings"
	"testing"
)

func TestOpenAIToAnthropicStreamText(t *testing.T) {
	input := strings.Join([]string{
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n")

	var out strings.Builder
	err := WriteOpenAIToAnthropicStream(&out, strings.NewReader(input), OpenAIStreamOptions{
		MessageID: "msg-123",
		Model:     "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("WriteOpenAIToAnthropicStream failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, `event: message_start`) || !strings.Contains(output, `"id":"msg-123"`) {
		t.Fatalf("expected message_start event, got:\n%s", output)
	}
	if !strings.Contains(output, `event: content_block_start`) || !strings.Contains(output, `"type":"text"`) {
		t.Fatalf("expected content_block_start for text, got:\n%s", output)
	}
	if !strings.Contains(output, `event: content_block_delta`) || !strings.Contains(output, `"text":"hello"`) {
		t.Fatalf("expected content_block_delta with text delta, got:\n%s", output)
	}
	if !strings.Contains(output, `event: content_block_stop`) {
		t.Fatalf("expected content_block_stop, got:\n%s", output)
	}
	if !strings.Contains(output, `event: message_delta`) || !strings.Contains(output, `"stop_reason":"end_turn"`) {
		t.Fatalf("expected message_delta with end_turn, got:\n%s", output)
	}
	if !strings.Contains(output, `event: message_stop`) {
		t.Fatalf("expected message_stop, got:\n%s", output)
	}
}

func TestOpenAIToAnthropicStreamToolUse(t *testing.T) {
	input := strings.Join([]string{
		`data: {"id":"chatcmpl-2","choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"北京\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n")

	var out strings.Builder
	err := WriteOpenAIToAnthropicStream(&out, strings.NewReader(input), OpenAIStreamOptions{
		MessageID: "msg-456",
		Model:     "claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("WriteOpenAIToAnthropicStream failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, `event: message_start`) {
		t.Fatalf("expected message_start, got:\n%s", output)
	}
	if !strings.Contains(output, `event: content_block_start`) || !strings.Contains(output, `"type":"tool_use"`) || !strings.Contains(output, `"name":"get_weather"`) {
		t.Fatalf("expected content_block_start for tool_use, got:\n%s", output)
	}
	if !strings.Contains(output, `event: content_block_delta`) || !strings.Contains(output, `"partial_json":"{\"city\""`) {
		t.Fatalf("expected content_block_delta with partial json arguments, got:\n%s", output)
	}
	if !strings.Contains(output, `event: content_block_stop`) {
		t.Fatalf("expected content_block_stop, got:\n%s", output)
	}
	if !strings.Contains(output, `event: message_delta`) || !strings.Contains(output, `"stop_reason":"tool_use"`) {
		t.Fatalf("expected message_delta with tool_use, got:\n%s", output)
	}
	if !strings.Contains(output, `event: message_stop`) {
		t.Fatalf("expected message_stop, got:\n%s", output)
	}
}
