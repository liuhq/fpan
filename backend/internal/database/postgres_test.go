package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm/logger"
)

const testDatabaseURLEnv = "FPAN_TEST_DATABASE_URL"

func TestPostgresMigrationAndConstraints(t *testing.T) {
	db := newPostgresTestDB(t)

	if err := db.Migrate(); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	for _, table := range []string{"blobs", "folders", "files", "shares"} {
		var name *string
		if err := db.Raw("SELECT to_regclass(?)::text", table).Scan(&name).Error; err != nil {
			t.Fatalf("look up table %q: %v", table, err)
		}
		if name == nil || *name != table {
			t.Fatalf("table %q was not migrated: %v", table, name)
		}
	}

	invalidDigest := strings.Repeat("z", 64)
	if err := db.Create(&models.Blob{SHA256: invalidDigest, Size: 1}).Error; err == nil {
		t.Fatal("database accepted an invalid blob digest")
	}
	if err := db.Create(&models.Blob{SHA256: digest('a'), Size: -1}).Error; err == nil {
		t.Fatal("database accepted a negative blob size")
	}
	if err := db.Create(&models.Folder{Display: "   "}).Error; err == nil {
		t.Fatal("database accepted a blank folder display")
	}
	if err := db.Create(&models.File{Display: "orphan", MimeType: "text/plain", SHA256: digest('b')}).Error; err == nil {
		t.Fatal("database accepted a file without its blob")
	}
}

