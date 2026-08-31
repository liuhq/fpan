package filesystem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/liuhq/fpan/internal/storage"
)

func TestPutKnownContentAndPath(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Put(context.Background(), strings.NewReader("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	const digest = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if result.SHA256 != digest || result.Size != 11 || !result.Created {
		t.Fatalf("unexpected result: %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(root, "b9", "4d", digest[4:]))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world" {
		t.Fatalf("content = %q", content)
	}
}

func TestPutEmptyAndLarge(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range [][]byte{nil, bytes.Repeat([]byte("stream"), 100_000)} {
		result, err := store.Put(context.Background(), bytes.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		if result.Size != int64(len(content)) {
			t.Fatalf("size = %d", result.Size)
		}
	}
}

func TestDuplicateAndConcurrentPut(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	const count = 12
	results := make(chan storage.PutResult, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.Put(context.Background(), strings.NewReader("same data"))
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	created := 0
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		if result.Created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created count = %d, want 1", created)
	}
	var blobs int
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".fpan-blob-") {
			blobs++
		}
		return err
	})
	if err != nil || blobs != 1 {
		t.Fatalf("walk error %v, blobs %d", err, blobs)
	}
}

func TestDamagedExistingBlob(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Put(context.Background(), strings.NewReader("original"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, result.SHA256[:2], result.SHA256[2:4], result.SHA256[4:])
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = store.Put(context.Background(), strings.NewReader("original"))
	if !errors.Is(err, storage.ErrIntegrity) {
		t.Fatalf("error = %v", err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "damaged" {
		t.Fatalf("target overwritten: %q", content)
	}
}

func TestOpenExistsDelete(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Put(context.Background(), strings.NewReader("content"))
	if err != nil {
		t.Fatal(err)
	}
	exists, err := store.Exists(context.Background(), result.SHA256)
	if err != nil || !exists {
		t.Fatalf("exists %v, error %v", exists, err)
	}
	reader, err := store.Open(context.Background(), result.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || string(content) != "content" {
		t.Fatalf("content %q, error %v", content, err)
	}
	if closeErr != nil {
		t.Fatalf("close reader: %v", closeErr)
	}
	if err := store.Delete(context.Background(), result.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), result.SHA256); err != nil {
		t.Fatal(err)
	}
	_, err = store.Open(context.Background(), result.SHA256)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestInvalidDigests(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	invalid := []string{"", strings.Repeat("a", 63), strings.Repeat("A", 64), "../../" + strings.Repeat("a", 58), strings.Repeat("g", 64)}
	for _, digest := range invalid {
		if _, err := store.Open(context.Background(), digest); !errors.Is(err, storage.ErrInvalidSHA256) {
			t.Errorf("Open(%q): %v", digest, err)
		}
		if _, err := store.Exists(context.Background(), digest); !errors.Is(err, storage.ErrInvalidSHA256) {
			t.Errorf("Exists(%q): %v", digest, err)
		}
		if err := store.Delete(context.Background(), digest); !errors.Is(err, storage.ErrInvalidSHA256) {
			t.Errorf("Delete(%q): %v", digest, err)
		}
	}
}

func TestNewRejectsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("New accepted a file")
	}
}

func TestCanceledPutCleansTemporaryFile(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelingReader{cancel: cancel}
	_, err = store.Put(ctx, reader)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("leftover entries: %v", entries)
	}
}

func TestFileMatchesHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(path, bytes.Repeat([]byte("data"), 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fileMatches(ctx, path, strings.Repeat("a", 64))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}

func TestNonRegularBlobTargetsReturnIntegrityError(t *testing.T) {
	tests := []struct {
		name   string
		create func(string) error
	}{
		{name: "directory", create: func(path string) error { return os.Mkdir(path, 0o700) }},
		{name: "symlink", create: func(path string) error { return os.Symlink("elsewhere", path) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			const content = "original"
			const digest = "0682c5f2076f099c34cfdd15a9e063849ed437a49677e6fcc5b4198c76575be5"
			target := filepath.Join(root, digest[:2], digest[2:4], digest[4:])
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := test.create(target); err != nil {
				t.Fatal(err)
			}

			if _, err := store.Put(context.Background(), strings.NewReader(content)); !errors.Is(err, storage.ErrIntegrity) {
				t.Fatalf("Put error = %v", err)
			}
			if _, err := store.Open(context.Background(), digest); !errors.Is(err, storage.ErrIntegrity) {
				t.Fatalf("Open error = %v", err)
			}
			if exists, err := store.Exists(context.Background(), digest); exists || !errors.Is(err, storage.ErrIntegrity) {
				t.Fatalf("Exists = %v, error = %v", exists, err)
			}
			if err := store.Delete(context.Background(), digest); !errors.Is(err, storage.ErrIntegrity) {
				t.Fatalf("Delete error = %v", err)
			}
			if _, err := os.Lstat(target); err != nil {
				t.Fatalf("target was changed: %v", err)
			}
		})
	}
}

func TestEnumerateCanonicalBlobs(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Put(context.Background(), strings.NewReader("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(context.Background(), strings.NewReader("second"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrecognized"), []byte("leave me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".fpan-blob-leftover"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got := map[string]int64{}
	err = store.Enumerate(context.Background(), func(blob storage.BlobInfo) error {
		got[blob.SHA256] = blob.Size
		if blob.ModifiedAt.IsZero() {
			t.Error("missing modification time")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{first.SHA256: first.Size, second.SHA256: second.Size}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Enumerate() = %#v, want %#v", got, want)
	}
}

func TestEnumerateRejectsNonRegularCanonicalBlob(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			store, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			digest := strings.Repeat("a", 64)
			target := filepath.Join(root, digest[:2], digest[2:4], digest[4:])
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				err = os.Mkdir(target, 0o700)
			} else {
				err = os.Symlink("elsewhere", target)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Enumerate(context.Background(), func(storage.BlobInfo) error { return nil }); !errors.Is(err, storage.ErrIntegrity) {
				t.Fatalf("Enumerate() error = %v, want integrity error", err)
			}
		})
	}
}

func TestEnumerateHonorsCancellation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Enumerate(ctx, func(storage.BlobInfo) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Enumerate() error = %v, want context cancellation", err)
	}
}

type cancelingReader struct {
	cancel context.CancelFunc
	read   bool
}

func (r *cancelingReader) Read(buffer []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	r.cancel()
	return copy(buffer, "partially written"), nil
}
