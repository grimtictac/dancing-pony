// Package photo turns an uploaded image into a safe, web-ready JPEG plus
// tiny placeholders, and stores it.
//
// Nothing the client sends is stored as-is. The real type is sniffed from
// the bytes (the Content-Type header is ignored), the dimensions are checked
// before any pixels are decoded (a small file can claim to be enormous: a
// decompression bomb), and the image is re-encoded, which drops EXIF/GPS
// metadata and anything smuggled in alongside the pixels.
package photo

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"

	"go.n16f.net/thumbhash"
	"golang.org/x/image/draw"

	// Go note: a blank import ("_") runs a package only for its side
	// effects. These register their formats with image.Decode and
	// image.DecodeConfig, which pick a decoder from the file's first bytes.
	_ "image/gif"

	_ "golang.org/x/image/webp"
)

// Limits. MaxPixels bounds decoding memory (about 4 bytes per pixel), which
// matters on a 512 MB server; maxConcurrent bounds how many decode at once.
const (
	MaxBytes      = 10 << 20   // upload size, 10 MiB
	MaxPixels     = 16_000_000 // 16 megapixels
	MaxSide       = 1600       // long edge of the stored photo
	maxConcurrent = 2
	jpegQuality   = 82
)

// Errors for the client: the upload was refused, not the server broken.
var (
	ErrUnsupported = errors.New("photo must be a JPEG, PNG, GIF or WebP image")
	ErrTooLarge    = fmt.Errorf("photo must be at most %d megapixels", MaxPixels/1_000_000)
	ErrCorrupt     = errors.New("photo could not be decoded")
)

// Processed is an upload made safe to store and serve.
type Processed struct {
	JPEG          []byte
	Width, Height int
	// ThumbHash is a ~25-byte encoding of a blurred preview (base64), for
	// clients with a ThumbHash decoder.
	ThumbHash string
	// DataURL is a tiny PNG preview that any client can show directly.
	DataURL string
}

var slots = make(chan struct{}, maxConcurrent)

// Process validates and re-encodes an uploaded image.
func Process(ctx context.Context, data []byte) (Processed, error) {
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return Processed{}, ErrUnsupported
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Processed{}, ErrCorrupt
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return Processed{}, ErrTooLarge
	}

	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return Processed{}, ctx.Err()
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Processed{}, ErrCorrupt
	}
	img := fit(src, MaxSide)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Processed{}, fmt.Errorf("encode jpeg: %w", err)
	}
	var tiny bytes.Buffer
	if err := png.Encode(&tiny, fit(img, 16)); err != nil {
		return Processed{}, fmt.Errorf("encode preview: %w", err)
	}
	b := img.Bounds()
	return Processed{
		JPEG:      out.Bytes(),
		Width:     b.Dx(),
		Height:    b.Dy(),
		ThumbHash: base64.StdEncoding.EncodeToString(thumbhash.EncodeImage(fit(img, 100))),
		DataURL:   "data:image/png;base64," + base64.StdEncoding.EncodeToString(tiny.Bytes()),
	}, nil
}

// fit scales src down so its long edge is at most maxSide, keeping its
// aspect ratio, onto an opaque white background (JPEG has no transparency).
func fit(src image.Image, maxSide int) *image.RGBA {
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	if w > maxSide || h > maxSide {
		if w >= h {
			w, h = maxSide, max(1, h*maxSide/w)
		} else {
			w, h = max(1, w*maxSide/h), maxSide
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	if w == sb.Dx() && h == sb.Dy() {
		draw.Draw(dst, dst.Bounds(), src, sb.Min, draw.Over)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, draw.Over, nil)
	}
	return dst
}
