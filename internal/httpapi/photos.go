package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/id"
	"the-dancing-pony-v2/internal/photo"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/tenant"
)

// putPhoto handles PUT /restaurants/{slug}/dishes/{id}/photo. The body is
// the image itself (any Content-Type; the bytes are sniffed). It replaces
// any earlier photo, so PUT fits.
func (a *api) putPhoto(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, photo.MaxBytes))
	if _, tooBig := errors.AsType[*http.MaxBytesError](err); tooBig {
		a.metrics.photoUpload("rejected")
		writeProblem(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("photo must be at most %d MiB", photo.MaxBytes>>20))
		return
	}
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "could not read the photo")
		return
	}

	p, err := photo.Process(r.Context(), data)
	switch {
	case errors.Is(err, photo.ErrUnsupported):
		a.metrics.photoUpload("rejected")
		writeProblem(w, http.StatusUnsupportedMediaType, err.Error())
		return
	case errors.Is(err, photo.ErrTooLarge), errors.Is(err, photo.ErrCorrupt):
		a.metrics.photoUpload("rejected")
		writeProblem(w, http.StatusUnprocessableEntity, err.Error())
		return
	case err != nil:
		internalError(w, r, err)
		return
	}

	// A fresh key for every upload: URLs never change content, so they can
	// be cached forever, and nobody can guess one.
	key := id.Encode(rest.ID) + "/" + id.Encode(uuid.New()) + ".jpg"
	if err := a.photos.Put(r.Context(), key, p.JPEG); err != nil {
		internalError(w, r, err)
		return
	}
	old, err := a.store.SetDishPhoto(r.Context(), rest.ID, dishID, key, p)
	if err != nil {
		a.removePhoto(r, key)
		dishFound(w, r, err)
		return
	}
	a.removePhoto(r, old)
	a.metrics.photoUpload("stored")
	writeJSON(w, http.StatusOK, dish.Photo{
		URL: photo.URL(key), Width: p.Width, Height: p.Height, ThumbHash: p.ThumbHash, Placeholder: p.DataURL,
	})
}

// deletePhoto handles DELETE /restaurants/{slug}/dishes/{id}/photo.
func (a *api) deletePhoto(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	key, err := a.store.DeleteDishPhoto(r.Context(), rest.ID, dishID)
	if errors.Is(err, store.ErrNotFound) {
		// Also for a dish that doesn't exist, or is another restaurant's.
		writeProblem(w, http.StatusNotFound, "dish has no photo")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	a.removePhoto(r, key)
	w.WriteHeader(http.StatusNoContent)
}

// removePhoto deletes a photo that the database no longer refers to. It's
// best effort: the request has succeeded either way, so a failure is only
// logged (an orphaned file wastes disk; it can't be served by mistake, as
// nothing links to it).
func (a *api) removePhoto(r *http.Request, key string) {
	if key == "" {
		return
	}
	if err := a.photos.Delete(r.Context(), key); err != nil {
		a.logger.WarnContext(r.Context(), "could not delete photo", "key", key, "err", err)
	}
}

// servePhoto handles GET /photos/{restaurant}/{file}.
func (a *api) servePhoto(w http.ResponseWriter, r *http.Request) {
	f, err := a.photos.Open(r.PathValue("restaurant") + "/" + r.PathValue("file"))
	if err != nil {
		writeProblem(w, http.StatusNotFound, "photo not found")
		return
	}
	defer func() { _ = f.Close() }()
	content, ok := f.(io.ReadSeeker)
	if !ok {
		internalError(w, r, errors.New("photo store returned an unseekable file"))
		return
	}
	modified := time.Time{}
	if fi, err := f.Stat(); err == nil {
		modified = fi.ModTime()
	}
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", modified, content)
}
