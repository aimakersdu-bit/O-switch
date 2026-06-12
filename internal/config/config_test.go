package config

import "testing"

func TestConfigWithDefaultsIncludesAuditDefaults(t *testing.T) {
	cfg := Config{}.WithDefaults()

	if !cfg.AuditEnabled {
		t.Fatalf("expected audit enabled by default")
	}
	if cfg.AuditLogDir != "./logs" {
		t.Fatalf("unexpected audit log dir: %q", cfg.AuditLogDir)
	}
	if cfg.AuditLogPath != "./logs/usage.jsonl" {
		t.Fatalf("unexpected audit log path: %q", cfg.AuditLogPath)
	}
	if cfg.AuditCaptureBody != "preview" {
		t.Fatalf("unexpected audit capture body: %q", cfg.AuditCaptureBody)
	}
	if cfg.AuditPreviewChars != 2000 {
		t.Fatalf("unexpected preview chars: %d", cfg.AuditPreviewChars)
	}
	if cfg.AuditQueueSize != 8192 {
		t.Fatalf("unexpected queue size: %d", cfg.AuditQueueSize)
	}
	if cfg.AuditOverflowPolicy != "drop" {
		t.Fatalf("unexpected overflow policy: %q", cfg.AuditOverflowPolicy)
	}
}

func TestConfigWithDefaultsBuildsAuditPathFromDir(t *testing.T) {
	cfg := Config{AuditLogDir: "/var/log/baixin-switch"}.WithDefaults()

	if cfg.AuditLogPath != "/var/log/baixin-switch/usage.jsonl" {
		t.Fatalf("expected audit path from dir, got %q", cfg.AuditLogPath)
	}
}

func TestConfigWithDefaultsKeepsExplicitAuditPath(t *testing.T) {
	cfg := Config{
		AuditLogDir:  "/var/log/baixin-switch",
		AuditLogPath: "/data/custom/audit.jsonl",
	}.WithDefaults()

	if cfg.AuditLogPath != "/data/custom/audit.jsonl" {
		t.Fatalf("expected explicit audit path to win, got %q", cfg.AuditLogPath)
	}
}
