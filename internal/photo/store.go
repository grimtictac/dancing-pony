package photo

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"time"
)

// Store keeps processed photos under keys like "<restaurant>/<photo>.jpg".
// DiskStore is the implementation used locally and on the droplet; an
// S3-compatible one (DigitalOcean Spaces) would satisfy the same interface.
type Store interface {
	Put(ctx context.Context, key string, jpeg []byte) error
	Delete(ctx context.Context, key string) error
	// Open returns the photo for serving.
	Open(key string) (fs.File, error)
}

var keyPattern = regexp.MustCompile(`^[0-9A-Za-z]{22}/[0-9A-Za-z]{22}\.jpg$`)

// ValidKey reports whether key is a well-formed photo key.
func ValidKey(key string) bool { return keyPattern.MatchString(key) }

// ErrInvalidKey is returned for a malformed key.
var ErrInvalidKey = errors.New("invalid photo key")

// DiskStore keeps photos in a directory. All access goes through os.Root,
// which refuses any path that would escape the directory (via "..", or a
// symlink), on top of the key validation.
type DiskStore struct {
	root *os.Root
}

// NewDiskStore opens (creating if needed) the directory dir.
func NewDiskStore(dir string) (*DiskStore, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &DiskStore{root: root}, nil
}

// Put writes the photo atomically: to a temporary file, then renamed, so a
// reader never sees half a photo.
func (d *DiskStore) Put(_ context.Context, key string, jpeg []byte) error {
	if !ValidKey(key) {
		return ErrInvalidKey
	}
	if err := d.root.MkdirAll(key[:22], 0o750); err != nil {
		return err
	}
	tmp := key + ".tmp-" + time.Now().Format("150405.000000000")
	f, err := d.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	if _, err := bytes.NewReader(jpeg).WriteTo(f); err != nil {
		_ = f.Close()
		_ = d.root.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = d.root.Remove(tmp)
		return err
	}
	return d.root.Rename(tmp, key)
}

// Delete removes a photo. A photo that is already gone is not an error.
func (d *DiskStore) Delete(_ context.Context, key string) error {
	if !ValidKey(key) {
		return ErrInvalidKey
	}
	if err := d.root.Remove(key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Open returns the photo for reading.
func (d *DiskStore) Open(key string) (fs.File, error) {
	if !ValidKey(key) {
		return nil, fs.ErrNotExist
	}
	return d.root.Open(key)
}

// Close releases the directory handle.
func (d *DiskStore) Close() error { return d.root.Close() }
