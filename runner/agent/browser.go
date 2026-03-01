package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	cdpinput "github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"go.uber.org/zap"
)

const (
	screenW = 1280
	screenH = 720
)

// Browser wraps a headless Chromium instance controlled via CDP.
type Browser struct {
	ctx    context.Context
	cancel context.CancelFunc
	log    *zap.Logger
}

// NewBrowser launches headless Chromium and navigates to targetURL.
// The browser context is long-lived; Close() must be called when done.
func NewBrowser(parent context.Context, targetURL string, log *zap.Logger) (*Browser, error) {
	chromiumBin, err := findChromium()
	if err != nil {
		return nil, fmt.Errorf("chromium not found: %w", err)
	}
	log.Info("found chromium", zap.String("bin", chromiumBin))

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromiumBin),
		chromedp.WindowSize(screenW, screenH),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("remote-debugging-port", "9222"),
		chromedp.Flag("disable-features", "UseDBus"),
		chromedp.Flag("use-fake-ui-for-media-stream", true),
		chromedp.Flag("use-fake-device-for-media-stream", true),
		chromedp.Flag("autoplay-policy", "no-user-gesture-required"),
	)

	if raw := strings.TrimSpace(os.Getenv("CHROME_FLAGS")); raw != "" {
		extra := parseChromeFlags(raw)
		for k, v := range extra {
			if v == "" {
				opts = append(opts, chromedp.Flag(k, true))
			} else {
				opts = append(opts, chromedp.Flag(k, v))
			}
		}
		log.Info("applied CHROME_FLAGS", zap.String("raw", raw), zap.Int("count", len(extra)))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(parent, opts...)

	// tabCtx must remain alive for the session lifetime. We run the initial
	// navigation using tabCtx directly (no child timeout context) and impose a
	// wall-clock deadline via a goroutine+channel so that cancelling the timeout
	// does not cancel tabCtx itself.
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)

	type navResult struct{ err error }
	navCh := make(chan navResult, 1)
	go func() {
		navCh <- navResult{err: chromedp.Run(tabCtx, chromedp.Navigate(targetURL))}
	}()

	select {
	case res := <-navCh:
		if res.err != nil {
			tabCancel()
			allocCancel()
			return nil, fmt.Errorf("chromium navigation failed: %w", res.err)
		}
	case <-time.After(30 * time.Second):
		tabCancel()
		allocCancel()
		return nil, fmt.Errorf("chromium navigation timed out after 30s")
	}

	cancel := func() {
		tabCancel()
		allocCancel()
	}

	return &Browser{ctx: tabCtx, cancel: cancel, log: log}, nil
}

func (b *Browser) Close() { b.cancel() }

func (b *Browser) Navigate(url string) error {
	return chromedp.Run(b.ctx, chromedp.Navigate(url))
}

// Screenshot captures a full-page PNG at 80% quality.
func (b *Browser) Screenshot() ([]byte, error) {
	var buf []byte
	if err := chromedp.Run(b.ctx, chromedp.FullScreenshot(&buf, 80)); err != nil {
		return nil, err
	}
	return buf, nil
}

func (b *Browser) InjectMouseEvent(eventType, button string, x, y float64) error {
	btn := cdpMouseButton(button)
	act := chromedp.ActionFunc(func(ctx context.Context) error {
		return cdpinput.DispatchMouseEvent(cdpinput.MouseType(eventType), x, y).
			WithButton(btn).
			Do(ctx)
	})
	return chromedp.Run(b.ctx, act)
}

func (b *Browser) InjectWheelEvent(x, y, deltaX, deltaY float64) error {
	act := chromedp.ActionFunc(func(ctx context.Context) error {
		return cdpinput.DispatchMouseEvent(cdpinput.MouseType("mouseWheel"), x, y).
			WithDeltaX(deltaX).
			WithDeltaY(deltaY).
			Do(ctx)
	})
	return chromedp.Run(b.ctx, act)
}

func (b *Browser) InjectKeyEvent(key string) error {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	return chromedp.Run(b.ctx, chromedp.KeyEvent(key))
}

func cdpMouseButton(btn string) cdpinput.MouseButton {
	switch strings.ToLower(btn) {
	case "right":
		return cdpinput.MouseButton("right")
	case "middle":
		return cdpinput.MouseButton("middle")
	default:
		return cdpinput.MouseButton("left")
	}
}

func findChromium() (string, error) {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil && p != "" {
			return p, nil
		}
	}
	return "", fmt.Errorf("no chromium binary found in PATH")
}

// parseChromeFlags converts a space-separated flag string into a map of flag
// name to value. Boolean flags (e.g. --no-sandbox) map to an empty string.
func parseChromeFlags(raw string) map[string]string {
	out := map[string]string{}
	parts := strings.Fields(raw)
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if !strings.HasPrefix(p, "--") {
			continue
		}
		p = strings.TrimPrefix(p, "--")
		if strings.Contains(p, "=") {
			kv := strings.SplitN(p, "=", 2)
			if k := strings.TrimSpace(kv[0]); k != "" {
				out[k] = strings.TrimSpace(kv[1])
			}
			continue
		}
		k := strings.TrimSpace(p)
		if k == "" {
			continue
		}
		if i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "--") {
			out[k] = strings.TrimSpace(parts[i+1])
			i++
			continue
		}
		out[k] = ""
	}
	_ = strconv.ErrSyntax // keep strconv import used by callers
	return out
}
