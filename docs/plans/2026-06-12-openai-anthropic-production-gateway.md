# OpenAI-to-Anthropic Production Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade `baixin-switch` from OpenAI-compatible passthrough into a production gateway that converts `/v1/chat/completions` to Anthropic Messages API with observability and single-instance concurrency controls.

**Architecture:** Add a small internal IR, conversion adapters, Anthropic upstream client, streaming SSE translator, JSON access logs, `/metrics`, `/healthz`, `/readyz`, and process-local request/stream limiters. Keep existing OpenAI-compatible passthrough and DS Pro stream shim available via `MODE=openai_passthrough`; default production direction becomes `MODE=anthropic_messages`.

**Tech Stack:** Go 1.22, standard library first, `log/slog`, `net/http`, internal metrics renderer in Prometheus text format, no database.

---

## File Structure

- Modify: `go.mod`
- Modify: `cmd/baixin-switch/main.go`
- Modify: `internal/config/config.go`
- Modify: `internal/proxy/server.go`
- Modify: `internal/proxy/server_test.go`
- Create: `internal/ir/types.go`
- Create: `internal/anthropic/types.go`
- Create: `internal/convert/openai_chat_to_ir.go`
- Create: `internal/convert/ir_to_anthropic.go`
- Create: `internal/convert/anthropic_to_openai_chat.go`
- Create: `internal/convert/convert_test.go`
- Create: `internal/observability/metrics.go`
- Create: `internal/observability/metrics_test.go`
- Create: `internal/limits/limiter.go`
- Create: `internal/limits/limiter_test.go`
- Create: `internal/sse/anthropic_chat.go`
- Create: `internal/sse/anthropic_chat_test.go`
- Update: `README.md`
- Update: `docs/private-deployment-guide.md`
- Update: `docs/user-guide.md`

## Task 1: IR and Request Conversion

- [x] Write failing tests in `internal/convert/convert_test.go` covering OpenAI Chat messages/tools/tool results -> IR -> Anthropic Messages.
- [x] Run `go test ./internal/convert` and verify it fails due to missing package.
- [x] Implement `internal/ir/types.go`, `internal/anthropic/types.go`, and request converters.
- [x] Run `go test ./internal/convert` and verify it passes.

## Task 2: Anthropic Response Conversion

- [x] Add failing tests for Anthropic non-streaming text/tool_use response -> OpenAI Chat response.
- [x] Run `go test ./internal/convert` and verify expected failure.
- [x] Implement non-streaming response conversion.
- [x] Run `go test ./internal/convert` and verify it passes.

## Task 3: Anthropic SSE -> OpenAI Chat SSE

- [x] Add failing tests in `internal/sse/anthropic_chat_test.go` for text delta and tool input_json_delta conversion.
- [x] Run `go test ./internal/sse` and verify expected failure.
- [x] Implement streaming translator that reads Anthropic SSE blocks and writes OpenAI Chat SSE blocks incrementally.
- [x] Run `go test ./internal/sse` and verify it passes.

## Task 4: Observability and Limits

- [x] Add failing tests for limiter 429 behavior and metrics rendering.
- [x] Run `go test ./internal/limits ./internal/observability` and verify expected failure.
- [x] Implement simple atomic metrics registry and semaphore limiter.
- [x] Run `go test ./internal/limits ./internal/observability` and verify it passes.

## Task 5: HTTP Integration

- [x] Add failing proxy tests with fake Anthropic upstream:
  - non-streaming Chat request becomes Anthropic `/v1/messages`;
  - streaming Anthropic response becomes OpenAI Chat SSE;
  - `/metrics`, `/healthz`, `/readyz` work;
  - limiter returns 429.
- [x] Run `go test ./internal/proxy` and verify expected failure.
- [x] Update proxy server to route by `MODE`:
  - `anthropic_messages`: OpenAI Chat -> Anthropic Messages;
  - `openai_passthrough`: existing passthrough + DS Pro shim.
- [x] Run `go test ./internal/proxy` and verify it passes.

## Task 6: Docs and Verification

- [x] Update README and manuals for `MODE=anthropic_messages`.
- [x] Run `go test ./...`.
- [x] Run `go build ./cmd/baixin-switch`.

## Notes After Task 5

- `MODE=anthropic_messages` is now the default.
- `MODE=openai_passthrough` preserves the original DS Pro one-shot tool-call stream shim.
- Anthropic SSE conversion has a writer-based streaming path used by the proxy.
- The legacy passthrough normalizer still buffers full SSE output; keep this in mind if passthrough mode also needs 1000 long-lived streams.
