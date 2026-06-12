package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveIdentityPrefersSessionHeaderAndRequestHeader(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"metadata":{"session_id":"body-session"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Session-ID", "sess-header")
	req.Header.Set("X-Request-ID", "req-header")

	identity := ResolveIdentity(req, []byte(`{"metadata":{"session_id":"body-session"}}`))

	if identity.RequestID != "req-header" {
		t.Fatalf("expected request id from header, got %q", identity.RequestID)
	}
	if identity.SessionID != "sess-header" {
		t.Fatalf("expected session id from header, got %q", identity.SessionID)
	}
}

func TestResolveIdentityFallsBackToMetadataAndGeneratedRequestID(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	identity := ResolveIdentity(req, []byte(`{"metadata":{"conversation_id":"conv-1"}}`))

	if identity.RequestID == "" || !strings.HasPrefix(identity.RequestID, "req_") {
		t.Fatalf("expected generated request id, got %q", identity.RequestID)
	}
	if identity.SessionID != "conv-1" {
		t.Fatalf("expected session id from metadata, got %q", identity.SessionID)
	}
}

func TestCaptureBodyPreviewRedactsAndTruncates(t *testing.T) {
	captured := CaptureBody([]byte(`{"message":"hello world","api_key":"sk-secret","authorization":"Bearer abcdef"}`), Config{
		CaptureBody:  CapturePreview,
		PreviewChars: 12,
	})

	if captured.Body != nil {
		t.Fatalf("expected no full body in preview mode")
	}
	if !captured.Redacted {
		t.Fatalf("expected redacted=true")
	}
	if !captured.Truncated {
		t.Fatalf("expected truncated=true")
	}
	if strings.Contains(captured.Preview, "sk-secret") || strings.Contains(captured.Preview, "abcdef") {
		t.Fatalf("preview leaked secret: %s", captured.Preview)
	}
	if len([]rune(captured.Preview)) > 12 {
		t.Fatalf("expected preview to be truncated, got %q", captured.Preview)
	}
}

func TestCaptureBodyFullStillRedacts(t *testing.T) {
	captured := CaptureBody([]byte(`{"password":"pw","content":"hello"}`), Config{
		CaptureBody:  CaptureFull,
		PreviewChars: 100,
	})

	if captured.Body == nil {
		t.Fatalf("expected full body")
	}
	body := *captured.Body
	if strings.Contains(body, "pw") {
		t.Fatalf("full body leaked password: %s", body)
	}
	if !strings.Contains(body, "[REDACTED]") {
		t.Fatalf("expected redacted body, got %s", body)
	}
}

func TestRecorderWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	rec, err := NewRecorder(Config{
		Enabled:        true,
		LogPath:        path,
		QueueSize:      8,
		OverflowPolicy: OverflowSync,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec.Record(Event{
		SchemaVersion: 1,
		Timestamp:     time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
		Date:          "2026-06-12",
		Week:          "2026-W24",
		RequestID:     "req-1",
		SessionID:     "sess-1",
		Model:         "deepseek-v4-pro",
		InputTokens:   3,
		OutputTokens:  2,
		TotalTokens:   5,
		UsageSource:   UsageSourceUpstream,
	})
	if err := rec.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatalf("expected one jsonl line")
	}
	var event Event
	if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
		t.Fatalf("invalid jsonl: %v", err)
	}
	if event.SessionID != "sess-1" || event.TotalTokens != 5 {
		t.Fatalf("unexpected event: %#v", event)
	}
	if scanner.Scan() {
		t.Fatalf("expected only one line")
	}
}

func TestRecorderDropPolicyCountsDroppedEvents(t *testing.T) {
	rec, err := NewRecorder(Config{
		Enabled:        true,
		LogPath:        filepath.Join(t.TempDir(), "usage.jsonl"),
		QueueSize:      0,
		OverflowPolicy: OverflowDrop,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close(context.Background())

	rec.Record(Event{RequestID: "req-drop"})

	stats := rec.Stats()
	if stats.Dropped == 0 {
		t.Fatalf("expected dropped event, got %#v", stats)
	}
}
