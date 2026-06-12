# baixin-switch OpenAI-to-Anthropic Production Gateway Design

Date: 2026-06-12

## 1. Decision

Use **方案 B：内部统一 IR 的生产网关**.

`baixin-switch` will evolve from a DS Pro tool-call stream compatibility shim into a production-grade protocol gateway:

```text
OpenAI-compatible client
  -> baixin-switch
  -> Anthropic Messages-compatible upstream
```

Primary goals:

- Convert OpenAI-compatible APIs to Anthropic Messages API.
- Keep the current DS Pro one-shot tool-call streaming compatibility behavior.
- Add logs, metrics, tracing, health probes, and pprof for production observability.
- Support **1000 concurrent requests on a single instance** when hardware and upstream capacity allow it.
- Keep the architecture simple enough for private deployment and later horizontal scaling.

## 2. Scope

### Phase 1 Target

- `POST /v1/chat/completions` -> Anthropic Messages API.
- Streaming and non-streaming responses.
- Tool call and tool result conversion.
- Anthropic streaming events -> OpenAI Chat Completions streaming chunks.
- Production observability and concurrency controls.
- Preserve the existing OpenAI-compatible passthrough mode as an optional compatibility route.

### Phase 2 Target

- `POST /v1/responses` -> Anthropic Messages API.
- Anthropic streaming events -> OpenAI Responses API streaming events.
- More complete OpenAI parameter support.

### Out of Scope for This Design

- Multi-upstream failover.
- Persistent request log database.
- Admin UI.
- Billing system.
- Full prompt/body logging by default.

## 3. Current Baseline

The current code supports:

- `GET /health`
- `POST /v1/chat/completions`
- OpenAI-compatible upstream passthrough.
- DS Pro one-shot `tool_calls[].function.arguments` streaming normalization.

Current bottlenecks for production:

- SSE normalization currently buffers the full upstream stream before writing downstream.
- There is no metrics endpoint.
- Logs are minimal.
- No request IDs or trace IDs.
- No concurrency limiter.
- No OpenAI -> Anthropic request conversion yet.

## 4. Architecture

```text
Client / Comate
  -> net/http Server
  -> Middleware
       request_id
       panic recovery
       access log
       metrics
       body limit
       concurrency limit
  -> Route Handler
       /healthz
       /readyz
       /metrics
       /v1/chat/completions
       /v1/responses
  -> OpenAI Adapter
       parse Chat Completions
       parse Responses
  -> Internal IR
       Request
       Message
       ContentBlock
       Tool
       StreamEvent
  -> Anthropic Adapter
       build Messages request
       parse Messages response
       parse Anthropic SSE
  -> DeepSeek/Anthropic Rectifier
       unsupported fields
       unsupported blocks
       thinking/signature cleanup
  -> Upstream Client
       pooled transport
       timeout policy
       cancellation propagation
  -> Stream Translator
       Anthropic SSE -> OpenAI Chat chunks
       Anthropic SSE -> OpenAI Responses events
  -> Client
```

## 5. Package Layout

```text
internal/server/
  server.go              # http.Server construction and route registration
  middleware.go          # request id, recover, access log, metrics, limits

internal/limits/
  limiter.go             # global active request/stream semaphores

internal/observability/
  logger.go              # slog setup and redaction helpers
  metrics.go             # Prometheus collectors
  tracing.go             # OpenTelemetry setup

internal/openai/
  chat_types.go          # OpenAI Chat request/response/chunk structs
  responses_types.go     # OpenAI Responses request/event structs
  chat_writer.go         # OpenAI Chat non-stream/SSE writers
  responses_writer.go    # OpenAI Responses non-stream/SSE writers

internal/anthropic/
  messages_types.go      # Anthropic Messages request/response/event structs
  sse_reader.go          # streaming Anthropic event decoder

internal/ir/
  request.go             # provider-independent request model
  message.go             # message/content/tool models
  stream_event.go        # provider-independent stream events

internal/convert/
  openai_chat_to_ir.go
  openai_responses_to_ir.go
  ir_to_anthropic.go
  anthropic_to_ir.go
  ir_to_openai_chat.go
  ir_to_openai_responses.go

internal/rectifier/
  deepseek_anthropic.go
  media.go
  thinking.go
  tools.go

internal/upstream/
  client.go              # shared http.Client and transport
  anthopic.go            # Messages request execution

internal/sse/
  codec.go               # generic SSE read/write helpers
  normalizer.go          # current DS Pro OpenAI-compatible shim
```

