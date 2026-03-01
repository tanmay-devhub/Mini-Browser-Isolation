package agent

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

type InputEvent struct {
	Type      string   `json:"type"`
	X         float64  `json:"x"`
	Y         float64  `json:"y"`
	Button    int      `json:"button"`
	DeltaX    float64  `json:"deltaX"`
	DeltaY    float64  `json:"deltaY"`
	Key       string   `json:"key"`
	Code      string   `json:"code"`
	Modifiers []string `json:"modifiers"`
}

type InputHandler struct {
	browser *Browser
	log     *zap.Logger
}

func NewInputHandler(b *Browser, log *zap.Logger) *InputHandler {
	return &InputHandler{browser: b, log: log}
}

func (h *InputHandler) Handle(msg []byte) error {
	var ev InputEvent
	if err := json.Unmarshal(msg, &ev); err != nil {
		return fmt.Errorf("parse input event: %w", err)
	}

	switch ev.Type {
	case "mousemove":
		return h.browser.InjectMouseEvent("mouseMoved", "none", ev.X, ev.Y)

	case "mousedown":
		return h.browser.InjectMouseEvent("mousePressed", buttonName(ev.Button), ev.X, ev.Y)

	case "mouseup":
		return h.browser.InjectMouseEvent("mouseReleased", buttonName(ev.Button), ev.X, ev.Y)

	case "scroll":
		return h.browser.InjectWheelEvent(ev.X, ev.Y, ev.DeltaX, ev.DeltaY)

	case "keydown":
		// Most frontends send ev.Key like "a", "Enter", "Tab"
		return h.browser.InjectKeyEvent(ev.Key)

	case "keyup":
		return nil

	default:
		h.log.Debug("unknown input event type", zap.String("type", ev.Type))
		return nil
	}
}

func buttonName(b int) string {
	switch b {
	case 0:
		return "left"
	case 1:
		return "middle"
	case 2:
		return "right"
	default:
		return "none"
	}
}