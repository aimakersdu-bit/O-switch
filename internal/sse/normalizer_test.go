package sse

import (
	"strings"
	"testing"
)

func TestNormalizeChatCompletionStreamSplitsOneShotToolArguments(t *testing.T) {
	input := strings.Join([]string{
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"DeepSeek-V4-Pro","choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"北京\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"DeepSeek-V4-Pro","choices":[{"index":0,"delta":{"content":null},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")

	output, err := NormalizeChatCompletionStream(strings.NewReader(input), Options{ToolCallStreamShim: true, ArgumentChunkSize: 5})
	if err != nil {
		t.Fatalf("NormalizeChatCompletionStream returned error: %v", err)
	}

	events := dataLines(output)
	if len(events) < 5 {
		t.Fatalf("expected at least metadata, multiple argument chunks, finish, done; got %d events:\n%s", len(events), output)
	}

	if !strings.Contains(events[0], `"id":"call_1"`) {
		t.Fatalf("first event should preserve tool call id: %s", events[0])
	}
	if !strings.Contains(events[0], `"name":"get_weather"`) {
		t.Fatalf("first event should preserve function name: %s", events[0])
	}
	if !strings.Contains(events[0], `"arguments":""`) {
		t.Fatalf("first event should start arguments as empty string: %s", events[0])
	}

	joinedArgs := ""
	argChunkCount := 0
	for _, event := range events[1:] {
		if event == "[DONE]" || strings.Contains(event, `"finish_reason":"tool_calls"`) {
			continue
		}
		arg := extractJSONField(t, event, "arguments")
		if arg == "" {
			t.Fatalf("argument delta should not be empty after metadata event: %s", event)
		}
		joinedArgs += arg
		argChunkCount++
	}
	if argChunkCount < 2 {
		t.Fatalf("expected arguments to be split into multiple chunks, got %d", argChunkCount)
	}
	if joinedArgs != `{"city":"北京"}` {
		t.Fatalf("joined arguments mismatch: got %q", joinedArgs)
	}
	if events[len(events)-1] != "[DONE]" {
		t.Fatalf("last event should be [DONE], got %s", events[len(events)-1])
	}
}

func TestNormalizeChatCompletionStreamPassesIncrementalToolArgumentsThrough(t *testing.T) {
	input := strings.Join([]string{
		`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":2,"model":"glm-5.1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":2,"model":"glm-5.1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\""}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","created":2,"model":"glm-5.1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"北京\"}"}}]},"finish_reason":null}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")

	output, err := NormalizeChatCompletionStream(strings.NewReader(input), Options{ToolCallStreamShim: true, ArgumentChunkSize: 5})
	if err != nil {
		t.Fatalf("NormalizeChatCompletionStream returned error: %v", err)
	}

	events := dataLines(output)
	if len(events) != 4 {
		t.Fatalf("expected passthrough to preserve event count, got %d events:\n%s", len(events), output)
	}
	if events[0] == events[1] {
		t.Fatalf("events should be preserved as separate chunks")
	}
	if !strings.Contains(events[1], `{\"city\"`) {
		t.Fatalf("second event should preserve first argument delta: %s", events[1])
	}
	if events[3] != "[DONE]" {
		t.Fatalf("last event should be [DONE], got %s", events[3])
	}
}

func dataLines(s string) []string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			out = append(out, strings.TrimPrefix(line, "data: "))
		}
	}
	return out
}

func extractJSONField(t *testing.T, s, key string) string {
	t.Helper()
	needle := `"` + key + `":"`
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	start := idx + len(needle)
	var b strings.Builder
	escaped := false
	for _, r := range s[start:] {
		if escaped {
			switch r {
			case '"', '\\', '/':
				b.WriteRune(r)
			case 'n':
				b.WriteRune('\n')
			case 'r':
				b.WriteRune('\r')
			case 't':
				b.WriteRune('\t')
			default:
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}
