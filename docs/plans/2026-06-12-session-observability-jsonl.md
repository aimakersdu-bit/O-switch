# Session Observability JSONL Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add JSONL-based session observability and token usage reporting to `baixin-switch`.

**Architecture:** Add an `internal/audit` package for request/session identity, redaction, JSONL events, and async recording. Wire it into `internal/proxy` so each model call records one event with token usage and redacted previews. Add lightweight report scripts that aggregate JSONL by day, week, and session.

**Tech Stack:** Go 1.22 standard library, append-only JSONL, Python 3 standard library for reporting.

---

## File Structure

- Create: `internal/audit/types.go`
- Create: `internal/audit/identity.go`
- Create: `internal/audit/redact.go`
- Create: `internal/audit/recorder.go`
- Create: `internal/audit/audit_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/observability/metrics.go`
- Modify: `internal/observability/metrics_test.go`
- Modify: `internal/sse/anthropic_chat.go`
- Modify: `internal/sse/anthropic_chat_test.go`
- Modify: `internal/proxy/server.go`
- Modify: `internal/proxy/server_test.go`
- Create: `scripts/usage_report.py`
- Create: `scripts/usage_report.sh`
- Create: `scripts/usage_report_test.py`
- Update: `README.md`
- Update: `docs/private-deployment-guide.md`
- Update: `docs/user-guide.md`

## Task 1: Audit Core Package

- [x] Write failing tests for session id resolution, redaction, preview/full/off body capture, JSONL writer, and queue overflow.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./internal/audit` and verify it fails because the package does not exist.
- [x] Implement `internal/audit` with event types, body capture, redaction, and async recorder.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./internal/audit` and verify it passes.

## Task 2: Config and Metrics

- [x] Write failing tests for audit config defaults and audit/token metrics rendering.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./internal/config ./internal/observability` and verify expected failure.
- [x] Add audit config fields and metric counters/gauges.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./internal/config ./internal/observability` and verify it passes.

## Task 3: Streaming Usage Capture

- [x] Write failing tests showing Anthropic SSE usage is reported through a callback without breaking OpenAI SSE output.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./internal/sse` and verify expected failure.
- [x] Extend `sse.ChatStreamOptions` with `OnUsage` and `OnTextDelta` callbacks.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./internal/sse` and verify it passes.

## Task 4: Proxy Audit Integration

- [x] Write failing proxy tests for non-stream audit JSONL, stream audit JSONL, failed upstream audit JSONL, and response request/session headers.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./internal/proxy` and verify expected failure.
- [x] Wire audit recorder into `Server`, record events in `anthropic_messages`, and update metrics.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./internal/proxy` and verify it passes.

## Task 5: Reporting Script

- [x] Write failing Python tests for day/week/session aggregation and session timeline output.
- [x] Run `python3 scripts/usage_report_test.py` and verify expected failure.
- [x] Implement `scripts/usage_report.py` and `scripts/usage_report.sh`.
- [x] Run `python3 scripts/usage_report_test.py` and verify it passes.

## Task 6: Docs and Final Verification

- [x] Update README, deployment guide, and user guide with audit config and report commands.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go test ./...`.
- [x] Run `python3 scripts/usage_report_test.py`.
- [x] Run `GOCACHE=$(pwd)/.cache/go-build go build -o /private/tmp/baixin-switch-build ./cmd/baixin-switch`.
