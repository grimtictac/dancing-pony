package httpapi

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"the-dancing-pony-v2/internal/dish"
)

// testJPEG is a small, valid photo: 320x240, so it's stored at its own size.
var testJPEG = func() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := range 240 {
		for x := range 320 {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

func TestDishPhotoLifecycle(t *testing.T) {
	h := newAPI(t)
	cake := dishNamed(t, h, "orc-shack", "Seed Cake")
	path := shackDishes + "/" + cake.ID

	if cake.Photo != nil {
		t.Fatalf("seeded dish already has a photo: %+v", cake.Photo)
	}

	// Upload, as the owner.
	rec := do(t, h, http.MethodPut, path+"/photo", string(testJPEG))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: status %d, body %s", rec.Code, rec.Body)
	}
	first := decode[dish.Photo](t, rec)
	if first.Width != 320 || first.Height != 240 || first.ThumbHash == "" || !strings.HasPrefix(first.Placeholder, "data:image/png;base64,") {
		t.Errorf("photo = %+v", first)
	}
	if !strings.HasPrefix(first.URL, "/photos/") {
		t.Errorf("url = %q, want it under /photos/", first.URL)
	}

	// The dish now shows it, and it's served without logging in.
	if got := decode[dish.Dish](t, do(t, h, http.MethodGet, path, "")); got.Photo == nil || *got.Photo != first {
		t.Errorf("dish photo = %+v, want %+v", got.Photo, first)
	}
	rec = do(t, h.raw, http.MethodGet, first.URL, "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("GET photo: status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable (keys never change content)", cc)
	}

	// Replacing it deletes the old file.
	second := decode[dish.Photo](t, do(t, h, http.MethodPut, path+"/photo", string(testJPEG)))
	if second.URL == first.URL {
		t.Error("a new upload reused the old URL")
	}
	if rec := do(t, h.raw, http.MethodGet, first.URL, ""); rec.Code != http.StatusNotFound {
		t.Errorf("old photo after replacement: status %d, want 404", rec.Code)
	}

	// Removing it.
	if rec := do(t, h, http.MethodDelete, path+"/photo", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete photo: status %d", rec.Code)
	}
	if got := decode[dish.Dish](t, do(t, h, http.MethodGet, path, "")); got.Photo != nil {
		t.Errorf("dish photo after delete = %+v, want null", got.Photo)
	}
	if rec := do(t, h.raw, http.MethodGet, second.URL, ""); rec.Code != http.StatusNotFound {
		t.Errorf("photo file after delete: status %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, path+"/photo", ""); rec.Code != http.StatusNotFound {
		t.Errorf("deleting again: status %d, want 404", rec.Code)
	}

	// Deleting the dish deletes its photo file too.
	third := decode[dish.Photo](t, do(t, h, http.MethodPut, path+"/photo", string(testJPEG)))
	if rec := do(t, h, http.MethodDelete, path, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete dish: status %d", rec.Code)
	}
	if rec := do(t, h.raw, http.MethodGet, third.URL, ""); rec.Code != http.StatusNotFound {
		t.Errorf("photo of a deleted dish: status %d, want 404", rec.Code)
	}
}

func TestDishPhotoRejected(t *testing.T) {
	h := newAPI(t)
	path := shackDishes + "/" + dishNamed(t, h, "orc-shack", "Seed Cake").ID + "/photo"

	tests := []struct {
		name, user, body string
		want             int
	}{
		{"not an image", "orc-owner", "<svg onload=alert(1)>", http.StatusUnsupportedMediaType},
		{"corrupt", "orc-owner", string(testJPEG[:len(testJPEG)/3]), http.StatusUnprocessableEntity},
		{"customer", "frodo", string(testJPEG), http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := doAs(t, h, tt.user, http.MethodPut, path, tt.body); rec.Code != tt.want {
				t.Errorf("status %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
		})
	}

	t.Run("too big", func(t *testing.T) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, path, bytes.NewReader(make([]byte, 11<<20)))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status %d, want 413", rec.Code)
		}
	})

	for _, p := range []string{"/photos/x/y.jpg", "/photos/0000000000000000000001/..%2f..%2fetc%2fpasswd"} {
		if rec := do(t, h.raw, http.MethodGet, p, ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", p, rec.Code)
		}
	}
}
