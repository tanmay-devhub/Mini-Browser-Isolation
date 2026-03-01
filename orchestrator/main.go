package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mini-browser-isolation/orchestrator/api"
	"github.com/mini-browser-isolation/orchestrator/config"
	"github.com/mini-browser-isolation/orchestrator/runner"
	"github.com/mini-browser-isolation/orchestrator/session"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	cfg := config.Load()

	log := buildLogger(cfg.LogLevel)
	defer log.Sync() //nolint:errcheck

	log.Info("starting orchestrator",
		zap.String("port", cfg.Port),
		zap.Int("maxSessions", cfg.MaxSessions),
		zap.Duration("sessionTimeout", cfg.SessionTimeout))

	// Session manager with reaper.
	mgr := session.NewManager(cfg.MaxSessions, cfg.SessionTimeout, log)
	defer mgr.Stop()

	// Docker runner (fails fast if Docker daemon is unreachable).
	docker, err := runner.NewDockerRunner(cfg.RunnerImage, cfg.RunnerNetwork, log)
	if err != nil {
		log.Fatal("failed to connect to Docker", zap.Error(err))
	}

	// HTTP handlers.
	sessionHandler := api.NewSessionHandler(mgr, docker, log)
	signalingHandler := api.NewSignalingHandler(mgr, cfg, log)

	router := api.NewRouter(sessionHandler, signalingHandler)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	<-quit
	log.Info("shutting down gracefully")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("server shutdown error", zap.Error(err))
	}
}

func buildLogger(level string) *zap.Logger {
	lvl := zapcore.InfoLevel
	_ = lvl.UnmarshalText([]byte(level))

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(lvl)
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	log, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return log
}