The existing `internal/sse/normalizer.go` remains useful for OpenAI-compatible passthrough mode. The OpenAI-to-Anthropic path will use a new streaming translator instead of buffering.

## 6. Internal IR

The IR keeps protocol adapters separate from business behavior.

```go
type Request struct {
    SourceAPI      string // openai_chat | openai_responses
    Model          string
    System         []TextBlock
    Messages       []Message
    Tools          []Tool
    ToolChoice     ToolChoice
    MaxTokens      int
    Temperature    *float64
    TopP           *float64
    StopSequences  []string
    Stream         bool
    Metadata       map[string]string
}

type Message struct {
    Role    string // user | assistant
    Content []ContentBlock
}

type ContentBlock struct {
    Type       string // text | tool_use | tool_result | thinking | unsupported
    Text       string
    ToolUse    *ToolUse
    ToolResult *ToolResult
}

type StreamEvent struct {
    Type          string // text_delta | tool_start | tool_delta | tool_done | message_done | error
    Index         int
    Text          string
    ToolCallID    string
    ToolName      string
    ToolArgsDelta string
    StopReason    string
    Usage         *Usage
}
```

Benefits:

- OpenAI Chat and OpenAI Responses share one Anthropic conversion path.
- Anthropic streaming only needs to produce IR stream events once.
- Observability can log conversion-stage metadata without depending on provider structs.

## 7. OpenAI Chat -> Anthropic Messages

### Request Mapping

| OpenAI Chat | Internal IR | Anthropic Messages |
| --- | --- | --- |
| `model` | `Request.Model` | `model` after optional mapping |
| `messages[].role=system` | `System` | top-level `system` |
| `messages[].role=developer` | `System` | top-level `system` |
| `messages[].role=user` | user message | `messages[].role=user` |
| `messages[].role=assistant` text | assistant text | assistant `content[].text` |
| `assistant.tool_calls[]` | assistant `tool_use` | assistant `content[].tool_use` |
| `messages[].role=tool` | user `tool_result` | user `content[].tool_result` |
| `tools[].function` | `Tool` | `tools[].input_schema` |
| `tool_choice=auto` | auto | Anthropic `tool_choice.auto` or omitted |
| `tool_choice=required` | any | Anthropic `tool_choice.any` |
| forced function | forced tool | Anthropic `tool_choice.tool` |
| `max_tokens` | max tokens | `max_tokens` |
| `max_completion_tokens` | max tokens | `max_tokens` |
| `temperature` | temperature | `temperature` |
| `top_p` | top_p | `top_p` |
| `stop` | stop sequences | `stop_sequences` |
| `stream` | stream | `stream` |

### Non-Streaming Response Mapping

| Anthropic Messages | OpenAI Chat |
| --- | --- |
| text content blocks | `choices[0].message.content` |
| `tool_use` blocks | `choices[0].message.tool_calls[]` |
| `stop_reason=tool_use` | `finish_reason=tool_calls` |
| `stop_reason=max_tokens` | `finish_reason=length` |
| `stop_reason=end_turn` | `finish_reason=stop` |
| usage | usage |

### Streaming Response Mapping

Anthropic streaming flow:

```text
message_start
content_block_start
content_block_delta
content_block_stop
message_delta
message_stop
```

OpenAI Chat streaming output:

```text
data: {"choices":[{"delta":{"role":"assistant"}}]}
data: {"choices":[{"delta":{"content":"..."}}]}
data: {"choices":[{"delta":{"tool_calls":[...]}}]}
data: {"choices":[{"finish_reason":"tool_calls"}]}
data: [DONE]
```

Rules:

- Anthropic `text_delta.text` -> OpenAI `delta.content`.
- Anthropic `content_block_start` with `tool_use` -> OpenAI first `delta.tool_calls[]` with id/type/name and empty arguments.
- Anthropic `input_json_delta.partial_json` -> OpenAI `delta.tool_calls[].function.arguments`.
- Anthropic `message_delta.stop_reason=tool_use` -> OpenAI `finish_reason=tool_calls`.
- Anthropic `message_stop` -> `data: [DONE]`.
- Unknown Anthropic events are ignored but counted in metrics.

