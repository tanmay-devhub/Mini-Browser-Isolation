package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// FallbackStreamer provides a low-FPS PNG stream over WebSocket,
// plus a simple input proxy back into the same InputHandler.
type FallbackStreamer struct {
	browser *Browser
	fps     int
	log     *zap.Logger

	upgrader websocket.Upgrader

	stopCh   chan struct{}
	stopOnce sync.Once
}

// frameMsg matches the JSON envelope the frontend expects:
//
//	{ "type": "frame", "data": "<base64 PNG>" }
type frameMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

func NewFallbackStreamer(b *Browser, fps int, log *zap.Logger) *FallbackStreamer {
	if fps <= 0 {
		fps = 5
	}
	return &FallbackStreamer{
		browser: b,
		fps:     fps,
		log:     log,
		stopCh:  make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (fs *FallbackStreamer) Stop() {
	fs.stopOnce.Do(func() { close(fs.stopCh) })
}

// ServeWS upgrades to WebSocket and streams frames until the client disconnects,
// Stop() is called, or the browser fails permanently.
func (fs *FallbackStreamer) ServeWS(w http.ResponseWriter, r *http.Request, input *InputHandler) {
	conn, err := fs.upgrader.Upgrade(w, r, nil)
	if err != nil {
		fs.log.Warn("ws upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	// Keepalive: server sends ping every 15 s.
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		return nil
	})

	// Read loop for input events from client.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := input.Handle(msg); err != nil {
				fs.log.Debug("ws input handle error", zap.Error(err))
			}
		}
	}()

	interval := time.Second / time.Duration(fs.fps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	pingTick := time.NewTicker(15 * time.Second)
	defer pingTick.Stop()

	// Allow up to 10 s of consecutive errors before giving up.
	// This handles the race where the browser is still warming up when the WS connects.
	const maxConsecFail = 50 // 50 × (1/fps) seconds
	consecFail := 0
	var lastWarn time.Time

	fs.log.Info("fallback WS stream started", zap.Int("fps", fs.fps))

	for {
		select {
		case <-fs.stopCh:
			return
		case <-readDone:
			return
		case <-pingTick.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-ticker.C:
			pngBytes, err := fs.browser.Screenshot()
			if err != nil {
				consecFail++

				// Log at most once every 2 s to avoid spam.
				if time.Since(lastWarn) > 2*time.Second {
					fs.log.Warn("fallback screenshot error",
						zap.Int("consecFail", consecFail),
						zap.Error(err))
					lastWarn = time.Now()
				}

				// Hard stop only on genuine context cancellation after many retries,
				// not on the first error (browser may still be starting).
				if isContextCanceled(err) && consecFail >= maxConsecFail {
					fs.log.Info("fallback: browser context permanently canceled, exiting")
					return
				}
				continue
			}

			// Success – reset failure counter.
			consecFail = 0

			// Encode as JSON envelope matching frontend expectation.
			msg, err := json.Marshal(frameMsg{
				Type: "frame",
				Data: base64.StdEncoding.EncodeToString(pngBytes),
			})
			if err != nil {
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				fs.log.Info("fallback client disconnected", zap.Error(err))
				return
			}
		}
	}
}

func isContextCanceled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "deadline exceeded")
}
