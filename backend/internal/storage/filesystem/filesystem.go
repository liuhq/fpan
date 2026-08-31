// Package filesystem implements local content-addressed blob storage.
package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/liuhq/fpan/internal/storage"
)

const directoryMode = 0o700

type Store struct {
	root string
}

var _ storage.Store = (*Store)(nil)
var _ storage.BlobEnumerator = (*Store)(nil)
var _ storage.TemporaryBlobCleaner = (*Store)(nil)

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, directoryMode); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat storage root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("storage root %q is not a directory", root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	return &Store{root: abs}, nil
}

func (s *Store) Ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(s.root)
	if err != nil {
		return fmt.Errorf("stat storage root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("storage root %q is not a directory", s.root)
	}
	return nil
}

func (s *Store) Put(ctx context.Context, reader io.Reader) (result storage.PutResult, err error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	tmp, err := os.CreateTemp(s.root, ".fpan-blob-*")
	if err != nil {
		return result, fmt.Errorf("create temporary blob: %w", err)
	}
	tmpName := tmp.Name()
	tmpClosed := false
	defer func() {
		if closeErr := func() error {
			if tmpClosed {
				return nil
			}
			return tmp.Close()
		}(); err == nil && closeErr != nil {
			err = fmt.Errorf("close temporary blob: %w", closeErr)
		}
		if removeErr := os.Remove(tmpName); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && err == nil {
			err = fmt.Errorf("remove temporary blob: %w", removeErr)
		}
	}()

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), &contextReader{ctx: ctx, reader: reader})
	if err != nil {
		return result, fmt.Errorf("write temporary blob: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := tmp.Sync(); err != nil {
		return result, fmt.Errorf("sync temporary blob: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return result, fmt.Errorf("close temporary blob: %w", err)
	}
	tmpClosed = true

	digest := hex.EncodeToString(hash.Sum(nil))
	result = storage.PutResult{SHA256: digest, Size: size, Created: true}
	target, _ := s.blobPath(digest)
	shard := filepath.Dir(target)
	if err := s.ensureShard(digest); err != nil {
		return storage.PutResult{}, fmt.Errorf("create blob shard: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return storage.PutResult{}, err
	}
	if err := os.Link(tmpName, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return storage.PutResult{}, fmt.Errorf("publish blob: %w", err)
		}
		matches, checkErr := fileMatches(ctx, target, digest)
		if checkErr != nil {
			return storage.PutResult{}, checkErr
		}
		if !matches {
			return storage.PutResult{}, fmt.Errorf("%w: existing blob %s has unexpected contents", storage.ErrIntegrity, digest)
		}
		result.Created = false
		return result, nil
	}
	if err := syncDir(shard); err != nil {
		return storage.PutResult{}, fmt.Errorf("sync blob shard: %w", err)
	}
	return result, nil
}

func (s *Store) Open(ctx context.Context, digest string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.blobPath(digest)
	if err != nil {
		return nil, err
	}
	if err := requireRegularFile(path); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open blob %s: %w", digest, storage.ErrNotFound)
	} else if err != nil {
		return nil, fmt.Errorf("open blob %s: %w", digest, err)
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open blob %s: %w", digest, storage.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("open blob %s: %w", digest, err)
	}
	return file, nil
}

func (s *Store) Exists(ctx context.Context, digest string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := s.blobPath(digest)
	if err != nil {
		return false, err
	}
	err = requireRegularFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat blob %s: %w", digest, err)
	}
	return true, nil
}

func (s *Store) Delete(ctx context.Context, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.blobPath(digest)
	if err != nil {
		return err
	}
	if err := requireRegularFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("delete blob %s: %w", digest, err)
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("delete blob %s: %w", digest, err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync blob shard after delete: %w", err)
	}
	return nil
}

