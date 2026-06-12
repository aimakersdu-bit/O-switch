package audit

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func ResolveIdentity(r *http.Request, body []byte) Identity {
	requestID := firstNonEmpty(
		headerValue(r, "X-Request-ID"),
	)
	if requestID == "" {
		requestID = generateID("req")
	}

	sessionID := firstNonEmpty(
		headerValue(r, "X-Session-ID"),
		headerValue(r, "X-Conversation-ID"),
		metadataValue(body, "session_id"),
		metadataValue(body, "conversation_id"),
		headerValue(r, "X-Request-ID"),
	)
	if sessionID == "" {
		sessionID = requestID
	}
	return Identity{RequestID: requestID, SessionID: sessionID}
}

func headerValue(r *http.Request, key string) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(key))
}

func metadataValue(body []byte, key string) string {
	var payload struct {
		Metadata map[string]any `json:"metadata"`
	}
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	if payload.Metadata == nil {
		return ""
	}
	if value, ok := payload.Metadata[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func generateID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
