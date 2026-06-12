package audit

import (
	"fmt"
	"time"
)

const (
	CaptureOff     = "off"
	CapturePreview = "preview"
	CaptureFull    = "full"

	OverflowDrop = "drop"
	OverflowSync = "sync"

	UsageSourceUpstream = "upstream"
	UsageSourceMissing  = "missing"
)

type Config struct {
	Enabled        bool
	LogPath        string
	CaptureBody    string
	PreviewChars   int
	QueueSize      int
	OverflowPolicy string
}

func (c Config) WithDefaults() Config {
	if c.LogPath == "" {
		c.LogPath = "./logs/usage.jsonl"
	}
	if c.CaptureBody == "" {
		c.CaptureBody = CapturePreview
	}
	switch c.CaptureBody {
	case CaptureOff, CapturePreview, CaptureFull:
	default:
		c.CaptureBody = CapturePreview
	}
	if c.PreviewChars <= 0 {
		c.PreviewChars = 2000
	}
	if c.QueueSize < 0 {
		c.QueueSize = 0
	}
	if c.QueueSize == 0 && c.OverflowPolicy == "" {
		c.OverflowPolicy = OverflowDrop
	}
	if c.QueueSize == 0 {
		return c
	}
	if c.OverflowPolicy == "" {
		c.OverflowPolicy = OverflowDrop
	}
	switch c.OverflowPolicy {
	case OverflowDrop, OverflowSync:
	default:
		c.OverflowPolicy = OverflowDrop
	}
	return c
}

type Identity struct {
	RequestID string
	SessionID string
}

type CapturedBody struct {
	Preview   string
	Body      *string
	Truncated bool
	Redacted  bool
}

type Event struct {
	SchemaVersion int       `json:"schema_version"`
	Timestamp     time.Time `json:"ts"`
	Date          string    `json:"date"`
	Week          string    `json:"week"`

	RequestID string `json:"request_id"`
	SessionID string `json:"session_id"`

	Mode             string `json:"mode,omitempty"`
	Endpoint         string `json:"endpoint,omitempty"`
	UpstreamEndpoint string `json:"upstream_endpoint,omitempty"`
	Model            string `json:"model,omitempty"`
	RequestedModel   string `json:"requested_model,omitempty"`
	UpstreamModel    string `json:"upstream_model,omitempty"`

	Stream         bool `json:"stream"`
	Status         int  `json:"status"`
	UpstreamStatus int  `json:"upstream_status,omitempty"`
	DurationMs     int  `json:"duration_ms"`
	FirstTokenMs   int  `json:"first_token_ms,omitempty"`

	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	UsageSource  string `json:"usage_source"`

	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	RequestPreview  string  `json:"request_preview,omitempty"`
	ResponsePreview string  `json:"response_preview,omitempty"`
	RequestBody     *string `json:"request_body"`
	ResponseBody    *string `json:"response_body"`
	Truncated       bool    `json:"truncated"`
	Redacted        bool    `json:"redacted"`
}

func NewEventTime(now time.Time) (time.Time, string, string) {
	local := now
	year, week := local.ISOWeek()
	return local, local.Format("2006-01-02"), formatISOWeek(year, week)
}

func formatISOWeek(year, week int) string {
	return fmt.Sprintf("%04d-W%02d", year, week)
}
