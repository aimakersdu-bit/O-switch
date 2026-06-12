package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr            string
	Mode                  string
	UpstreamBaseURL       string
	UpstreamAPIKey        string
	AnthropicVersion      string
	DefaultModel          string
	ModelMap              map[string]string
	ToolCallStreamShim    bool
	ToolCallArgumentChunk int
	MaxConcurrentRequests int
	MaxConcurrentStreams  int
	RequestTimeout        time.Duration
	AuditEnabled          bool
	AuditLogDir           string
	AuditLogPath          string
	AuditCaptureBody      string
	AuditPreviewChars     int
	AuditQueueSize        int
	AuditOverflowPolicy   string
	LogLevel              string
}

func FromEnv() Config {
	loadEnvFile(".env")
	return Config{
		ListenAddr:            envString("LISTEN_ADDR", "127.0.0.1:11435"),
		Mode:                  envString("MODE", "anthropic_messages"),
		UpstreamBaseURL:       strings.TrimRight(envString("UPSTREAM_BASE_URL", "https://api.deepseek.com"), "/"),
		UpstreamAPIKey:        os.Getenv("UPSTREAM_API_KEY"),
		AnthropicVersion:      envString("ANTHROPIC_VERSION", "2023-06-01"),
		DefaultModel:          envString("DEFAULT_MODEL", "deepseek-v4-pro"),
		ModelMap:              envMap("MODEL_MAP"),
		ToolCallStreamShim:    envBool("TOOL_CALL_STREAM_SHIM", true),
		ToolCallArgumentChunk: envInt("TOOL_CALL_ARGUMENT_CHUNK_SIZE", 16),
		MaxConcurrentRequests: envInt("MAX_CONCURRENT_REQUESTS", 1000),
		MaxConcurrentStreams:  envInt("MAX_CONCURRENT_STREAMS", 1000),
		RequestTimeout:        time.Duration(envInt("REQUEST_TIMEOUT_SECONDS", 600)) * time.Second,
		AuditEnabled:          envBool("AUDIT_ENABLED", true),
		AuditLogDir:           envString("AUDIT_LOG_DIR", "./logs"),
		AuditLogPath:          strings.TrimSpace(os.Getenv("AUDIT_LOG_PATH")),
		AuditCaptureBody:      envString("AUDIT_CAPTURE_BODY", "preview"),
		AuditPreviewChars:     envInt("AUDIT_PREVIEW_CHARS", 2000),
		AuditQueueSize:        envInt("AUDIT_QUEUE_SIZE", 8192),
		AuditOverflowPolicy:   envString("AUDIT_OVERFLOW_POLICY", "drop"),
		LogLevel:              envString("LOG_LEVEL", "info"),
	}
}

func loadEnvFile(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}

func (c Config) WithDefaults() Config {
	if c.ListenAddr == "" {
		c.ListenAddr = "127.0.0.1:11435"
	}
	if c.Mode == "" {
		c.Mode = "anthropic_messages"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	c.UpstreamBaseURL = strings.TrimRight(c.UpstreamBaseURL, "/")
	if c.AnthropicVersion == "" {
		c.AnthropicVersion = "2023-06-01"
	}
	if c.DefaultModel == "" {
		c.DefaultModel = "deepseek-v4-pro"
	}
	if c.ModelMap == nil {
		c.ModelMap = map[string]string{}
	}
	if c.ToolCallArgumentChunk <= 0 {
		c.ToolCallArgumentChunk = 16
	}
	if c.MaxConcurrentRequests <= 0 {
		c.MaxConcurrentRequests = 1000
	}
	if c.MaxConcurrentStreams <= 0 {
		c.MaxConcurrentStreams = 1000
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = 600 * time.Second
	}
	if !c.AuditEnabled && c.AuditLogPath == "" {
		c.AuditEnabled = true
	}
	if c.AuditLogDir == "" {
		c.AuditLogDir = "./logs"
	}
	if c.AuditLogPath == "" {
		if c.AuditLogDir == "./logs" {
			c.AuditLogPath = "./logs/usage.jsonl"
		} else {
			c.AuditLogPath = filepath.Join(c.AuditLogDir, "usage.jsonl")
		}
	}
	if c.AuditCaptureBody == "" {
		c.AuditCaptureBody = "preview"
	}
	switch c.AuditCaptureBody {
	case "off", "preview", "full":
	default:
		c.AuditCaptureBody = "preview"
	}
	if c.AuditPreviewChars <= 0 {
		c.AuditPreviewChars = 2000
	}
	if c.AuditQueueSize <= 0 {
		c.AuditQueueSize = 8192
	}
	if c.AuditOverflowPolicy == "" {
		c.AuditOverflowPolicy = "drop"
	}
	switch c.AuditOverflowPolicy {
	case "drop", "sync":
	default:
		c.AuditOverflowPolicy = "drop"
	}
	return c
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envMap(key string) map[string]string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, item := range strings.Split(value, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 {
			continue
		}
		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])
		if from != "" && to != "" {
			out[from] = to
		}
	}
	return out
}