func TestPostgresFoldersFilesAndEntries(t *testing.T) {
	db := newPostgresTestDB(t)
	ctx := context.Background()

	root := createFolder(t, db, "root", nil)
	alpha := createFolder(t, db, "alpha", &root.ID)
	child := createFolder(t, db, "child", &root.ID)
	grandchild := createFolder(t, db, "grandchild", &child.ID)

	descendant, err := db.IsFolderDescendant(ctx, grandchild.ID, root.ID)
	if err != nil || !descendant {
		t.Fatalf("grandchild descendant of root = %t, error = %v", descendant, err)
	}
	descendant, err = db.IsFolderDescendant(ctx, root.ID, child.ID)
	if err != nil || descendant {
		t.Fatalf("root descendant of child = %t, error = %v", descendant, err)
	}
	if _, err := db.UpdateFolder(ctx, root.ID, UpdateFolderInput{ParentID: Optional[*uint]{Set: true, Value: &grandchild.ID}}); !errors.Is(err, ErrInvalidMove) {
		t.Fatalf("move folder into descendant error = %v, want ErrInvalidMove", err)
	}
	if _, err := db.UpdateFolder(ctx, root.ID, UpdateFolderInput{ParentID: Optional[*uint]{Set: true, Value: &root.ID}}); !errors.Is(err, ErrInvalidMove) {
		t.Fatalf("move folder into itself error = %v, want ErrInvalidMove", err)
	}
	if err := db.CreateFolder(ctx, &models.Folder{Display: "root"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate root folder error = %v, want ErrConflict", err)
	}

	percent := createFile(t, db, "100%_done.txt", &root.ID, '1', 10)
	zeta := createFile(t, db, "zeta.txt", &root.ID, '2', 20)
	createFile(t, db, "alpha", &root.ID, '3', 30) // Files and folders have separate namespaces.

	page, err := db.ListEntries(ctx, &root.ID, ListEntriesOptions{Page: 1, Size: 2, Sort: SortDescending, SortBy: EntrySortName})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 5 || len(page.Items) != 2 || page.Items[0].File == nil || page.Items[0].File.ID != zeta.ID {
		t.Fatalf("unexpected first entries page: %#v", page)
	}
	filtered, err := db.ListEntries(ctx, &root.ID, ListEntriesOptions{Filter: "%_", Type: EntryTypeFile})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].File.ID != percent.ID {
		t.Fatalf("escaped entry filter returned %#v", filtered)
	}
	folders, err := db.ListEntries(ctx, &root.ID, ListEntriesOptions{Type: EntryTypeFolder})
	if err != nil || folders.Total != 2 || folders.Items[0].Folder.ID != alpha.ID {
		t.Fatalf("folder-only listing = %#v, error = %v", folders, err)
	}
	missingParent := uint(999999)
	if _, err := db.ListEntries(ctx, &missingParent, ListEntriesOptions{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("list missing parent error = %v, want ErrNotFound", err)
	}

	moved, err := db.UpdateFile(ctx, zeta.ID, UpdateFileInput{
		Display:  Optional[string]{Set: true, Value: "moved.txt"},
		ParentID: Optional[*uint]{Set: true, Value: &child.ID},
	})
	if err != nil || moved.Display != "moved.txt" || moved.ParentID == nil || *moved.ParentID != child.ID {
		t.Fatalf("updated file = %#v, error = %v", moved, err)
	}
	if _, err := db.UpdateFile(ctx, moved.ID, UpdateFileInput{ParentID: Optional[*uint]{Set: true, Value: &missingParent}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move file to missing parent error = %v, want ErrNotFound", err)
	}
	if err := db.CreateFolder(ctx, &models.Folder{Display: "missing-parent", ParentID: &missingParent}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("create folder below missing parent error = %v, want ErrNotFound", err)
	}

	if err := db.DeleteFile(ctx, percent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetFile(ctx, percent.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted file error = %v, want ErrNotFound", err)
	}
	createFile(t, db, percent.Display, &root.ID, '4', 40)
	if err := db.DeleteFolder(ctx, alpha.ID); err != nil {
		t.Fatal(err)
	}
	createFolder(t, db, alpha.Display, &root.ID)
}

func TestPostgresBlobLifecycle(t *testing.T) {
	db := newPostgresTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	oldest := models.Blob{SHA256: digest('a'), Size: 1, CreatedAt: now.Add(-3 * time.Hour)}
	newer := models.Blob{SHA256: digest('b'), Size: 2, CreatedAt: now.Add(-2 * time.Hour)}
	referenced := models.Blob{SHA256: digest('c'), Size: 3, CreatedAt: now.Add(-time.Hour)}
	if err := db.Create(&[]models.Blob{oldest, newer}).Error; err != nil {
		t.Fatal(err)
	}
	createFileWithBlob(t, db, &models.File{Display: "referenced", MimeType: "text/plain", SHA256: referenced.SHA256}, &referenced)
	createFileWithBlob(t, db, &models.File{Display: "copy", MimeType: "text/plain", SHA256: referenced.SHA256}, &referenced)

	exists, err := db.BlobExists(ctx, referenced.SHA256)
	if err != nil || !exists {
		t.Fatalf("referenced blob exists = %t, error = %v", exists, err)
	}
	if err := db.CreateFile(ctx, &models.File{Display: "bad-copy", MimeType: "text/plain", SHA256: referenced.SHA256}, &models.Blob{SHA256: referenced.SHA256, Size: 99}); !errors.Is(err, ErrConflict) {
		t.Fatalf("blob metadata mismatch error = %v, want ErrConflict", err)
	}

	orphans, err := db.ListUnreferencedBlobs(ctx, 1)
	if err != nil || len(orphans) != 1 || orphans[0].SHA256 != oldest.SHA256 {
		t.Fatalf("oldest orphan = %#v, error = %v", orphans, err)
	}
	if _, err := db.ListUnreferencedBlobs(ctx, 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("invalid orphan limit error = %v, want ErrConflict", err)
	}
	deleted, err := db.DeleteBlobIfUnreferenced(ctx, referenced.SHA256)
	if err != nil || deleted {
		t.Fatalf("delete referenced blob = %t, error = %v", deleted, err)
	}
	deleted, err = db.DeleteBlobIfUnreferenced(ctx, oldest.SHA256)
	if err != nil || !deleted {
		t.Fatalf("delete orphan blob = %t, error = %v", deleted, err)
	}
	if _, err := db.GetBlob(ctx, oldest.SHA256); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted blob error = %v, want ErrNotFound", err)
	}
	if exists, err := db.BlobExists(ctx, newer.SHA256); err != nil || !exists {
		t.Fatalf("newer orphan retained = %t, error = %v", exists, err)
	}
}

func TestPostgresTrashLifecycle(t *testing.T) {
	db := newPostgresTestDB(t)
	ctx := context.Background()

	root := createFolder(t, db, "trash-root", nil)
	child := createFolder(t, db, "trash-child", &root.ID)
	file := createFile(t, db, "nested.txt", &child.ID, 'd', 4)
	if err := db.DeleteFolder(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	trash, err := db.ListTrash(ctx)
	if err != nil || len(trash) != 1 || trash[0].Folder == nil || trash[0].Folder.ID != root.ID {
		t.Fatalf("trash after subtree deletion = %#v, error = %v", trash, err)
	}
	if err := db.Restore(ctx, models.EntryTypeFolder, root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetFolder(ctx, child.ID); err != nil {
		t.Fatalf("restored child: %v", err)
	}
	if _, err := db.GetFile(ctx, file.ID); err != nil {
		t.Fatalf("restored nested file: %v", err)
	}
	if err := db.Restore(ctx, models.EntryTypeFolder, root.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("restore active folder error = %v, want ErrConflict", err)
	}
	if err := db.Purge(ctx, models.EntryTypeFolder, root.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("purge active folder error = %v, want ErrConflict", err)
	}

	if err := db.DeleteFolder(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Purge(ctx, models.EntryTypeFolder, root.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Unscoped().First(&models.Folder{}, root.ID).Error; err == nil {
		t.Fatal("purged root folder still exists")
	}
	if exists, err := db.BlobExists(ctx, file.SHA256); err != nil || !exists {
		t.Fatalf("blob after purge exists = %t, error = %v", exists, err)
	}

	first := createFile(t, db, "first.txt", nil, 'e', 5)
	second := createFile(t, db, "second.txt", nil, 'f', 6)
	if err := db.DeleteFile(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteFile(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.EmptyTrash(ctx); err != nil {
		t.Fatal(err)
	}
	if trash, err := db.ListTrash(ctx); err != nil || len(trash) != 0 {
		t.Fatalf("trash after empty = %#v, error = %v", trash, err)
	}
	for _, sha256 := range []string{first.SHA256, second.SHA256} {
		if exists, err := db.BlobExists(ctx, sha256); err != nil || !exists {
			t.Fatalf("blob %s after empty trash exists = %t, error = %v", sha256, exists, err)
		}
	}
	if err := db.Restore(ctx, models.EntryType("invalid"), 1); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("restore invalid type error = %v, want ErrInvalidInput", err)
	}
}

func TestPostgresShares(t *testing.T) {
	db := newPostgresTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	root := createFolder(t, db, "shared-root", nil)
	child := createFolder(t, db, "shared-child", &root.ID)
	inside := createFile(t, db, "inside.txt", &child.ID, '1', 7)
	outsideFolder := createFolder(t, db, "outside", nil)
	outside := createFile(t, db, "outside.txt", &outsideFolder.ID, '2', 8)
	limit := uint(3)
	share := &models.Share{EntryID: root.ID, EntryType: models.EntryTypeFolder, Token: "folder-token", MaxDownloads: &limit}
	if err := db.CreateShare(ctx, share); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateShare(ctx, &models.Share{EntryID: root.ID, EntryType: models.EntryTypeFolder, Token: share.Token}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate share token error = %v, want ErrConflict", err)
	}

	resource, err := db.ResolveSharedResource(ctx, share.Token, now)
	if err != nil || resource.Entry.Folder == nil || resource.Entry.Folder.ID != root.ID {
		t.Fatalf("resolved folder share = %#v, error = %v", resource, err)
	}
	page, err := db.ListSharedEntries(ctx, share.Token, &child.ID, ListEntriesOptions{})
	if err != nil || page.Total != 1 || page.Items[0].File == nil || page.Items[0].File.ID != inside.ID {
		t.Fatalf("shared child listing = %#v, error = %v", page, err)
	}
	if _, err := db.ListSharedEntries(ctx, share.Token, &outsideFolder.ID, ListEntriesOptions{}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("outside shared listing error = %v, want ErrAccessDenied", err)
	}
	if _, err := db.AuthorizeSharedBlobDownload(ctx, share.Token, inside.SHA256, now); err != nil {
		t.Fatalf("authorize nested blob: %v", err)
	}
	if _, err := db.AuthorizeSharedBlobDownload(ctx, share.Token, outside.SHA256, now); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("authorize outside blob error = %v, want ErrAccessDenied", err)
	}

	password := "hash"
	expires := now.Add(time.Hour)
	newLimit := uint(4)
	updated, err := db.UpdateShare(ctx, share.ID, UpdateShareInput{
		HashedPassword: Optional[*string]{Set: true, Value: &password},
		ExpiresAt:      Optional[*time.Time]{Set: true, Value: &expires},
		MaxDownloads:   Optional[*uint]{Set: true, Value: &newLimit},
	})
	if err != nil || updated.HashedPassword == nil || *updated.HashedPassword != password || updated.MaxDownloads == nil || *updated.MaxDownloads != newLimit {
		t.Fatalf("updated share = %#v, error = %v", updated, err)
	}
	listed, err := db.ListShares(ctx, 1, 10)
	if err != nil || listed.Total != 1 || listed.Items[0].ID != share.ID {
		t.Fatalf("listed shares = %#v, error = %v", listed, err)
	}

	expiredAt := now.Add(-time.Minute)
	expired := &models.Share{EntryID: inside.ID, EntryType: models.EntryTypeFile, Token: "expired", ExpiresAt: &expiredAt}
	if err := db.CreateShare(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ResolveSharedResource(ctx, expired.Token, now); !errors.Is(err, ErrShareExpired) {
		t.Fatalf("expired share error = %v, want ErrShareExpired", err)
	}
	if err := db.CreateShare(ctx, &models.Share{EntryID: 999999, EntryType: models.EntryTypeFile, Token: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("share missing entry error = %v, want ErrNotFound", err)
	}
	if err := db.DeleteShare(ctx, share.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetShareByToken(ctx, share.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted share error = %v, want ErrNotFound", err)
	}
}

func TestPostgresConcurrentDownloadLimit(t *testing.T) {
	db := newPostgresTestDB(t)
	ctx := context.Background()
	file := createFile(t, db, "limited.txt", nil, '9', 9)
	limit := uint(1)
	share := &models.Share{EntryID: file.ID, EntryType: models.EntryTypeFile, Token: "limited-token", MaxDownloads: &limit}
	if err := db.CreateShare(ctx, share); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := db.ConsumeSharedBlobDownload(ctx, share.Token, file.SHA256, time.Now().UTC())
			if err == nil {
				successes.Add(1)
				return
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if !errors.Is(err, ErrDownloadLimit) {
			t.Errorf("concurrent download error = %v, want ErrDownloadLimit", err)
		}
	}
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful downloads = %d, want 1", got)
	}
	stored, err := db.GetShare(ctx, share.ID)
	if err != nil || stored.DownloadCount != 1 {
		t.Fatalf("stored download count = %#v, error = %v", stored, err)
	}
	if _, err := db.AuthorizeSharedBlobDownload(ctx, share.Token, file.SHA256, time.Now().UTC()); !errors.Is(err, ErrDownloadLimit) {
		t.Fatalf("authorize exhausted share error = %v, want ErrDownloadLimit", err)
	}
}

func newPostgresTestDB(t *testing.T) *DB {
	t.Helper()
	if testing.Short() {
		t.Skip("PostgreSQL integration tests are disabled in short mode")
	}
	rawURL := os.Getenv(testDatabaseURLEnv)
	if rawURL == "" {
		t.Skipf("set %s to run PostgreSQL integration tests", testDatabaseURLEnv)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.TrimPrefix(parsed.Path, "/") == "" {
		t.Fatalf("%s must be a PostgreSQL URL: %q", testDatabaseURLEnv, rawURL)
	}
	query := parsed.Query()
	if query.Get("connect_timeout") == "" {
		query.Set("connect_timeout", "5")
	}
	parsed.RawQuery = query.Encode()

	admin, err := Open(parsed.String())
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := admin.Ping(ctx); err != nil {
		_ = admin.Close()
		t.Fatalf("ping PostgreSQL test database: %v", err)
	}

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		_ = admin.Close()
		t.Fatalf("generate test schema name: %v", err)
	}
	schema := "fpan_test_" + hex.EncodeToString(suffix)
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		_ = admin.Close()
		t.Fatalf("create PostgreSQL test schema %q: %v", schema, err)
	}

	var scoped *DB
	t.Cleanup(func() {
		if scoped != nil {
			if err := scoped.Close(); err != nil {
				t.Errorf("close PostgreSQL test connection: %v", err)
			}
		}
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop PostgreSQL test schema %q: %v", schema, err)
		}
		if err := admin.Close(); err != nil {
			t.Errorf("close PostgreSQL administration connection: %v", err)
		}
	})

	query = parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	scoped, err = Open(parsed.String())
	if err != nil {
		t.Fatalf("open isolated PostgreSQL test schema: %v", err)
	}
	scoped.Logger = scoped.Logger.LogMode(logger.Silent)
	if err := scoped.Ping(ctx); err != nil {
		t.Fatalf("ping isolated PostgreSQL test schema: %v", err)
	}
	if err := scoped.Migrate(); err != nil {
		t.Fatalf("migrate isolated PostgreSQL test schema: %v", err)
	}
	return scoped
}

func createFolder(t *testing.T, db *DB, display string, parentID *uint) *models.Folder {
	t.Helper()
	folder := &models.Folder{Display: display, ParentID: parentID}
	if err := db.CreateFolder(context.Background(), folder); err != nil {
		t.Fatalf("create folder %q: %v", display, err)
	}
	return folder
}

func createFile(t *testing.T, db *DB, display string, parentID *uint, digestChar byte, size int64) *models.File {
	t.Helper()
	blob := &models.Blob{SHA256: digest(digestChar), Size: size}
	file := &models.File{Display: display, MimeType: "text/plain", ParentID: parentID, SHA256: blob.SHA256}
	createFileWithBlob(t, db, file, blob)
	return file
}

func createFileWithBlob(t *testing.T, db *DB, file *models.File, blob *models.Blob) {
	t.Helper()
	if err := db.CreateFile(context.Background(), file, blob); err != nil {
		t.Fatalf("create file %q: %v", file.Display, err)
	}
}

func digest(char byte) string {
	return strings.Repeat(string(char), 64)
}
