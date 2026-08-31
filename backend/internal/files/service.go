// Package files coordinates file metadata with physical blob storage.
package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/liuhq/fpan/internal/database"
	"github.com/liuhq/fpan/internal/models"
	"github.com/liuhq/fpan/internal/storage"
)

const (
	DefaultGCGracePeriod = 24 * time.Hour
	DefaultGCBatchSize   = 100
)

var (
	ErrInvalidInput     = errors.New("invalid file service input")
	ErrInconsistentBlob = errors.New("blob metadata exists but physical content is missing")
)

type Repository interface {
	CreateFile(context.Context, *models.File, *models.Blob) error
	GetFile(context.Context, uint) (*models.File, error)
	GetBlob(context.Context, string) (*models.Blob, error)
	BlobExists(context.Context, string) (bool, error)
	ListUnreferencedBlobs(context.Context, int) ([]models.Blob, error)
	DeleteBlobIfUnreferenced(context.Context, string) (bool, error)
}

type BlobStore interface {
	storage.Store
	storage.BlobEnumerator
	storage.TemporaryBlobCleaner
}

type Service struct {
	repository Repository
	store      BlobStore
	gate       sync.RWMutex
}

type UploadInput struct {
	Display  string
	MimeType string
	ParentID *uint
	Content  io.Reader
}

type GCOptions struct {
	// Before is the exclusive age cutoff. The default is 24 hours ago.
	Before      time.Time
	GracePeriod time.Duration
	BatchSize   int
}

type GCReport struct {
	DatabaseBlobsDeleted  int
	StorageBlobsDeleted   int
	TemporaryFilesDeleted int
	Failures              int
}

func New(repository Repository, store BlobStore) (*Service, error) {
	if repository == nil || store == nil {
		return nil, fmt.Errorf("create file service: %w", ErrInvalidInput)
	}
	return &Service{repository: repository, store: store}, nil
}

// Upload stores content before registering its metadata. If registration
// fails, the physical blob is intentionally retained for garbage collection.
func (s *Service) Upload(ctx context.Context, input UploadInput) (*models.File, error) {
	if input.Content == nil {
		return nil, fmt.Errorf("upload file: %w: nil content", ErrInvalidInput)
	}
	s.gate.RLock()
	defer s.gate.RUnlock()

	result, err := s.store.Put(ctx, input.Content)
	if err != nil {
		return nil, fmt.Errorf("upload file: store content: %w", err)
	}
	file := &models.File{
		Display:  input.Display,
		MimeType: input.MimeType,
		ParentID: input.ParentID,
		SHA256:   result.SHA256,
	}
	blob := &models.Blob{SHA256: result.SHA256, Size: result.Size}
	if err := s.repository.CreateFile(ctx, file, blob); err != nil {
		return nil, fmt.Errorf("upload file: register metadata: %w", err)
	}
	stored, err := s.repository.GetFile(ctx, file.ID)
	if err != nil {
		return nil, fmt.Errorf("upload file: load metadata: %w", err)
	}
	return stored, nil
}

// OpenBlob verifies that metadata exists before opening physical content.
func (s *Service) OpenBlob(ctx context.Context, digest string) (*models.Blob, io.ReadCloser, error) {
	if err := storage.ValidateSHA256(digest); err != nil {
		return nil, nil, fmt.Errorf("open blob: %w", err)
	}
	blob, err := s.repository.GetBlob(ctx, digest)
	if err != nil {
		return nil, nil, fmt.Errorf("open blob metadata: %w", err)
	}
	reader, err := s.store.Open(ctx, digest)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil, fmt.Errorf("open blob %s: %w: %w", digest, ErrInconsistentBlob, err)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open blob %s: %w", digest, err)
	}
	return blob, reader, nil
}