## 8. OpenAI Responses -> Anthropic Messages

Responses support will reuse the same IR.

| OpenAI Responses | Internal IR | Anthropic Messages |
| --- | --- | --- |
| `instructions` | system | top-level `system` |
| `input` string | user text | user message |
| `input[].type=message` | message | user/assistant message |
| `function_call` | assistant tool use | `tool_use` |
| `function_call_output` | user tool result | `tool_result` |
| `tools[]` | tools | Anthropic tools |
| `max_output_tokens` | max tokens | `max_tokens` |
| `reasoning` | thinking config | `thinking` if enabled |
| `stream` | stream | `stream` |

Responses streaming is more complex and should be implemented after Chat Completions is production-stable.

## 9. DeepSeek Anthropic Rectifier

The rectifier runs after `IR -> Anthropic` conversion and before the upstream request.

Rules:

- Move any accidental `system` role messages into top-level `system`.
- Drop or replace unsupported content blocks:
  - image
  - document
  - MCP tool use/result
  - search_result
  - redacted_thinking
  - container_upload
  - code_execution_tool_result
- Drop fields DeepSeek documents as ignored unless `STRICT_PASSTHROUGH=true`:
  - `container`
  - `mcp_servers`
  - most `metadata`
  - `service_tier`
  - `top_k`
- Preserve supported fields:
  - `model`
  - `messages`
  - `system`
  - `max_tokens`
  - `stop_sequences`
  - `stream`
  - `temperature`
  - `thinking`
  - `top_p`
  - `tools`
  - `tool_choice`
- Remove `thinking.signature` and `redacted_thinking` for DeepSeek if present.
- Convert unsupported media to text placeholders by default:
  - `[Unsupported image omitted by baixin-switch]`
  - `[Unsupported document omitted by baixin-switch]`

Rectifier output must include an action list for logs and metrics:

```json
["drop_redacted_thinking", "media_placeholder", "drop_top_k"]
```

## 10. Observability

### Request ID

Every request gets:

- `X-Request-ID` from client if present.
- Generated UUID/ULID if missing.
- Returned in downstream response header.
- Forwarded upstream as `X-Request-ID`.

### Structured Logs

Use `log/slog` with JSON output.

Access log fields:

```json
{
  "ts": "2026-06-12T10:00:00Z",
  "level": "info",
  "event": "request_completed",
  "request_id": "req_...",
  "trace_id": "...",
  "client_ip": "10.0.0.1",
  "method": "POST",
  "path": "/v1/chat/completions",
  "source_api": "openai_chat",
  "target_api": "anthropic_messages",
  "model": "deepseek-v4-pro",
  "stream": true,
  "status": 200,
  "duration_ms": 1532,
  "upstream_status": 200,
  "upstream_duration_ms": 1501,
  "input_tokens": 123,
  "output_tokens": 45,
  "tool_calls": 1,
  "rectifier_actions": ["drop_top_k"],
  "error_code": ""
}
```

Do not log:

- API keys.
- Full prompts by default.
- Full tool arguments by default.
- Raw upstream response bodies by default.

Debug-only sampling:

```text
LOG_BODY_SAMPLE=false
LOG_BODY_SAMPLE_BYTES=512
LOG_BODY_SAMPLE_RATE=0.001
```

### Metrics

Expose `/metrics` in Prometheus format.

Counters:

```text
baixin_http_requests_total{method,path,status}
baixin_upstream_requests_total{upstream,status}
baixin_conversion_errors_total{stage,reason}
baixin_rectifier_actions_total{action}
baixin_sse_events_total{source,event_type}
baixin_tool_call_stream_shim_total{mode}
baixin_tokens_total{direction}
```

Gauges:

```text
baixin_active_requests
baixin_active_streams
baixin_limiter_available_slots{limiter}
baixin_upstream_inflight_requests
```

Histograms:

```text
baixin_http_request_duration_seconds{method,path}
baixin_upstream_request_duration_seconds{upstream}
baixin_stream_duration_seconds{path}
baixin_first_byte_duration_seconds{path,upstream}
baixin_request_body_bytes{path}
baixin_response_body_bytes{path}
```

