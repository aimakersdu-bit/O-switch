package audit

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

var bearerPattern = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)

func CaptureBody(raw []byte, cfg Config) CapturedBody {
	cfg = cfg.WithDefaults()
	if cfg.CaptureBody == CaptureOff || len(raw) == 0 {
		return CapturedBody{}
	}

	text, redacted := Redact(raw)
	preview, truncated := truncateRunes(text, cfg.PreviewChars)
	out := CapturedBody{
		Preview:   preview,
		Truncated: truncated,
		Redacted:  redacted,
	}
	if cfg.CaptureBody == CaptureFull {
		body := text
		out.Body = &body
	}
	return out
}

func Redact(raw []byte) (string, bool) {
	var value any
	if err := json.Unmarshal(raw, &value); err == nil {
		redacted := redactJSON(value)
		out, err := json.Marshal(redacted.value)
		if err == nil {
			text := bearerPattern.ReplaceAllString(string(out), "Bearer [REDACTED]")
			return text, redacted.changed || text != string(out)
		}
	}
	text := bearerPattern.ReplaceAllString(string(raw), "Bearer [REDACTED]")
	return text, text != string(raw)
}

type redactResult struct {
	value   any
	changed bool
}

func redactJSON(value any) redactResult {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		changed := false
		for key, item := range v {
			if isSensitiveKey(key) {
				out[key] = "[REDACTED]"
				changed = true
				continue
			}
			next := redactJSON(item)
			out[key] = next.value
			changed = changed || next.changed
		}
		return redactResult{value: out, changed: changed}
	case []any:
		out := make([]any, len(v))
		changed := false
		for i, item := range v {
			next := redactJSON(item)
			out[i] = next.value
			changed = changed || next.changed
		}
		return redactResult{value: out, changed: changed}
	case string:
		text := bearerPattern.ReplaceAllString(v, "Bearer [REDACTED]")
		return redactResult{value: text, changed: text != v}
	default:
		return redactResult{value: value}
	}
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range []string{"api_key", "authorization", "token", "secret", "password", "cookie"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

func truncateRunes(s string, limit int) (string, bool) {
	if limit <= 0 {
		return "", s != ""
	}
	if utf8.RuneCountInString(s) <= limit {
		return s, false
	}
	runes := []rune(s)
	return string(runes[:limit]), true
}
