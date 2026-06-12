# Session Observability and Token Usage JSONL Design

Date: 2026-06-12

## 1. Decision

Use **JSONL append-only audit logs plus offline/lightweight report scripts**.

This keeps `baixin-switch` simple and privately deployable while adding the required ability to inspect one session's model request/response and summarize token usage by day, session, and week.

This design intentionally does not introduce PostgreSQL, SQLite, Redis, or an admin UI in this phase.

## 2. Reference From sub2api

`sub2api` provides useful production patterns:

- append-only usage records;
- separate request path from usage recording;
- include request id, model, stream flag, duration, token counts, and timestamps;
- aggregate usage by time windows and dimensions;
- provide request-detail views for troubleshooting.

`baixin-switch` should borrow those patterns without copying the full platform architecture. The target is a lightweight single-binary proxy, so the storage layer is a JSONL file and the reporting layer is a script/CLI that scans the file.

## 3. Goals

- Record each model call as one JSON object in an append-only `.jsonl` file.
- Allow operators to inspect a specific `session_id`.
- Track token usage by:
  - day;
  - week;
  - session.
- Capture request/response body previews by default with redaction and truncation.
- Allow full body capture with an explicit environment variable.
- Keep audit writing off the critical request path through an asynchronous bounded queue.
- Remain safe under 1000 concurrent requests.

## 4. Non-Goals

- No web dashboard in this phase.
- No database migrations.
- No per-user billing.
- No client authentication model.
- No long-term archival or compression daemon.
- No exact token estimation when upstream usage is absent; record `usage_source=missing` instead.

## 5. Runtime Config

| Variable | Default | Description |
| --- | --- | --- |
| `AUDIT_ENABLED` | `true` | Enables JSONL audit recording. |
| `AUDIT_LOG_DIR` | `./logs` | Directory for append-only JSONL storage. The default file name is `usage.jsonl`. |
| `AUDIT_LOG_PATH` | derived from `AUDIT_LOG_DIR` | Advanced exact JSONL path override. Takes precedence over `AUDIT_LOG_DIR`. |
| `AUDIT_CAPTURE_BODY` | `preview` | `off`, `preview`, or `full`. |
| `AUDIT_PREVIEW_CHARS` | `2000` | Maximum rune count for request/response previews. |
| `AUDIT_QUEUE_SIZE` | `8192` | Buffered audit event queue size. |
| `AUDIT_OVERFLOW_POLICY` | `drop` | `drop` or `sync`. `drop` protects latency; `sync` protects completeness. |
| `AUDIT_REDACT_HEADERS` | `Authorization,Cookie,Set-Cookie,X-API-Key` | Header names to redact if header capture is added later. |

Recommended production default:

```bash
AUDIT_ENABLED=true
AUDIT_CAPTURE_BODY=preview
AUDIT_OVERFLOW_POLICY=drop
```

Full capture must be an explicit operational choice:

```bash
AUDIT_CAPTURE_BODY=full
```

## 6. Session Identity

Session id is resolved in this order:

1. `X-Session-ID`
2. `X-Conversation-ID`
3. body metadata if available:
   - `metadata.session_id`
   - `metadata.conversation_id`
4. `X-Request-ID`
5. generated `request_id`

Request id is resolved in this order:

1. `X-Request-ID`
2. generated id, for example `req_<timestamp>_<random>`

Downstream response headers should include:

```text
X-Request-ID: <request_id>
X-Session-ID: <session_id>
```

## 7. JSONL Schema

Each model call writes one JSON object:

```json
{
  "schema_version": 1,
  "ts": "2026-06-12T11:40:00+08:00",
  "date": "2026-06-12",
  "week": "2026-W24",
  "request_id": "req_abc",
  "session_id": "sess_abc",
  "mode": "anthropic_messages",
  "endpoint": "/v1/chat/completions",
  "upstream_endpoint": "/v1/messages",
  "model": "deepseek-v4-pro",
  "requested_model": "gpt-4.1",
  "upstream_model": "deepseek-v4-pro",
  "stream": true,
  "status": 200,
  "upstream_status": 200,
  "duration_ms": 1234,
  "first_token_ms": 320,
  "input_tokens": 1200,
  "output_tokens": 380,
  "total_tokens": 1580,
  "usage_source": "upstream",
  "error_code": "",
  "error_message": "",
  "request_preview": "...",
  "response_preview": "...",
  "request_body": null,
  "response_body": null,
  "truncated": true,
  "redacted": true
}
```

### Body Fields

For `AUDIT_CAPTURE_BODY=off`:

- `request_preview` empty;
- `response_preview` empty;
- `request_body` null;
- `response_body` null.

For `AUDIT_CAPTURE_BODY=preview`:

- `request_preview` contains redacted and truncated request text;
- `response_preview` contains redacted and truncated response text;
- `request_body` null;
- `response_body` null.

For `AUDIT_CAPTURE_BODY=full`:

- `request_body` contains redacted full body;
- `response_body` contains redacted full body;
- previews may still be populated for quick scanning.

## 8. Redaction and Truncation

Redaction applies before truncation.

Default redaction rules:

- JSON keys containing `api_key`, `authorization`, `token`, `secret`, `password`, `cookie` become `"[REDACTED]"`.
- Strings matching `Bearer <value>` become `Bearer [REDACTED]`.
- Long scalar strings are capped by `AUDIT_PREVIEW_CHARS` in preview mode.

If JSON parsing fails, treat the body as text and apply regex-level redaction plus truncation.

The audit module must never log upstream/client API keys.

## 9. Request Path Data Flow

```text
HTTP handler
  -> resolve request_id/session_id
  -> read inbound body
  -> capture request preview/full body according to config
  -> convert and forward upstream
  -> capture usage from non-stream response or stream events
  -> capture response preview/full body according to config
  -> enqueue audit event
  -> return response to client
```

