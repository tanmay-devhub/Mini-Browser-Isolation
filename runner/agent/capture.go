package agent

import (
	"context"
	"image"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"go.uber.org/zap"
)

const (
	captureWidth  = 1280
	captureHeight = 720
	captureFPS    = 30

	// Hard stop: never retry forever.
	maxScreenshotFailures = 20 // ~0.7s at 30fps
	maxWriteFailures      = 20

	// Rate-limit logs during failure storms.
	logEvery = 2 * time.Second
)

type FrameCapture struct {
	browser *Browser
	track   *webrtc.TrackLocalStaticSample
	log     *zap.Logger

	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewFrameCapture(b *Browser, track *webrtc.TrackLocalStaticSample, log *zap.Logger) *FrameCapture {
	return &FrameCapture{
		browser: b,
		track:   track,
		log:     log,
		stopCh:  make(chan struct{}),
	}
}

func (fc *FrameCapture) Start(ctx context.Context) { go fc.loop(ctx) }

func (fc *FrameCapture) Stop() {
	fc.stopOnce.Do(func() { close(fc.stopCh) })
}

func (fc *FrameCapture) loop(ctx context.Context) {
	fc.log.Info("CAPTURE_V3: starting", zap.Int("fps", captureFPS))

	ticker := time.NewTicker(time.Second / captureFPS)
	defer ticker.Stop()

	frameDuration := time.Second / captureFPS

	var (
		ssFailCount   = 0
		wsFailCount   = 0
		lastLog       = time.Time{}
	)

	shouldLog := func() bool {
		if lastLog.IsZero() || time.Since(lastLog) >= logEvery {
			lastLog = time.Now()
			return true
		}
		return false
	}

	for {
		select {
		case <-ctx.Done():
			fc.log.Info("CAPTURE_V3: exiting (ctx done)")
			return
		case <-fc.stopCh:
			fc.log.Info("CAPTURE_V3: exiting (stop requested)")
			return
		case <-ticker.C:
			// Screenshot
			pngBytes, err := fc.browser.Screenshot()
			if err != nil {
				ssFailCount++
				if shouldLog() {
					fc.log.Warn("CAPTURE_V3: screenshot failed",
						zap.Int("consecutive", ssFailCount),
						zap.Error(err),
					)
				}

				// HARD STOP: never infinite loop
				if ssFailCount >= maxScreenshotFailures {
					fc.log.Warn("CAPTURE_V3: exiting (too many screenshot failures)",
						zap.Int("failures", ssFailCount),
					)
					return
				}

				// small backoff so we don’t hammer CPU if ticker is fast
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// success resets screenshot failure counter
			ssFailCount = 0

			i420 := pngToI420(pngBytes)
			if i420 == nil {
				i420 = blankI420(captureWidth, captureHeight)
			}

			// Write sample
			if err := fc.track.WriteSample(media.Sample{
				Data:     i420,
				Duration: frameDuration,
			}); err != nil {
				wsFailCount++
				if shouldLog() {
					fc.log.Warn("CAPTURE_V3: WriteSample failed",
						zap.Int("consecutive", wsFailCount),
						zap.Error(err),
					)
				}

				// HARD STOP: never infinite loop
				if wsFailCount >= maxWriteFailures {
					fc.log.Warn("CAPTURE_V3: exiting (too many WriteSample failures)",
						zap.Int("failures", wsFailCount),
					)
					return
				}

				time.Sleep(100 * time.Millisecond)
				continue
			}

			// success resets write failure counter
			wsFailCount = 0
		}
	}
}

func pngToI420(pngData []byte) []byte {
	return decodeAndConvertI420(pngData)
}

func blankI420(w, h int) []byte {
	frameSize := w * h * 3 / 2
	buf := make([]byte, frameSize)
	ySize := w * h
	for i := ySize; i < frameSize; i++ {
		buf[i] = 128
	}
	return buf
}

// keep if used elsewhere; harmless
func rgbaToI420(img image.Image) []byte {
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y

	yPlane := make([]byte, w*h)
	uPlane := make([]byte, (w/2)*(h/2))
	vPlane := make([]byte, (w/2)*(h/2))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			rf, gf, bf := float64(r>>8), float64(g>>8), float64(b>>8)

			yVal := 0.257*rf + 0.504*gf + 0.098*bf + 16
			yPlane[y*w+x] = clamp(yVal)

			if x%2 == 0 && y%2 == 0 {
				uVal := -0.148*rf - 0.291*gf + 0.439*bf + 128
				vVal := 0.439*rf - 0.368*gf - 0.071*bf + 128
				idx := (y/2)*(w/2) + x/2
				uPlane[idx] = clamp(uVal)
				vPlane[idx] = clamp(vVal)
			}
		}
	}

	out := make([]byte, 0, len(yPlane)+len(uPlane)+len(vPlane))
	out = append(out, yPlane...)
	out = append(out, uPlane...)
	out = append(out, vPlane...)
	return out
}

func clamp(v float64) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}