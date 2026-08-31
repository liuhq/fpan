package files

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuhq/fpan/internal/database"
	"github.com/liuhq/fpan/internal/models"
	"github.com/liuhq/fpan/internal/storage"
	"github.com/liuhq/fpan/internal/storage/filesystem"
)

func TestUploadStoresContentAndMetadata(t *testing.T) {
	repository := newFakeRepository()
	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(repository, store)
	if err != nil {
		t.Fatal(err)
	}

	file, err := service.Upload(context.Background(), UploadInput{
		Display: "hello.txt", MimeType: "text/plain", Content: strings.NewReader("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if file.ID == 0 || file.Display != "hello.txt" || file.Blob.Size != 5 {
		t.Fatalf("unexpected file: %#v", file)
	}
	reader, err := store.Open(context.Background(), file.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || string(content) != "hello" {
		t.Fatalf("stored content = %q, error = %v", content, err)
	}
	if closeErr != nil {
		t.Fatalf("close stored content: %v", closeErr)
	}

	second, err := service.Upload(context.Background(), UploadInput{
		Display: "copy.txt", MimeType: "text/plain", Content: strings.NewReader("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.SHA256 != file.SHA256 || len(repository.blobs) != 1 {
		t.Fatalf("duplicate upload was not deduplicated: %#v", second)
	}
}

func TestUploadFailureLeavesPhysicalBlobForCollection(t *testing.T) {
	root := t.TempDir()
	repository := newFakeRepository()
	repository.createErr = database.ErrConflict
	store, err := filesystem.New(root)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(repository, store)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Upload(context.Background(), UploadInput{Display: "conflict", Content: strings.NewReader("orphan")})
	if !errors.Is(err, database.ErrConflict) {
		t.Fatalf("Upload() error = %v", err)
	}
	digest := "88f6811ab5d8fc6d3177f9b7609ae0fcebfda187e5046b62d38bb539e88b74d7"
	exists, err := store.Exists(context.Background(), digest)
	if err != nil || !exists {
		t.Fatalf("orphan exists = %t, error = %v", exists, err)
	}
}

func TestOpenBlobReportsMissingPhysicalContent(t *testing.T) {
	repository := newFakeRepository()
	digest := strings.Repeat("a", 64)
	repository.blobs[digest] = models.Blob{SHA256: digest, Size: 1, CreatedAt: time.Now()}
	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(repository, store)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.OpenBlob(context.Background(), digest)
	if !errors.Is(err, ErrInconsistentBlob) {
		t.Fatalf("OpenBlob() error = %v", err)
	}
}

func TestCollectGarbageHonorsCutoffAndReferences(t *testing.T) {
	root := t.TempDir()
	repository := newFakeRepository()
	store, err := filesystem.New(root)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(repository, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	oldDatabaseBlob, err := store.Put(ctx, strings.NewReader("old database orphan"))
	if err != nil {
		t.Fatal(err)
	}
	repository.blobs[oldDatabaseBlob.SHA256] = models.Blob{
		SHA256: oldDatabaseBlob.SHA256, Size: oldDatabaseBlob.Size, CreatedAt: now.Add(-48 * time.Hour),
	}
	oldDiskOnly, err := store.Put(ctx, strings.NewReader("old disk orphan"))
	if err != nil {
		t.Fatal(err)
	}
	recentDiskOnly, err := store.Put(ctx, strings.NewReader("recent disk orphan"))
	if err != nil {
		t.Fatal(err)
	}
	referenced, err := store.Put(ctx, strings.NewReader("referenced"))
	if err != nil {
		t.Fatal(err)
	}
	repository.blobs[referenced.SHA256] = models.Blob{
		SHA256: referenced.SHA256, Size: referenced.Size, CreatedAt: now.Add(-48 * time.Hour),
	}
	repository.references[referenced.SHA256] = 1
	setBlobModTime(t, root, oldDatabaseBlob.SHA256, now.Add(-48*time.Hour))
	setBlobModTime(t, root, oldDiskOnly.SHA256, now.Add(-48*time.Hour))

	report, err := service.CollectGarbage(ctx, GCOptions{Before: now.Add(-24 * time.Hour), BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if report.DatabaseBlobsDeleted != 1 || report.StorageBlobsDeleted != 2 || report.Failures != 0 {
		t.Fatalf("CollectGarbage() report = %#v", report)
	}
	for _, digest := range []string{oldDatabaseBlob.SHA256, oldDiskOnly.SHA256} {
		if exists, err := store.Exists(ctx, digest); err != nil || exists {
			t.Fatalf("collected blob %s exists = %t, error = %v", digest, exists, err)
		}
	}
	for _, digest := range []string{recentDiskOnly.SHA256, referenced.SHA256} {
		if exists, err := store.Exists(ctx, digest); err != nil || !exists {
			t.Fatalf("retained blob %s exists = %t, error = %v", digest, exists, err)
		}
	}
}

func TestCollectGarbageWaitsForUpload(t *testing.T) {
	repository := newFakeRepository()
	base, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &blockingPutStore{BlobStore: base, entered: make(chan struct{}), release: make(chan struct{})}
	service, err := New(repository, store)
	if err != nil {
		t.Fatal(err)
	}
	uploadDone := make(chan error, 1)
	go func() {
		_, err := service.Upload(context.Background(), UploadInput{Display: "file", Content: strings.NewReader("content")})
		uploadDone <- err
	}()
	<-store.entered

	gcDone := make(chan error, 1)
	go func() {
		_, err := service.CollectGarbage(context.Background(), GCOptions{Before: time.Now().Add(time.Hour)})
		gcDone <- err
	}()
	select {
	case <-repository.listCalled:
		t.Fatal("garbage collection entered repository while upload held the gate")
	case <-time.After(50 * time.Millisecond):
	}
	close(store.release)
	if err := <-uploadDone; err != nil {
		t.Fatal(err)
	}
	if err := <-gcDone; err != nil {
		t.Fatal(err)
	}
}

func TestCollectGarbageCleansStaleTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	repository := newFakeRepository()
	store, err := filesystem.New(root)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(repository, store)
	if err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(root, ".fpan-blob-stale")
	if err := os.WriteFile(temporary, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := os.Chtimes(temporary, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	report, err := service.CollectGarbage(context.Background(), GCOptions{Before: now.Add(-24 * time.Hour)})
	if err != nil || report.TemporaryFilesDeleted != 1 {
		t.Fatalf("CollectGarbage() = %#v, %v", report, err)
	}
	if _, err := os.Stat(temporary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file stat error = %v", err)
	}
}

func TestRunGarbageCollectorStopsWhenCanceled(t *testing.T) {
	repository := newFakeRepository()
	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(repository, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logs := make(chan struct{}, 1)
	err = service.RunGarbageCollector(ctx, time.Millisecond, GCOptions{}, func(string, ...any) {
		select {
		case logs <- struct{}{}:
			cancel()
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-logs:
	default:
		t.Fatal("garbage collector did not run before stopping")
	}
}

func setBlobModTime(t *testing.T, root, digest string, modified time.Time) {
	t.Helper()
	path := filepath.Join(root, digest[:2], digest[2:4], digest[4:])
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

type blockingPutStore struct {
	BlobStore
	entered chan struct{}
	release chan struct{}
}

func (s *blockingPutStore) Put(ctx context.Context, reader io.Reader) (storage.PutResult, error) {
	close(s.entered)
	select {
	case <-ctx.Done():
		return storage.PutResult{}, ctx.Err()
	case <-s.release:
		return s.BlobStore.Put(ctx, reader)
	}
}

type fakeRepository struct {
	mu         sync.Mutex
	blobs      map[string]models.Blob
	files      map[uint]models.File
	references map[string]int
	nextID     uint
	createErr  error
	listCalled chan struct{}
	listOnce   sync.Once
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		blobs: make(map[string]models.Blob), files: make(map[uint]models.File), references: make(map[string]int),
		listCalled: make(chan struct{}),
	}
}

func (r *fakeRepository) CreateFile(_ context.Context, file *models.File, blob *models.Blob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	stored, ok := r.blobs[blob.SHA256]
	if !ok {
		stored = *blob
		stored.CreatedAt = time.Now().UTC()
		r.blobs[blob.SHA256] = stored
	}
	r.nextID++
	file.ID = r.nextID
	copy := *file
	copy.Blob = stored
	r.files[file.ID] = copy
	r.references[file.SHA256]++
	return nil
}

func (r *fakeRepository) GetFile(_ context.Context, id uint) (*models.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &file, nil
}

func (r *fakeRepository) GetBlob(_ context.Context, digest string) (*models.Blob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	blob, ok := r.blobs[digest]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &blob, nil
}

func (r *fakeRepository) BlobExists(_ context.Context, digest string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.blobs[digest]
	return ok, nil
}

func (r *fakeRepository) ListUnreferencedBlobs(_ context.Context, limit int) ([]models.Blob, error) {
	r.listOnce.Do(func() { close(r.listCalled) })
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]models.Blob, 0)
	for digest, blob := range r.blobs {
		if r.references[digest] == 0 {
			result = append(result, blob)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *fakeRepository) DeleteBlobIfUnreferenced(_ context.Context, digest string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.references[digest] != 0 {
		return false, nil
	}
	if _, ok := r.blobs[digest]; !ok {
		return false, nil
	}
	delete(r.blobs, digest)
	return true, nil
}
