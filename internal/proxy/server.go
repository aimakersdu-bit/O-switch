package proxy

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"baixin-switch/internal/config"
	"baixin-switch/internal/convert"
	"baixin-switch/internal/limits"
	"baixin-switch/internal/observability"
	"baixin-switch/internal/sse"
)

type Server struct {
	cfg            config.Config
	client         *http.Client
	mux            *http.ServeMux
	metrics        *observability.Metrics
	requestLimiter *limits.Limiter
	streamLimiter  *limits.Limiter
	logger         *slog.Logger
}

func NewServer(cfg config.Config) *Server {
	cfg = cfg.WithDefaults()
	return NewServerWithClient(cfg, newUpstreamClient(cfg))
}

func NewServerWithClient(cfg config.Config, client *http.Client) *Server {
	cfg = cfg.WithDefaults()
	if client == nil {
		client = &http.Client{Timeout: cfg.RequestTimeout}
	}
	s := &Server{
		cfg:            cfg,
		client:         client,
		mux:            http.NewServeMux(),
		metrics:        observability.NewMetrics(),
		requestLimiter: limits.NewLimiter(cfg.MaxConcurrentRequests),
		streamLimiter:  limits.NewLimiter(cfg.MaxConcurrentStreams),
		logger:         slog.Default(),
	}
	s.routes()
	return s
}

func newUpstreamClient(cfg config.Config) *http.Client {
	poolSize := cfg.MaxConcurrentRequests
	if cfg.MaxConcurrentStreams > poolSize {
		poolSize = cfg.MaxConcurrentStreams
	}
	if poolSize < 100 {
		poolSize = 100
	}

	return &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          poolSize,
			MaxIdleConnsPerHost:   poolSize,
			MaxConnsPerHost:       poolSize,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "baixin-switch",
		"status":  "ok",
	})
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.UpstreamBaseURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"service": "baixin-switch",
			"status":  "not_ready",
			"reason":  "upstream_base_url_missing",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "baixin-switch",
		"status":  "ready",
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, s.metrics.Render())
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := http.StatusOK
	defer func() {
		s.metrics.IncHTTPRequests(r.Method, r.URL.Path, status)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"mode", s.cfg.Mode,
		)
	}()

	if !s.requestLimiter.TryAcquire() {
		status = http.StatusTooManyRequests
		writeOpenAIError(w, status, "rate_limit_exceeded", "too many active requests")
		return
	}
	defer func() {
		s.requestLimiter.Release()
		s.metrics.SetActiveRequests(int64(s.requestLimiter.InUse()))
	}()
	s.metrics.SetActiveRequests(int64(s.requestLimiter.InUse()))

	if s.cfg.UpstreamBaseURL == "" {
		status = http.StatusBadGateway
		writeOpenAIError(w, status, "upstream_base_url_missing", "UPSTREAM_BASE_URL is required")
		return
	}

	if s.cfg.Mode == "openai_passthrough" {
		status = s.handleOpenAIPassthrough(w, r)
		return
	}

	status = s.handleAnthropicMessages(w, r)
}

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) int {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return http.StatusBadRequest
	}

	anthropicReq, err := convert.OpenAIChatToAnthropic(body, convert.Options{
		DefaultModel: s.cfg.DefaultModel,
		ModelMap:     s.cfg.ModelMap,
	})
	if err != nil {
		s.metrics.IncConversionErrors("openai_chat_to_anthropic", "invalid_request")
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return http.StatusBadRequest
	}
	upstreamBody, err := json.Marshal(anthropicReq)
	if err != nil {
		s.metrics.IncConversionErrors("openai_chat_to_anthropic", "marshal")
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return http.StatusBadRequest
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.cfg.UpstreamBaseURL+"/v1/messages", bytes.NewReader(upstreamBody))
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upstream_request_error", err.Error())
		return http.StatusBadGateway
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", acceptForAnthropic(anthropicReq.Stream))
	if s.cfg.UpstreamAPIKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+s.cfg.UpstreamAPIKey)
		upstreamReq.Header.Set("x-api-key", s.cfg.UpstreamAPIKey)
	}
	if s.cfg.AnthropicVersion != "" {
		upstreamReq.Header.Set("anthropic-version", s.cfg.AnthropicVersion)
	}

	resp, err := s.client.Do(upstreamReq)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upstream_connection_error", err.Error())
		return http.StatusBadGateway
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return resp.StatusCode
	}

	if isEventStream(resp.Header.Get("Content-Type")) {
		if !s.streamLimiter.TryAcquire() {
			writeOpenAIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many active streams")
			return http.StatusTooManyRequests
		}
		defer func() {
			s.streamLimiter.Release()
			s.metrics.SetActiveStreams(int64(s.streamLimiter.InUse()))
		}()
		s.metrics.SetActiveStreams(int64(s.streamLimiter.InUse()))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if err := sse.WriteAnthropicToOpenAIChatStream(w, resp.Body, sse.ChatStreamOptions{
			Model: anthropicReq.Model,
		}); err != nil {
			s.metrics.IncConversionErrors("anthropic_stream_to_openai_chat", "parse")
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", jsonString(map[string]string{"message": err.Error()}))
		}
		return http.StatusOK
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upstream_read_error", err.Error())
		return http.StatusBadGateway
	}
	converted, err := convert.AnthropicToOpenAIChat(raw, "")
	if err != nil {
		s.metrics.IncConversionErrors("anthropic_to_openai_chat", "parse")
		writeOpenAIError(w, http.StatusBadGateway, "upstream_response_error", err.Error())
		return http.StatusBadGateway
	}
	writeJSON(w, http.StatusOK, converted)
	return http.StatusOK
}

func (s *Server) handleOpenAIPassthrough(w http.ResponseWriter, r *http.Request) int {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return http.StatusBadRequest
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.cfg.UpstreamBaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upstream_request_error", err.Error())
		return http.StatusBadGateway
	}
	copyForwardHeaders(upstreamReq.Header, r.Header)
	upstreamReq.Header.Set("Content-Type", contentTypeOrJSON(r.Header.Get("Content-Type")))
	if s.cfg.UpstreamAPIKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+s.cfg.UpstreamAPIKey)
	}

	resp, err := s.client.Do(upstreamReq)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "upstream_connection_error", err.Error())
		return http.StatusBadGateway
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(w, resp.Body)
		return resp.StatusCode
	}

	if isEventStream(resp.Header.Get("Content-Type")) {
		normalized, err := sse.NormalizeChatCompletionStream(resp.Body, sse.Options{
			ToolCallStreamShim: s.cfg.ToolCallStreamShim,
			ArgumentChunkSize:  s.cfg.ToolCallArgumentChunk,
		})
		if err != nil {
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", jsonString(map[string]string{"message": err.Error()}))
			return http.StatusBadGateway
		}
		_, _ = io.WriteString(w, normalized)
		return resp.StatusCode
	}

	_, _ = io.Copy(w, resp.Body)
	return resp.StatusCode
}

func copyForwardHeaders(dst, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Host") || strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOpenAIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"type":    code,
			"code":    code,
			"message": message,
		},
	})
}

func contentTypeOrJSON(contentType string) string {
	if strings.TrimSpace(contentType) == "" {
		return "application/json"
	}
	return contentType
}

func isEventStream(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

func acceptForAnthropic(stream *bool) string {
	if stream != nil && *stream {
		return "text/event-stream"
	}
	return "application/json"
}

func jsonString(payload any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return `{"message":"unknown error"}`
	}
	return string(raw)
}
