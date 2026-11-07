package photo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"testing"
)

func encodePNG(t *testing.T, w, h int, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 120, A: alpha})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProcess(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		width, height int
	}{
		{"large PNG with transparency is scaled down", encodePNG(t, 3200, 2000, 128), 1600, 1000},
		{"portrait", encodePNG(t, 900, 1800, 255), 800, 1600},
		{"small JPEG keeps its size", encodeJPEG(t, 200, 100), 200, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Process(t.Context(), tt.data)
			if err != nil {
				t.Fatal(err)
			}
			if p.Width != tt.width || p.Height != tt.height {
				t.Errorf("size = %dx%d, want %dx%d", p.Width, p.Height, tt.width, tt.height)
			}
			cfg, format, err := image.DecodeConfig(bytes.NewReader(p.JPEG))
			if err != nil || format != "jpeg" || cfg.Width != tt.width {
				t.Errorf("stored photo: format %q, width %d, err %v; want a %d-wide jpeg", format, cfg.Width, err, tt.width)
			}
			if th, err := base64.StdEncoding.DecodeString(p.ThumbHash); err != nil || len(th) < 5 || len(th) > 40 {
				t.Errorf("ThumbHash = %q (%d bytes, err %v), want 5-40 bytes of base64", p.ThumbHash, len(th), err)
			}
			if !strings.HasPrefix(p.DataURL, "data:image/png;base64,") || len(p.DataURL) > 2048 {
				t.Errorf("DataURL is %d bytes, prefix %.30q; want a small PNG data URL", len(p.DataURL), p.DataURL)
			}
		})
	}
}

func TestProcessStripsMetadata(t *testing.T) {
	// Insert an EXIF segment (APP1) with a fake GPS position after the
	// JPEG's start-of-image marker.
	orig := encodeJPEG(t, 64, 64)
	exif := append([]byte("Exif\x00\x00"), []byte("GPS 51.5072N 0.1276W secret-location")...)
	seg := append([]byte{0xFF, 0xE1, 0, 0}, exif...)
	binary.BigEndian.PutUint16(seg[2:], uint16(len(exif)+2)) //nolint:gosec // G115: a short fixed test segment
	withExif := append(append(append([]byte{}, orig[:2]...), seg...), orig[2:]...)
	if !bytes.Contains(withExif, []byte("secret-location")) {
		t.Fatal("test setup: EXIF not inserted")
	}

	p, err := Process(t.Context(), withExif)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(p.JPEG, []byte("Exif")) || bytes.Contains(p.JPEG, []byte("secret-location")) {
		t.Error("stored photo still carries the EXIF metadata")
	}
}

// pngHeader returns just a PNG signature and IHDR chunk claiming the given
// size: a few dozen bytes that would decode to gigabytes.
func pngHeader(w, h uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	chunk := append([]byte("IHDR"), ihdr...)
	out := []byte("\x89PNG\r\n\x1a\n")
	out = binary.BigEndian.AppendUint32(out, 13) // IHDR data length
	out = append(out, chunk...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(chunk))
}

func TestProcessRejects(t *testing.T) {
	jpg := encodeJPEG(t, 50, 50)
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"decompression bomb", pngHeader(50_000, 50_000), ErrTooLarge},
		{"just over the pixel limit", pngHeader(4001, 4000), ErrTooLarge},
		{"text", []byte("definitely not an image, just some text"), ErrUnsupported},
		{"HTML posing as an image", []byte("<html><script>alert(1)</script></html>"), ErrUnsupported},
		{"empty", nil, ErrUnsupported},
		{"truncated JPEG", jpg[:len(jpg)/2], ErrCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Process(t.Context(), tt.data); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestProcessHonoursCancellation(t *testing.T) {
	for range maxConcurrent {
		slots <- struct{}{}
	}
	defer func() {
		for range maxConcurrent {
			<-slots
		}
	}()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Process(ctx, encodeJPEG(t, 10, 10)); !errors.Is(err, context.Canceled) {
		t.Errorf("all slots busy, context cancelled: err = %v, want context.Canceled", err)
	}
}

func TestDiskStore(t *testing.T) {
	ctx := t.Context()
	d, err := NewDiskStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	key := "0000000000000000000001/0000000000000000000002.jpg"

	if err := d.Put(ctx, key, []byte("jpeg bytes")); err != nil {
		t.Fatal(err)
	}
	f, err := d.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	_ = f.Close()
	if string(got) != "jpeg bytes" {
		t.Errorf("read back %q", got)
	}
	if err := d.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, key); err != nil {
		t.Errorf("deleting a missing photo: %v, want nil", err)
	}
	if _, err := d.Open(key); err == nil {
		t.Error("photo still there after Delete")
	}

	for _, bad := range []string{"../etc/passwd", "0000000000000000000001/../../x.jpg", "/abs.jpg", "a/b.jpg", ""} {
		if err := d.Put(ctx, bad, nil); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("Put(%q) = %v, want ErrInvalidKey", bad, err)
		}
		if _, err := d.Open(bad); err == nil {
			t.Errorf("Open(%q) succeeded", bad)
		}
	}
}
