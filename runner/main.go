package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mini-browser-isolation/runner/agent"
	runnerwebrtc "github.com/mini-browser-isolation/runner/webrtc"
	pionwebrtc "github.com/pion/webrtc/v3"
	"go.uber.org/zap"
)

func main() {
	log := buildLogger()
	defer log.Sync() //nolint:errcheck

	sessionID := getEnv("SESSION_ID", "unknown")
	targetURL := getEnv("TARGET_URL", "https://example.com")
	port := getEnv("RUNNER_PORT", "8080")
	fallbackFPS := getEnvInt("FALLBACK_FPS", 5)

	log.Info("runner starting",
		zap.String("sessionId", sessionID),
		zap.String("url", targetURL),
		zap.String("port", port))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Launch Chromium.
	browser, err := agent.NewBrowser(ctx, targetURL, log)
	if err != nil {
		log.Fatal("browser init failed", zap.Error(err))
	}
	defer browser.Close()

	// Build Pion peer connection (ICE servers from env).
	iceServers := buildICEServers()
	pc, err := runnerwebrtc.NewPeerConnection(iceServers, log)
	if err != nil {
		log.Fatal("peer connection init failed", zap.Error(err))
	}
	defer pc.Close()

	// Input handler dispatches DataChannel/WS events to Chromium.
	inputHandler := agent.NewInputHandler(browser, log)

	// Frame capture – pushes screenshots to the VP8 video track.
	capture := agent.NewFrameCapture(browser, pc.VideoTrack, log)
	capture.Start(ctx)
	defer capture.Stop()

	// Dispatch input events from the WebRTC data channel to the browser.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-pc.InputCh:
				if err := inputHandler.Handle(msg); err != nil {
					log.Warn("input dispatch error", zap.Error(err))
				}
			}
		}
	}()

	// Fallback WS streamer (low-FPS JPEG).
	fallback := agent.NewFallbackStreamer(browser, fallbackFPS, log)
	defer fallback.Stop()

	// HTTP server: healthz + /offer (signaling) + /ws (fallback).
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	// SDP offer endpoint: receives offer from orchestrator, returns answer.
	mux.HandleFunc("/offer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}

		answer, err := pc.HandleOffer(body)
		if err != nil {
			log.Error("offer handling failed", zap.Error(err))
			http.Error(w, "offer failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(answer) //nolint:errcheck
	})

	// WebSocket: fallback frame streaming + input proxying.
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		fallback.ServeWS(w, r, inputHandler)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // streaming; no write timeout
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("runner HTTP listening", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("runner server error", zap.Error(err))
		}
	}()

	<-quit
	log.Info("runner shutting down")
	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx) //nolint:errcheck
}

func buildICEServers() []pionwebrtc.ICEServer {
	stun := getEnv("STUN_HOST", "stun:stun.l.google.com:19302")
	servers := []pionwebrtc.ICEServer{
		{URLs: []string{stun}},
	}

	if getEnvBool("TURN_ENABLED", false) {
		servers = append(servers, pionwebrtc.ICEServer{
			URLs:           []string{fmt.Sprintf("turn:%s:%s", getEnv("TURN_HOST", "coturn"), getEnv("TURN_PORT", "3478"))},
			Username:       getEnv("TURN_USERNAME", "user"),
			Credential:     getEnv("TURN_CREDENTIAL", "password"),
			CredentialType: pionwebrtc.ICECredentialTypePassword,
		})
	}
	return servers
}

func buildLogger() *zap.Logger {
	log, _ := zap.NewProduction()
	return log
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1"
	}
	return fallback
}

