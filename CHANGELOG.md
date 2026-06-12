# Changelog

## 2026-06-12

### Added

- Added `MODE=anthropic_messages` gateway support for OpenAI-compatible Chat Completions to Anthropic Messages.
- Added session-level JSONL audit logging for model requests and responses.
- Added token usage reporting by day, week, and session through `scripts/usage_report.sh`.
- Added `AUDIT_LOG_DIR` to configure the audit storage directory. The default audit file is `usage.jsonl` under that directory.
- Added `AUDIT_LOG_PATH` as an advanced exact-file override, with higher priority than `AUDIT_LOG_DIR`.
- Added Prometheus metrics for audit events, audit queue depth, and token totals.

### Changed

- Audit body capture defaults to redacted and truncated previews through `AUDIT_CAPTURE_BODY=preview`.
- Responses now include `X-Request-ID` and `X-Session-ID` in Anthropic Messages mode.

### Notes

- `AUDIT_CAPTURE_BODY=full` stores redacted full request and response bodies. Enable it only when prompt/output retention is acceptable.
- `MODE=openai_passthrough` remains available for the original DS Pro one-shot tool-call stream shim.