### Tracing

Use OpenTelemetry:

```text
client request span
  parse openai span
  convert request span
  rectifier span
  upstream request span
  stream translate span
```

Trace IDs are written into logs.

### Health Endpoints

```text
GET /healthz
  process is alive

GET /readyz
  config is valid and concurrency limiter can accept traffic

GET /metrics
  Prometheus metrics

GET /debug/pprof/*
  disabled by default; enable only in trusted private network
```

Keep existing `/health` as an alias for compatibility.

## 11. Single-Instance 1000 Concurrency Design

### Key Requirement

Prefer a single instance if it can support 1000 concurrent requests. The architecture should not require cluster coordination.

### Main Rule

No full-stream buffering.

The current string-buffer SSE normalizer must be replaced in production paths with:

```text
upstream response body
  -> streaming SSE decoder
  -> event transformer
  -> downstream writer
  -> http.Flusher.Flush()
```

### HTTP Server

Recommended server settings:

```go
http.Server{
    Addr:              cfg.ListenAddr,
    Handler:           handler,
    ReadHeaderTimeout: 5 * time.Second,
    ReadTimeout:       30 * time.Second,
    WriteTimeout:      0, // streaming responses can be long-lived
    IdleTimeout:       120 * time.Second,
    MaxHeaderBytes:    1 << 20,
}
```

### Upstream Transport

Recommended transport settings:

```go
http.Transport{
    Proxy:                 http.ProxyFromEnvironment,
    MaxIdleConns:          2000,
    MaxIdleConnsPerHost:   1000,
    MaxConnsPerHost:       1200,
    IdleConnTimeout:       90 * time.Second,
    TLSHandshakeTimeout:   10 * time.Second,
    ResponseHeaderTimeout: 60 * time.Second,
    ExpectContinueTimeout: 1 * time.Second,
}
```

Do not set a short `http.Client.Timeout` for streaming requests. Use:

- request context cancellation;
- response header timeout;
- optional stream idle timeout.

### Limiters

Add process-local semaphores:

```text
MAX_CONCURRENT_REQUESTS=1200
MAX_CONCURRENT_STREAMS=1000
MAX_REQUEST_BODY_BYTES=8388608
STREAM_IDLE_TIMEOUT_SECONDS=300
```

Behavior:

- If request slots are exhausted, return HTTP 429 with OpenAI-compatible error.
- If stream slots are exhausted, return HTTP 429 before contacting upstream.
- Release slots on request completion or client disconnect.

### Cancellation

Use request context everywhere:

```text
client disconnect
  -> request context canceled
  -> upstream request canceled
  -> stream goroutine exits
  -> limiter slot released
```

### Memory Target

For 1000 active streams:

- Avoid per-request buffers larger than 64 KiB.
- Stream tool arguments as deltas; do not accumulate full responses except for a single tool input buffer if needed.
- Keep logs lightweight.
- Avoid goroutine fan-out per SSE event.

Recommended initial sizing:

```text
CPU: 4-8 cores
Memory: 2-4 GiB
ulimit -n: >= 65535
```

### Load Testing

Add load tests using `vegeta` or `wrk`.

Targets:

```text
1000 concurrent streaming requests
p95 first-byte latency observed
no unbounded memory growth
no goroutine leak after test
error rate < 0.1% excluding upstream errors
```

Minimum test commands:

```bash
go test ./...
go test -run TestNoGoroutineLeak ./internal/...
vegeta attack -duration=5m -rate=1000/s -body chat_stream.json -header "Content-Type: application/json" http://host/v1/chat/completions
```

For streaming concurrency, use a custom Go load harness if `vegeta` does not model long-lived SSE streams accurately enough.

## 12. Error Handling

All client-facing errors should be OpenAI-compatible:

```json
{
  "error": {
    "type": "upstream_error",
    "code": "upstream_502",
    "message": "Upstream returned 502"
  }
}
```

Error classes:

```text
invalid_request_error
conversion_error
rectifier_error
upstream_connection_error
upstream_timeout
upstream_error
rate_limit_exceeded
stream_error
```

Streaming errors:

- If headers are not sent yet, return HTTP error JSON.
- If SSE has started, emit an OpenAI-compatible error event when possible, then close.
- Always log `request_id`, `stage`, and `error_code`.

