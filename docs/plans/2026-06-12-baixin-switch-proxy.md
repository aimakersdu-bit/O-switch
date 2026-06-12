# Baixin Switch Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone Go proxy in `baixin-switch` that can forward OpenAI-compatible requests and normalize DS Pro one-shot streaming tool calls for Comate.

**Architecture:** The service exposes OpenAI-compatible HTTP endpoints, forwards requests to a configurable upstream, and transforms streaming Chat Completions SSE on the response path. The first production-critical behavior is the DS Pro tool-call stream shim: when upstream sends a complete `function.arguments` in one chunk, the proxy emits OpenAI-style incremental argument deltas.

**Tech Stack:** Go standard library only for phase 1 (`net/http`, `encoding/json`, `bufio`, `httptest`, `log/slog`), Docker for deployment.

---

## File Structure

- `baixin-switch/go.mod`: Go module declaration.
- `baixin-switch/cmd/baixin-switch/main.go`: binary entrypoint.
- `baixin-switch/internal/config/config.go`: environment-driven config.
- `baixin-switch/internal/openai/types.go`: minimal OpenAI-compatible JSON structures.
- `baixin-switch/internal/sse/normalizer.go`: Chat Completions SSE parser and DS Pro tool-call chunk normalizer.
- `baixin-switch/internal/sse/normalizer_test.go`: unit tests for one-shot and already-incremental tool calls.
- `baixin-switch/internal/proxy/server.go`: HTTP handlers, upstream forwarding, response streaming.
- `baixin-switch/internal/proxy/server_test.go`: integration tests with fake upstream.
- `baixin-switch/README.md`: usage, config, and Comate compatibility note.
- `baixin-switch/Dockerfile`: small runtime image.

## Task 1: Go Module and Tool-Call Normalizer

**Files:**
- Create: `baixin-switch/go.mod`
- Create: `baixin-switch/internal/openai/types.go`
- Create: `baixin-switch/internal/sse/normalizer.go`
- Test: `baixin-switch/internal/sse/normalizer_test.go`

- [ ] **Step 1: Write failing tests**

Create tests that:

- feed a DS Pro-style one-shot `tool_calls[].function.arguments` chunk;
- expect one metadata chunk with empty arguments plus multiple argument delta chunks;
- feed GLM-style incremental chunks;
- expect unchanged output.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sse`
Expected: FAIL because module/package does not exist yet or normalizer is missing.

- [ ] **Step 3: Implement minimal normalizer**

Implement an SSE block parser that reads `data:` blocks, decodes JSON chunks, splits one-shot tool-call arguments on UTF-8-safe rune boundaries, and passes through existing incremental chunks.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/sse`
Expected: PASS.

## Task 2: HTTP Proxy Server

**Files:**
- Create: `baixin-switch/internal/config/config.go`
- Create: `baixin-switch/internal/proxy/server.go`
- Test: `baixin-switch/internal/proxy/server_test.go`
- Create: `baixin-switch/cmd/baixin-switch/main.go`

- [ ] **Step 1: Write failing integration tests**

Create tests that:

- start a fake upstream returning DS Pro-style SSE;
- call local `/v1/chat/completions`;
- verify the proxy returns multiple `data:` chunks and preserves `[DONE]`;
- verify `/health` returns JSON status.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/proxy`
Expected: FAIL because server is missing.

- [ ] **Step 3: Implement server**

Implement:

- `GET /health`;
- `POST /v1/chat/completions` forwarding to upstream `/v1/chat/completions`;
- streaming response path through `sse.NormalizeChatCompletionStream`;
- non-streaming passthrough;
- configurable upstream URL and API key.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/proxy`
Expected: PASS.

## Task 3: Docs and Deployment

**Files:**
- Create: `baixin-switch/README.md`
- Create: `baixin-switch/Dockerfile`

- [ ] **Step 1: Document runtime**

Document env vars:

- `LISTEN_ADDR`
- `UPSTREAM_BASE_URL`
- `UPSTREAM_API_KEY`
- `TOOL_CALL_STREAM_SHIM`
- `TOOL_CALL_ARGUMENT_CHUNK_SIZE`

- [ ] **Step 2: Add Dockerfile**

Add a two-stage Dockerfile that builds `cmd/baixin-switch` and runs it in a small runtime image.

- [ ] **Step 3: Verify all tests**

Run: `go test ./...`
Expected: PASS.