// CollectGarbage removes old unreferenced database blobs and old physical
// blobs with no database metadata. It excludes recent blobs so failed or
// in-flight uploads can settle before collection.
func (s *Service) CollectGarbage(ctx context.Context, options GCOptions) (GCReport, error) {
	if options.BatchSize < 0 {
		return GCReport{}, fmt.Errorf("collect garbage: %w: negative batch size", ErrInvalidInput)
	}
	if options.BatchSize == 0 {
		options.BatchSize = DefaultGCBatchSize
	}
	if options.Before.IsZero() {
		if options.GracePeriod < 0 {
			return GCReport{}, fmt.Errorf("collect garbage: %w: negative grace period", ErrInvalidInput)
		}
		if options.GracePeriod == 0 {
			options.GracePeriod = DefaultGCGracePeriod
		}
		options.Before = time.Now().UTC().Add(-options.GracePeriod)
	}

	s.gate.Lock()
	defer s.gate.Unlock()

	report := GCReport{}
	var failures []error
	candidates, err := s.repository.ListUnreferencedBlobs(ctx, options.BatchSize)
	if err != nil {
		return report, fmt.Errorf("collect garbage: list database blobs: %w", err)
	}
	for _, blob := range candidates {
		if err := ctx.Err(); err != nil {
			return report, errors.Join(append(failures, err)...)
		}
		if !blob.CreatedAt.Before(options.Before) {
			continue
		}
		deleted, err := s.repository.DeleteBlobIfUnreferenced(ctx, blob.SHA256)
		if err != nil {
			report.Failures++
			failures = append(failures, fmt.Errorf("delete database blob %s: %w", blob.SHA256, err))
			continue
		}
		if !deleted {
			continue
		}
		report.DatabaseBlobsDeleted++
		exists, err := s.store.Exists(ctx, blob.SHA256)
		if err != nil {
			report.Failures++
			failures = append(failures, fmt.Errorf("check physical blob %s: %w", blob.SHA256, err))
			continue
		}
		if !exists {
			continue
		}
		if err := s.store.Delete(ctx, blob.SHA256); err != nil {
			report.Failures++
			failures = append(failures, fmt.Errorf("delete physical blob %s: %w", blob.SHA256, err))
			continue
		}
		report.StorageBlobsDeleted++
	}

	err = s.store.Enumerate(ctx, func(blob storage.BlobInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !blob.ModifiedAt.Before(options.Before) {
			return nil
		}
		exists, err := s.repository.BlobExists(ctx, blob.SHA256)
		if err != nil {
			report.Failures++
			failures = append(failures, fmt.Errorf("check database blob %s: %w", blob.SHA256, err))
			return nil
		}
		if exists {
			return nil
		}
		if err := s.store.Delete(ctx, blob.SHA256); err != nil {
			report.Failures++
			failures = append(failures, fmt.Errorf("delete orphaned physical blob %s: %w", blob.SHA256, err))
			return nil
		}
		report.StorageBlobsDeleted++
		return nil
	})
	if err != nil {
		report.Failures++
		failures = append(failures, fmt.Errorf("enumerate physical blobs: %w", err))
	}
	temporary, err := s.store.CleanupTemporary(ctx, options.Before)
	report.TemporaryFilesDeleted += temporary
	if err != nil {
		report.Failures++
		failures = append(failures, fmt.Errorf("clean temporary blobs: %w", err))
	}
	return report, errors.Join(failures...)
}

// RunGarbageCollector runs collection on a fixed interval until ctx is
// canceled. Collection errors are reported to logf and do not stop later
// rounds.
func (s *Service) RunGarbageCollector(ctx context.Context, interval time.Duration, options GCOptions, logf func(string, ...any)) error {
	if interval <= 0 {
		return fmt.Errorf("run garbage collector: %w: interval must be positive", ErrInvalidInput)
	}
	if logf == nil {
		return fmt.Errorf("run garbage collector: %w: nil logger", ErrInvalidInput)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			report, err := s.CollectGarbage(ctx, options)
			if err != nil {
				logf("WARN: blob garbage collection failed: %v (database=%d storage=%d temporary=%d failures=%d)", err, report.DatabaseBlobsDeleted, report.StorageBlobsDeleted, report.TemporaryFilesDeleted, report.Failures)
				continue
			}
			logf("INFO: blob garbage collection complete (database=%d storage=%d temporary=%d)", report.DatabaseBlobsDeleted, report.StorageBlobsDeleted, report.TemporaryFilesDeleted)
		}
	}
}

var _ Repository = (*database.DB)(nil)
