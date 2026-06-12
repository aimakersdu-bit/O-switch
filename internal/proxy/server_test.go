package proxy

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"baixin-switch/internal/config"
)

func TestHealth(t *testing.T) {
	srv := NewServer(noAuditConfig(config.Config{ListenAddr: "127.0.0.1:0"}))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected health body: %s", rec.Body.String())
	}
}

func TestHealthzReadyzAndMetrics(t *testing.T) {
	srv := NewServer(noAuditConfig(config.Config{ListenAddr: "127.0.0.1:0", UpstreamBaseURL: "http://upstream.test"}))

	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("expected text metrics content type, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "baixin_active_requests") {
		t.Fatalf("expected baixin metrics, got:\n%s", rec.Body.String())
	}
}

func TestChatCompletionsAnthropicModeConvertsNonStreamRequestAndResponse(t *testing.T) {
	var upstreamBody map[string]any
	upstream := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer upstream-key" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("unexpected Content-Type header: %q", got)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &upstreamBody); err != nil {
			t.Fatalf("invalid upstream json: %v\n%s", err, string(raw))
		}
		body := `{"id":"msg_1","type":"message","role":"assistant","model":"deepseek-v4-pro","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	srv := NewServerWithClient(config.Config{
		Mode:            "anthropic_messages",
		UpstreamBaseURL: "http://anthropic.test",
		UpstreamAPIKey:  "upstream-key",
		DefaultModel:    "deepseek-v4-pro",
		AuditEnabled:    false,
		AuditLogPath:    "disabled",
	}, &http.Client{Transport: upstream})

	reqBody := `{
		"messages":[
			{"role":"system","content":"be concise"},
			{"role":"user","content":"say hello"}
		],
		"stream":false
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamBody["model"] != "deepseek-v4-pro" {
		t.Fatalf("expected default model in upstream body, got %#v", upstreamBody["model"])
	}
	if upstreamBody["max_tokens"].(float64) != 4096 {
		t.Fatalf("expected default max_tokens 4096, got %#v", upstreamBody["max_tokens"])
	}
	if upstreamBody["system"] != "be concise" {
		t.Fatalf("expected system prompt, got %#v", upstreamBody["system"])
	}
	var downstream map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &downstream); err != nil {
		t.Fatalf("invalid downstream json: %v\n%s", err, rec.Body.String())
	}
	choices := downstream["choices"].([]any)
	message := choices[0].(map[string]any)["message"].(map[string]any)
	if message["content"] != "hello" {
		t.Fatalf("expected assistant content hello, got %#v", message["content"])
	}
	if downstream["usage"].(map[string]any)["total_tokens"].(float64) != 5 {
		t.Fatalf("expected total tokens 5, got %#v", downstream["usage"])
	}
}

func TestChatCompletionsAnthropicModeWritesNonStreamAuditEvent(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "usage.jsonl")
	upstream := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"id":"msg_1","type":"message","role":"assistant","model":"deepseek-v4-pro","content":[{"type":"text","text":"hello secret"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	srv := NewServerWithClient(config.Config{
		Mode:                "anthropic_messages",
		UpstreamBaseURL:     "http://anthropic.test",
		DefaultModel:        "deepseek-v4-pro",
		AuditEnabled:        true,
		AuditLogPath:        auditPath,
		AuditCaptureBody:    "preview",
		AuditPreviewChars:   2000,
		AuditQueueSize:      1,
		AuditOverflowPolicy: "sync",
	}, &http.Client{Transport: upstream})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"metadata":{"session_id":"sess-body"},"messages":[{"role":"user","content":"hello"}],"api_key":"sk-secret"}`))
	req.Header.Set("X-Request-ID", "req-1")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	srv.Close()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Request-ID") != "req-1" {
		t.Fatalf("expected X-Request-ID response header, got %q", rec.Header().Get("X-Request-ID"))
	}
	if rec.Header().Get("X-Session-ID") != "sess-body" {
		t.Fatalf("expected X-Session-ID response header, got %q", rec.Header().Get("X-Session-ID"))
	}

	event := readSingleAuditEvent(t, auditPath)
	if event["request_id"] != "req-1" || event["session_id"] != "sess-body" {
		t.Fatalf("unexpected ids in audit event: %#v", event)
	}
	if event["input_tokens"].(float64) != 3 || event["output_tokens"].(float64) != 2 || event["total_tokens"].(float64) != 5 {
		t.Fatalf("unexpected tokens: %#v", event)
	}
	if !strings.Contains(event["response_preview"].(string), "hello secret") {
		t.Fatalf("expected response preview, got %#v", event["response_preview"])
	}
	if strings.Contains(event["request_preview"].(string), "sk-secret") {
		t.Fatalf("request preview leaked secret: %s", event["request_preview"])
	}
}

func TestChatCompletionsAnthropicModeWritesStreamAuditEvent(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "usage.jsonl")
	upstream := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","model":"deepseek-v4-pro","usage":{"input_tokens":5}}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
			``,
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	srv := NewServerWithClient(config.Config{
		Mode:                "anthropic_messages",
		UpstreamBaseURL:     "http://anthropic.test",
		DefaultModel:        "deepseek-v4-pro",
		AuditEnabled:        true,
		AuditLogPath:        auditPath,
		AuditCaptureBody:    "preview",
		AuditPreviewChars:   2000,
		AuditQueueSize:      1,
		AuditOverflowPolicy: "sync",
	}, &http.Client{Transport: upstream})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}],"stream":true}`))
	req.Header.Set("X-Session-ID", "sess-stream")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	srv.Close()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	event := readSingleAuditEvent(t, auditPath)
	if event["session_id"] != "sess-stream" {
		t.Fatalf("unexpected session id: %#v", event)
	}
	if event["input_tokens"].(float64) != 5 || event["output_tokens"].(float64) != 2 {
		t.Fatalf("unexpected stream tokens: %#v", event)
	}
	if event["response_preview"].(string) != "hi" {
		t.Fatalf("expected stream response preview hi, got %#v", event["response_preview"])
	}
}

