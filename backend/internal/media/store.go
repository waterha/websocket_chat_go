// Package media stores uploaded image and video bytes on the local filesystem.
// The database stays authoritative for ownership and membership; this package
// only resolves opaque storage keys to files inside one configured directory.
package media

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Store struct{ dir string }

func NewStore(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("media directory is empty")
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: absolute}, nil
}

func (store *Store) Dir() string { return store.dir }

// ValidKey reports whether a key is a 32 character lowercase hex identifier.
// Only such keys are accepted, so a key can never escape the media directory.
func ValidKey(key string) bool {
	if len(key) != 32 {
		return false
	}
	for _, digit := range key {
		if (digit < '0' || digit > '9') && (digit < 'a' || digit > 'f') {
			return false
		}
	}
	return true
}

func (store *Store) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", fmt.Errorf("invalid media key")
	}
	return filepath.Join(store.dir, key[:2], key), nil
}

// Create opens a new file for writing. It fails when the key already exists so
// a generated key can never overwrite another upload.
func (store *Store) Create(key string) (*os.File, error) {
	path, err := store.path(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
}

func (store *Store) Open(key string) (*os.File, error) {
	path, err := store.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Remove deletes a stored file and treats an already missing file as success.
func (store *Store) Remove(key string) error {
	path, err := store.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
