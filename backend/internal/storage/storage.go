// Package storage defines content-addressed blob storage contracts.
package storage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("blob not found")
	ErrInvalidSHA256 = errors.New("invalid SHA256")
	ErrIntegrity     = errors.New("blob integrity check failed")
	ErrInvalidPath   = errors.New("invalid logical path")
)

// PutResult describes a blob written to a Store.
type PutResult struct {
	SHA256  string
	Size    int64
	Created bool
}

// Store stores blobs by the SHA256 digest of their contents.
type Store interface {
	Put(context.Context, io.Reader) (PutResult, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Exists(context.Context, string) (bool, error)
	Delete(context.Context, string) error
}

// BlobInfo describes a physical blob discovered in a Store.
type BlobInfo struct {
	SHA256     string
	Size       int64
	ModifiedAt time.Time
}

// BlobEnumerator visits physical blobs without loading their contents.
// Implementations must not yield temporary files or unrecognized paths.
type BlobEnumerator interface {
	Enumerate(context.Context, func(BlobInfo) error) error
}

// TemporaryBlobCleaner removes stale files created while publishing blobs.
type TemporaryBlobCleaner interface {
	CleanupTemporary(context.Context, time.Time) (int, error)
}

// ValidateSHA256 checks that digest is a canonical SHA256 string.
func ValidateSHA256(digest string) error {
	if len(digest) != sha256.Size*2 {
		return fmt.Errorf("%w: must be 64 lowercase hexadecimal characters", ErrInvalidSHA256)
	}
	for _, char := range digest {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return fmt.Errorf("%w: must be 64 lowercase hexadecimal characters", ErrInvalidSHA256)
		}
	}
	return nil
}

const AssociationFormatVersion = 1

// AssociationRecord is one line in the versioned JSONL association backup.
type AssociationRecord struct {
	Version int      `json:"version"`
	SHA256  string   `json:"sha256"`
	Paths   []string `json:"paths"`
}

// NewAssociationRecord creates a deterministic record by sorting and
// de-duplicating logical paths.
func NewAssociationRecord(digest string, paths []string) (AssociationRecord, error) {
	if err := ValidateSHA256(digest); err != nil {
		return AssociationRecord{}, fmt.Errorf("create association record: %w", err)
	}
	unique := make(map[string]struct{}, len(paths))
	for _, logicalPath := range paths {
		if err := validateLogicalPath(logicalPath); err != nil {
			return AssociationRecord{}, fmt.Errorf("create association record: %w", err)
		}
		unique[logicalPath] = struct{}{}
	}
	normalized := make([]string, 0, len(unique))
	for path := range unique {
		normalized = append(normalized, path)
	}
	sort.Strings(normalized)
	return AssociationRecord{Version: AssociationFormatVersion, SHA256: digest, Paths: normalized}, nil
}

func validateLogicalPath(logicalPath string) error {
	if logicalPath == "" || path.IsAbs(logicalPath) || strings.ContainsRune(logicalPath, '\\') || path.Clean(logicalPath) != logicalPath {
		return fmt.Errorf("%w: %q must be a canonical root-relative slash-separated path", ErrInvalidPath, logicalPath)
	}
	for _, segment := range strings.Split(logicalPath, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("%w: %q must not contain dot segments", ErrInvalidPath, logicalPath)
		}
	}
	return nil
}

// AssociationExporter writes versioned JSONL blob-to-logical-path records.
type AssociationExporter interface {
	ExportAssociations(context.Context, io.Writer) error
}