For non-streaming responses, usage is read from the Anthropic response:

```json
"usage": {
  "input_tokens": 1,
  "output_tokens": 1
}
```

For streaming responses, token usage should be captured from Anthropic stream events when present:

- `message_start.message.usage.input_tokens`
- `message_delta.usage.output_tokens`
- `message_stop` if usage appears in provider-specific variants

If upstream usage is absent:

- record token fields as `0`;
- set `usage_source` to `missing`;
- do not estimate unless a later design explicitly adds a tokenizer dependency.

## 10. Asynchronous Writer

The audit writer is a small service:

```go
type Recorder interface {
    Record(event Event)
    Close(ctx context.Context) error
}
```

Behavior:

- creates parent directory for `AUDIT_LOG_PATH`;
- opens the file with append mode;
- serializes events as one compact JSON object per line;
- protects file writes with a single writer goroutine;
- uses a bounded channel sized by `AUDIT_QUEUE_SIZE`;
- exposes counters for written and dropped audit events.

Overflow behavior:

- `drop`: increment dropped counter and continue request path;
- `sync`: write from request goroutine if queue is full.

`drop` is the default to protect request latency under 1000 concurrent traffic.

## 11. Metrics

Extend `/metrics` with:

```text
baixin_audit_events_total{result="written|dropped|sync"} <count>
baixin_audit_queue_depth <gauge>
baixin_tokens_total{direction="input|output|total"} <count>
```

Metrics are process-local and reset on restart. JSONL remains the durable source for day/week/session reports.

## 12. Reporting Script

Add:

```text
scripts/usage_report.sh
scripts/usage_report.py
```

The shell script is a thin wrapper around Python for portability.

Supported commands:

```bash
# by day
./scripts/usage_report.sh --by day --from 2026-06-01 --to 2026-06-12

# by week
./scripts/usage_report.sh --by week

# by session
./scripts/usage_report.sh --by session

# inspect one session timeline
./scripts/usage_report.sh --session sess_abc

# JSON output
./scripts/usage_report.sh --by day --json
```

Default input path:

```text
./logs/usage.jsonl
```

Override:

```bash
./scripts/usage_report.sh --file /data/baixin/usage.jsonl --by day
```

### Report Output

Daily table:

```text
date         requests  input_tokens  output_tokens  total_tokens
2026-06-12   1200      640000        210000         850000
```

Weekly table:

```text
week      requests  input_tokens  output_tokens  total_tokens
2026-W24  8120      4200000       1300000        5500000
```

Session table:

```text
session_id  requests  input_tokens  output_tokens  total_tokens  first_seen            last_seen
sess_abc    12        20000         6000           26000         2026-06-12T10:00    2026-06-12T10:30
```

Session timeline:

```text
ts                         request_id  model            status  input  output  total  preview
2026-06-12T10:01:02+08:00  req_1       deepseek-v4-pro  200     800    200     1000   user: ...
```

## 13. File Retention

Phase 1 leaves rotation to the deployment layer:

- Docker logrotate;
- systemd timer;
- cron;
- external log collection.

Recommended operational pattern:

```text
/var/log/baixin-switch/usage.jsonl
/var/log/baixin-switch/usage-2026-06-12.jsonl.gz
```

A built-in rotator can be added later if operations require it.

## 14. Error Handling

Audit recording must not break the proxy:

- if audit file cannot open, service starts with audit disabled and logs an error;
- if individual event marshaling fails, increment dropped counter;
- if queue is full and policy is `drop`, increment dropped counter;
- if queue is full and policy is `sync`, write synchronously and increment sync counter.

For failed model calls, still write an audit event with:

- `status`;
- `upstream_status` if known;
- `error_code`;
- `error_message`;
- duration;
- token counts as zero unless usage is available.

## 15. Testing Strategy

Unit tests:

- session id resolution from headers and metadata;
- redaction rules;
- preview truncation;
- JSONL writer writes one event per line;
- queue overflow policies;
- report script aggregation by day/week/session.

Proxy integration tests:

- non-streaming Anthropic response records usage;
- streaming Anthropic SSE records usage;
- failed upstream response records an audit event;
- response headers include `X-Request-ID` and `X-Session-ID`.

Manual verification:

```bash
AUDIT_ENABLED=true AUDIT_LOG_DIR=/tmp/baixin-audit go run ./cmd/baixin-switch
./scripts/usage_report.sh --file /tmp/baixin-audit/usage.jsonl --by day
```

## 16. Implementation Phases

### Phase 1: Core Audit Event

- Add audit config.
- Add event type, redactor, and recorder.
- Wire request/session ids into proxy responses.
- Record non-streaming request/response usage.
- Add metrics counters.

### Phase 2: Streaming Usage and Previews

- Extend Anthropic SSE writer to report stream usage and response preview deltas.
- Record streaming audit events.
- Keep streaming response path incremental.

### Phase 3: Reporting

- Add `scripts/usage_report.py`.
- Add shell wrapper.
- Support `--by day`, `--by week`, `--by session`, `--session`, `--json`.

### Phase 4: Docs and Operations

- Update README, deployment guide, and user guide.
- Document privacy behavior and full-capture risk.
- Add logrotate/systemd examples.

## 17. Open Risks

- Full body capture can store sensitive prompts and outputs. It must remain opt-in.
- JSONL scanning is acceptable for small/medium files. Very large installations may later need SQLite/PostgreSQL.
- Streaming usage availability depends on upstream events. When usage is absent, reports will undercount and mark `usage_source=missing`.
- If deployment rotates logs while the process keeps the file descriptor open, operators need copytruncate-style rotation or a future reopen signal.