## 13. Configuration

New configuration:

```text
MODE=anthropic_messages               # anthropic_messages | openai_passthrough
LISTEN_ADDR=0.0.0.0:11435
UPSTREAM_BASE_URL=https://api.deepseek.com/anthropic
UPSTREAM_API_KEY=...
UPSTREAM_API_FORMAT=anthropic_messages
DEFAULT_MODEL=deepseek-v4-pro
MODEL_MAP={"gpt-4.1":"deepseek-v4-pro"}

MAX_CONCURRENT_REQUESTS=1200
MAX_CONCURRENT_STREAMS=1000
MAX_REQUEST_BODY_BYTES=8388608
STREAM_IDLE_TIMEOUT_SECONDS=300

LOG_LEVEL=info
LOG_FORMAT=json
LOG_BODY_SAMPLE=false

METRICS_ENABLED=true
TRACING_ENABLED=false
OTEL_EXPORTER_OTLP_ENDPOINT=
PPROF_ENABLED=false

TOOL_CALL_STREAM_SHIM=true
TOOL_CALL_ARGUMENT_CHUNK_SIZE=16
MEDIA_MODE=placeholder
STRICT_PASSTHROUGH=false
```

## 14. Implementation Phases

### Phase 1: Production Runtime and Observability

- Introduce `internal/server`, `internal/observability`, and `internal/limits`.
- Add request IDs.
- Add JSON access logs.
- Add `/healthz`, `/readyz`, `/metrics`.
- Add concurrency limiters.
- Replace full-stream buffering with streaming SSE decode/write for current passthrough normalizer.
- Add stream idle timeout.

Acceptance:

- Existing tests pass.
- New tests prove streaming output is flushed incrementally.
- Metrics expose active request/stream counts.

### Phase 2: OpenAI Chat -> Anthropic Messages

- Add IR structs.
- Add OpenAI Chat request parser.
- Add Anthropic Messages request builder.
- Add non-streaming Anthropic response -> OpenAI Chat response conversion.
- Add Anthropic SSE -> OpenAI Chat chunk conversion.
- Add DeepSeek rectifier.

Acceptance:

- Text chat non-streaming works.
- Text chat streaming works.
- Tool use streaming works with `input_json_delta`.
- Tool result follow-up works.
- Stop reasons map correctly.

### Phase 3: OpenAI Responses -> Anthropic Messages

- Add Responses parser.
- Add Responses writer.
- Add Anthropic SSE -> Responses event translator.

Acceptance:

- Codex-style Responses requests work for text and tool calls.
- Streaming Responses event sequence is valid.

### Phase 4: Load and Operations

- Add 1000-stream load harness.
- Add deployment tuning docs.
- Add dashboard examples.
- Add pprof runbook.

Acceptance:

- Single instance sustains 1000 concurrent streams on target hardware.
- Memory remains bounded.
- Client disconnect cancels upstream.
- Metrics and logs identify slow upstreams and conversion errors.

## 15. Test Strategy

Unit tests:

- OpenAI Chat -> IR.
- IR -> Anthropic Messages.
- DeepSeek rectifier actions.
- Anthropic non-stream response -> OpenAI Chat.
- Anthropic SSE -> OpenAI Chat SSE.
- Stop reason mapping.
- Error mapping.

Integration tests:

- Fake Anthropic upstream non-streaming.
- Fake Anthropic upstream streaming text.
- Fake Anthropic upstream streaming tool use.
- Client cancellation cancels upstream.
- Concurrency limiter returns 429.

Load tests:

- 100 concurrent smoke test.
- 1000 concurrent stream test.
- Long-running stream idle timeout test.
- Upstream slow-first-byte test.

## 16. Source References

- Anthropic Messages API: https://docs.anthropic.com/en/api/messages
- Anthropic Streaming Messages: https://docs.anthropic.com/en/docs/build-with-claude/streaming
- OpenAI Chat Completions API: https://platform.openai.com/docs/api-reference/chat/create
- OpenAI Responses API: https://platform.openai.com/docs/api-reference/responses/create
- Go `net/http`: https://pkg.go.dev/net/http
- Prometheus Go client: https://github.com/prometheus/client_golang
