# baixin-switch

`baixin-switch` is a private Go gateway for internal coding assistants.

The default runtime mode converts:

```text
OpenAI-compatible Chat Completions
  -> baixin-switch
  -> Anthropic Messages-compatible upstream
```

It also keeps the earlier DS Pro/vLLM compatibility shim as `MODE=openai_passthrough`: when an upstream returns tool calls with a complete `function.arguments` value in one SSE frame, the proxy can rewrite it into OpenAI-style incremental tool-call deltas.

## Run

```bash
export MODE="anthropic_messages"
export UPSTREAM_BASE_URL="https://your-ds-pro-anthropic-gateway.example.com"
export UPSTREAM_API_KEY="sk-..."
export DEFAULT_MODEL="deepseek-v4-pro"
go run ./cmd/baixin-switch
```

The service listens on `127.0.0.1:11435` by default.

Point Comate or another OpenAI-compatible client at:

```text
http://127.0.0.1:11435/v1
```

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `LISTEN_ADDR` | `127.0.0.1:11435` | Listen address. Use `0.0.0.0:11435` for private network deployment. |
| `MODE` | `anthropic_messages` | `anthropic_messages` converts OpenAI Chat to Anthropic Messages. `openai_passthrough` forwards OpenAI Chat and enables the DS Pro stream shim. |
| `UPSTREAM_BASE_URL` | `https://api.deepseek.com` | Upstream base URL. In default mode this should expose `/v1/messages`. |
| `UPSTREAM_API_KEY` | empty | Token sent to upstream as `Authorization: Bearer` and `x-api-key`. |
| `ANTHROPIC_VERSION` | `2023-06-01` | `anthropic-version` header for Anthropic-compatible upstreams. |
| `DEFAULT_MODEL` | `deepseek-v4-pro` | Model used when the OpenAI request omits `model`. |
| `MODEL_MAP` | empty | Comma-separated mapping, for example `gpt-4.1=deepseek-v4-pro,claude-sonnet=deepseek-v4-pro`. |
| `MAX_CONCURRENT_REQUESTS` | `1000` | Process-local active request limit. Excess requests get HTTP 429. |
| `MAX_CONCURRENT_STREAMS` | `1000` | Process-local active streaming response limit. |
| `TOOL_CALL_STREAM_SHIM` | `true` | Only used in `openai_passthrough`; rewrites one-shot tool-call arguments into incremental deltas. |
| `TOOL_CALL_ARGUMENT_CHUNK_SIZE` | `16` | Rune count per emitted argument delta in passthrough shim mode. |
| `REQUEST_TIMEOUT_SECONDS` | `600` | Upstream request timeout. |
| `AUDIT_ENABLED` | `true` | Write one JSONL audit event per model call. |
| `AUDIT_LOG_DIR` | `./logs` | Directory for usage/audit JSONL storage. The default file is `usage.jsonl`. |
| `AUDIT_LOG_PATH` | derived from `AUDIT_LOG_DIR` | Advanced override for the exact JSONL file path. Takes precedence over `AUDIT_LOG_DIR`. |
| `AUDIT_CAPTURE_BODY` | `preview` | `off`, `preview`, or `full`. Preview/full are redacted; full body capture is opt-in. |
| `AUDIT_PREVIEW_CHARS` | `2000` | Max preview rune count. |
| `AUDIT_QUEUE_SIZE` | `8192` | Async audit queue size. |
| `AUDIT_OVERFLOW_POLICY` | `drop` | `drop` protects latency; `sync` protects audit completeness. |

## Supported Endpoints

- `GET /health`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `POST /v1/chat/completions`

In default `anthropic_messages` mode:

```text
POST /v1/chat/completions
  -> ${UPSTREAM_BASE_URL}/v1/messages
```

Supported Chat fields include `messages`, `tools`, `tool_choice`, `max_tokens`, `max_completion_tokens`, `temperature`, `top_p`, `stop`, and `stream`.

## Session Observability

By default, `baixin-switch` writes redacted and truncated request/response previews to:

```text
./logs/usage.jsonl
```

Session id is resolved from `X-Session-ID`, `X-Conversation-ID`, body `metadata.session_id`, or `X-Request-ID`. Responses include `X-Request-ID` and `X-Session-ID`.

Usage reports:

```bash
./scripts/usage_report.sh --by day
./scripts/usage_report.sh --by week
./scripts/usage_report.sh --by session
./scripts/usage_report.sh --session sess_abc
```

Use `AUDIT_CAPTURE_BODY=full` only when operators explicitly accept prompt/output storage risk.

## Tests

```bash
GOCACHE="$(pwd)/.cache/go-build" go test ./...
```

## Development Docs

The analysis, design notes, manuals, and implementation plans are kept in the project for ongoing development:

- [CHANGELOG.md](CHANGELOG.md)
- [docs/analysis-and-design.md](docs/analysis-and-design.md)
- [docs/openai-to-anthropic-observability-concurrency-design.md](docs/openai-to-anthropic-observability-concurrency-design.md)
- [docs/private-deployment-guide.md](docs/private-deployment-guide.md)
- [docs/user-guide.md](docs/user-guide.md)
- [docs/session-observability-jsonl-design.md](docs/session-observability-jsonl-design.md)
- [docs/plans/2026-06-12-baixin-switch-proxy.md](docs/plans/2026-06-12-baixin-switch-proxy.md)
- [docs/plans/2026-06-12-openai-anthropic-production-gateway.md](docs/plans/2026-06-12-openai-anthropic-production-gateway.md)
- [docs/plans/2026-06-12-session-observability-jsonl.md](docs/plans/2026-06-12-session-observability-jsonl.md)

## Docker

```bash
docker build -t baixin-switch .
docker run --rm -p 11435:11435 \
  -e LISTEN_ADDR=0.0.0.0:11435 \
  -e MODE=anthropic_messages \
  -e UPSTREAM_BASE_URL=https://your-ds-pro-anthropic-gateway.example.com \
  -e UPSTREAM_API_KEY=sk-... \
  -e DEFAULT_MODEL=deepseek-v4-pro \
  -e MAX_CONCURRENT_REQUESTS=1000 \
  -e MAX_CONCURRENT_STREAMS=1000 \
  baixin-switch
```
