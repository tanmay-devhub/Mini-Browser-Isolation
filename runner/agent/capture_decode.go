package agent

import (
	"bytes"
	"image/png"
)

// decodeAndConvertI420 decodes PNG bytes and returns raw I420.
func decodeAndConvertI420(pngData []byte) []byte {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil
	}
	return rgbaToI420(img)
}
