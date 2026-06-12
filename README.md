# baixin-switch

`baixin-switch` is a small private OpenAI-compatible proxy for internal coding assistants.

Phase 1 focuses on `/v1/chat/completions` and the urgent DS Pro compatibility issue: some DS Pro/vLLM deployments stream tool calls as one complete `function.arguments` value, while Comate expects incremental OpenAI-style tool-call deltas. This proxy rewrites those one-shot chunks into multiple delta chunks.

## Run

```bash
export UPSTREAM_BASE_URL="https://your-ds-pro-gateway.example.com"
export UPSTREAM_API_KEY="sk-..."
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
| `LISTEN_ADDR` | `127.0.0.1:11435` | Local listen address. |
| `UPSTREAM_BASE_URL` | `https://api.deepseek.com` | Upstream OpenAI-compatible base URL. |
| `UPSTREAM_API_KEY` | empty | Bearer token sent to upstream. |
| `TOOL_CALL_STREAM_SHIM` | `true` | Rewrite one-shot tool-call arguments into incremental deltas. |
| `TOOL_CALL_ARGUMENT_CHUNK_SIZE` | `16` | Rune count per emitted argument delta. |
| `REQUEST_TIMEOUT_SECONDS` | `600` | Upstream request timeout. |

## Supported Endpoints

- `GET /health`
- `POST /v1/chat/completions`

The proxy forwards the request body to:

```text
${UPSTREAM_BASE_URL}/v1/chat/completions
```

For `text/event-stream` responses, it normalizes DS Pro one-shot tool-call chunks. Non-streaming responses are passed through unchanged.

## Tests

```bash
GOCACHE="$(pwd)/.cache/go-build" go test ./...
```

## Development Docs

The analysis, design notes, and implementation plan are kept in the project for ongoing development:

- [docs/analysis-and-design.md](docs/analysis-and-design.md)
- [docs/openai-to-anthropic-observability-concurrency-design.md](docs/openai-to-anthropic-observability-concurrency-design.md)
- [docs/private-deployment-guide.md](docs/private-deployment-guide.md)
- [docs/user-guide.md](docs/user-guide.md)
- [docs/plans/2026-06-12-baixin-switch-proxy.md](docs/plans/2026-06-12-baixin-switch-proxy.md)

## Docker

```bash
docker build -t baixin-switch .
docker run --rm -p 11435:11435 \
  -e LISTEN_ADDR=0.0.0.0:11435 \
  -e UPSTREAM_BASE_URL=https://your-ds-pro-gateway.example.com \
  -e UPSTREAM_API_KEY=sk-... \
  baixin-switch
```