func TestChatCompletionsAnthropicModeWritesFailedAuditEvent(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "usage.jsonl")
	upstream := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"bad upstream"}`)),
		}, nil
	})

	srv := NewServerWithClient(config.Config{
		Mode:                "anthropic_messages",
		UpstreamBaseURL:     "http://anthropic.test",
		DefaultModel:        "deepseek-v4-pro",
		AuditEnabled:        true,
		AuditLogPath:        auditPath,
		AuditCaptureBody:    "preview",
		AuditQueueSize:      1,
		AuditOverflowPolicy: "sync",
	}, &http.Client{Transport: upstream})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)
	srv.Close()

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	event := readSingleAuditEvent(t, auditPath)
	if event["status"].(float64) != 502 || event["upstream_status"].(float64) != 502 {
		t.Fatalf("expected failed status in audit event: %#v", event)
	}
	if event["usage_source"] != "missing" {
		t.Fatalf("expected missing usage source, got %#v", event["usage_source"])
	}
}

func TestChatCompletionsAnthropicModeConvertsStreamResponse(t *testing.T) {
	upstream := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		body := strings.Join([]string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","model":"deepseek-v4-pro"}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			``,
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	srv := NewServerWithClient(config.Config{
		Mode:            "anthropic_messages",
		UpstreamBaseURL: "http://anthropic.test",
		UpstreamAPIKey:  "upstream-key",
		DefaultModel:    "deepseek-v4-pro",
		AuditEnabled:    false,
		AuditLogPath:    "disabled",
	}, &http.Client{Transport: upstream})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"content":"hi"`) {
		t.Fatalf("expected OpenAI content delta, got:\n%s", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("expected OpenAI stop finish reason, got:\n%s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected [DONE], got:\n%s", body)
	}
}

func TestChatCompletionsReturns429WhenRequestLimitExceeded(t *testing.T) {
	srv := NewServerWithClient(config.Config{
		Mode:                  "anthropic_messages",
		UpstreamBaseURL:       "http://anthropic.test",
		MaxConcurrentRequests: 1,
		AuditEnabled:          false,
		AuditLogPath:          "disabled",
	}, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"msg_1","model":"m","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`)),
		}, nil
	})})
	if !srv.requestLimiter.TryAcquire() {
		t.Fatal("failed to occupy limiter")
	}
	defer srv.requestLimiter.Release()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatCompletionsProxyNormalizesOneShotToolCallStream(t *testing.T) {
	upstream := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer upstream-key" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		body := strings.Join([]string{
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"DeepSeek-V4-Pro","choices":[{"index":0,"delta":{"content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"北京\"}"}}]},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"DeepSeek-V4-Pro","choices":[{"index":0,"delta":{"content":null},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
			``,
		}, "\n\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	srv := NewServerWithClient(config.Config{
		Mode:                  "openai_passthrough",
		UpstreamBaseURL:       "http://upstream.test",
		UpstreamAPIKey:        "upstream-key",
		ToolCallStreamShim:    true,
		ToolCallArgumentChunk: 5,
		AuditEnabled:          false,
		AuditLogPath:          "disabled",
	}, &http.Client{Transport: upstream})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", got)
	}
	body := rec.Body.String()
	if count := strings.Count(body, "data: "); count < 5 {
		t.Fatalf("expected normalized stream to contain multiple data events, got %d:\n%s", count, body)
	}
	if !strings.Contains(body, `"arguments":""`) {
		t.Fatalf("expected metadata chunk with empty arguments:\n%s", body)
	}
	if !strings.Contains(body, `"finish_reason":"tool_calls"`) {
		t.Fatalf("expected finish_reason tool_calls:\n%s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected [DONE]:\n%s", body)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func noAuditConfig(cfg config.Config) config.Config {
	cfg.AuditEnabled = false
	cfg.AuditLogPath = "disabled"
	return cfg
}

func readSingleAuditEvent(t *testing.T, path string) map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatalf("expected one audit line")
	}
	var event map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
		t.Fatalf("invalid audit json: %v", err)
	}
	if scanner.Scan() {
		t.Fatalf("expected exactly one audit line")
	}
	return event
}