// Enumerate visits regular files stored at canonical content-addressed paths.
// Unknown files and directories are left untouched. Canonical paths backed by
// non-regular files are reported as integrity errors and are never yielded.
func (s *Store) Enumerate(ctx context.Context, yield func(storage.BlobInfo) error) error {
	if yield == nil {
		return errors.New("enumerate blobs: nil callback")
	}
	var failures []error
	err := filepath.WalkDir(s.root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			failures = append(failures, fmt.Errorf("enumerate blobs: %w", walkErr))
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if current == s.root {
			return nil
		}

		relative, err := filepath.Rel(s.root, current)
		if err != nil {
			return fmt.Errorf("enumerate blobs: resolve path: %w", err)
		}
		parts := strings.Split(relative, string(filepath.Separator))
		if entry.IsDir() {
			if len(parts) == 3 {
				digest := parts[0] + parts[1] + parts[2]
				if len(parts[0]) == 2 && len(parts[1]) == 2 && len(parts[2]) == 60 && storage.ValidateSHA256(digest) == nil {
					failures = append(failures, fmt.Errorf("enumerate blob %s: %w", digest, storage.ErrIntegrity))
				}
				return filepath.SkipDir
			}
			if len(parts) > 2 || !validShard(parts) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(parts) == 1 && strings.HasPrefix(entry.Name(), ".fpan-blob-") {
			return nil
		}
		if len(parts) != 3 {
			return nil
		}
		digest := parts[0] + parts[1] + parts[2]
		if len(parts[0]) != 2 || len(parts[1]) != 2 || len(parts[2]) != 60 || storage.ValidateSHA256(digest) != nil {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			failures = append(failures, fmt.Errorf("enumerate blob %s: %w", digest, err))
			return nil
		}
		if !info.Mode().IsRegular() {
			failures = append(failures, fmt.Errorf("enumerate blob %s: %w", digest, storage.ErrIntegrity))
			return nil
		}
		if err := yield(storage.BlobInfo{SHA256: digest, Size: info.Size(), ModifiedAt: info.ModTime()}); err != nil {
			return err
		}
		return nil
	})
	return errors.Join(err, errors.Join(failures...))
}

// CleanupTemporary removes stale temporary upload files from the storage root.
// Files newer than before are retained because they may belong to an active
// or recently interrupted upload.
func (s *Store) CleanupTemporary(ctx context.Context, before time.Time) (int, error) {
	if before.IsZero() {
		return 0, errors.New("cleanup temporary blobs: zero cutoff")
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return 0, fmt.Errorf("read storage root: %w", err)
	}
	removed := 0
	var failures []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return removed, errors.Join(append(failures, err)...)
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".fpan-blob-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			failures = append(failures, fmt.Errorf("stat temporary blob %s: %w", entry.Name(), err))
			continue
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(before) {
			continue
		}
		if err := os.Remove(filepath.Join(s.root, entry.Name())); err != nil {
			failures = append(failures, fmt.Errorf("delete temporary blob %s: %w", entry.Name(), err))
			continue
		}
		removed++
	}
	if removed > 0 {
		failures = append(failures, syncDir(s.root))
	}
	return removed, errors.Join(failures...)
}

func validShard(parts []string) bool {
	if len(parts) < 1 || len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if len(part) != 2 {
			return false
		}
		for _, char := range part {
			if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
				return false
			}
		}
	}
	return true
}

func (s *Store) ensureShard(digest string) error {
	first := filepath.Join(s.root, digest[:2])
	second := filepath.Join(first, digest[2:4])
	if err := os.MkdirAll(second, directoryMode); err != nil {
		return err
	}
	// Synchronizing each level makes newly-created directory entries durable.
	for _, directory := range []string{s.root, first, second} {
		if err := syncDir(directory); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) blobPath(digest string) (string, error) {
	if err := storage.ValidateSHA256(digest); err != nil {
		return "", err
	}
	return filepath.Join(s.root, digest[:2], digest[2:4], digest[4:]), nil
}

func fileMatches(ctx context.Context, path, expected string) (bool, error) {
	if err := requireRegularFile(path); err != nil {
		return false, fmt.Errorf("verify existing blob: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("verify existing blob: %w", err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, &contextReader{ctx: ctx, reader: file}); err != nil {
		return false, fmt.Errorf("verify existing blob: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("verify existing blob: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected, nil
}

func requireRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: blob target is not a regular file", storage.ErrIntegrity)
	}
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	return errors.Join(syncErr, closeErr)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
