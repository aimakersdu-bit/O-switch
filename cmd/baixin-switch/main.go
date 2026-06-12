package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

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

	// 设置默认日志输出为 JSON 格式
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.FromEnv()
	server := proxy.NewServer(cfg)

	log.Printf("baixin-switch version: %s, listening on %s", Version, cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, server); err != nil {
		log.Fatal(err)
	}
}
