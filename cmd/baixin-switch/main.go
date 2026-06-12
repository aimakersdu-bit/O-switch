package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"baixin-switch/internal/config"
	"baixin-switch/internal/proxy"
)

var Version = "dev"

func main() {
	versionFlag := flag.Bool("v", false, "print version and exit")
	flag.BoolVar(versionFlag, "version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("baixin-switch version: %s\n", Version)
		os.Exit(0)
	}

	cfg := config.FromEnv()

	// Parse log level
	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// 设置默认日志输出为 JSON 格式
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	server := proxy.NewServer(cfg)

	log.Printf("baixin-switch version: %s, listening on %s", Version, cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, server); err != nil {
		log.Fatal(err)
	}
}
