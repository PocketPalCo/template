package main

import (
	"context"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/core/rtc" // Added import for rtc package
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/PocketPalCo/shopping-service/internal/infra/server"
	"github.com/PocketPalCo/shopping-service/pkg/logger"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx := context.Background()
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	defaultLogger := logger.NewLogger(&cfg)
	slog.SetDefault(defaultLogger)

	conn, err := postgres.Init(&cfg)
	if err != nil {
		slog.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	rtcService := rtc.NewRTCService() // Create RTCService instance

	mainServer := server.New(ctx, &cfg, conn, rtcService) // Pass rtcService to server.New
	go mainServer.Start()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interrupt
	mainServer.Shutdown()
	conn.Close()
}
