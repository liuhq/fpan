package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/liuhq/fpan/internal/auth"
	"github.com/liuhq/fpan/internal/database"
	"github.com/liuhq/fpan/internal/files"
	"github.com/liuhq/fpan/internal/models"
	"github.com/liuhq/fpan/internal/shares"
	"github.com/liuhq/fpan/internal/storage/filesystem"
	"gorm.io/gorm"
)

func TestProtectedRoutesRequireSession(t *testing.T) {
	router, _, _, _ := newTestRouter(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestOIDCLoginCallbackCreatesSession(t *testing.T) {
	router, _, oidc, _ := newTestRouter(t)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "https://issuer.example/authorize" {
		t.Fatalf("login response = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=abc&state=state", nil))
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/" {
		t.Fatalf("callback response = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
	if oidc.code != "abc" || oidc.state != "state" || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("callback did not authenticate and set a cookie: %#v", oidc)
	}
}

func TestEntriesAndStreamUpload(t *testing.T) {
	router, repository, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)

	created := time.Unix(1700000000, 0).UTC()
	repository.page = database.Page[database.Entry]{
		Items: []database.Entry{{Type: models.EntryTypeFolder, Folder: &models.Folder{Model: modelAt(9, created), Display: "docs"}}},
		Total: 1, Page: 1, Size: 20,
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"created_at":1700000000`) {
		t.Fatalf("entries response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/files/stream", strings.NewReader("hello"))
	request.Header.Set("X-File-Name", "hello.txt")
	request.Header.Set("X-File-Type", "text/plain")
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"display":"hello.txt"`) {
		t.Fatalf("upload response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestBlobDownload(t *testing.T) {
	router, _, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)
	content := []byte("download me")
	digestBytes := sha256.Sum256(content)
	digest := hex.EncodeToString(digestBytes[:])
	upload := httptest.NewRequest(http.MethodPost, "/api/v1/files/stream", bytes.NewReader(content))
	upload.Header.Set("X-File-Name", "download.txt")
	upload.AddCookie(session)
	uploadRecorder := httptest.NewRecorder()
	router.ServeHTTP(uploadRecorder, upload)
	if uploadRecorder.Code != http.StatusCreated {
		t.Fatalf("upload status = %d: %s", uploadRecorder.Code, uploadRecorder.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/"+digest, nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != string(content) {
		t.Fatalf("download response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestShareManagementAndPublicDownload(t *testing.T) {
	router, repository, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/files/stream", strings.NewReader("shared content"))
	request.Header.Set("X-File-Name", "shared.txt")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var file fileResponseBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(`{"entry_id":1,"entry_type":"file","password":"secret","max_downloads":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create share status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var share shareResponseBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &share); err != nil {
		t.Fatal(err)
	}
	if share.Token == "" || !share.HasPassword {
		t.Fatalf("unexpected share response: %#v", share)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/s/"+share.Token+"?password=wrong", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("wrong password status = %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/s/"+share.Token+"?password=secret", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "hashed_password") {
		t.Fatalf("shared access response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/s/"+share.Token+"/blobs/"+file.Blob.SHA256+"?password=secret", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "shared content" {
		t.Fatalf("shared download response = %d %q", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/s/"+share.Token+"/blobs/"+file.Blob.SHA256+"?password=secret", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("exhausted download status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if repository.shares[share.ID].DownloadCount != 1 {
		t.Fatalf("download count = %d, want 1", repository.shares[share.ID].DownloadCount)
	}
}

func authenticatedSession(t *testing.T, sessions *auth.Sessions) *http.Cookie {
	t.Helper()
	id, err := sessions.Create()
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "fpan_session", Value: id}
}

func newTestRouter(t *testing.T) (http.Handler, *testRepository, *fakeOIDC, *auth.Sessions) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repository := newTestRepository()
	store, err := filesystem.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fileService, err := files.New(repository, store)
	if err != nil {
		t.Fatal(err)
	}
	shareService, err := shares.New(repository, fileService)
	if err != nil {
		t.Fatal(err)
	}
	oidc := &fakeOIDC{}
	// The router owns the session store, so tests use a pre-created session
	// from this same store through the helper below.
	sessions := auth.NewSessions()
	router, err := NewRouter(RouterConfig{Repository: repository, Files: fileService, Shares: shareService, OIDC: oidc, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	return router, repository, oidc, sessions
}

type fakeOIDC struct {
	code  string
	state string
}

func (o *fakeOIDC) LoginURL() (string, error) { return "https://issuer.example/authorize", nil }
func (o *fakeOIDC) Authenticate(_ context.Context, code, state string) error {
	o.code, o.state = code, state
	return nil
}

type testRepository struct {
	mu         sync.Mutex
	page       database.Page[database.Entry]
	files      map[uint]models.File
	folders    map[uint]models.Folder
	shares     map[uint]models.Share
	blobs      map[string]models.Blob
	references map[string]int
	nextID     uint
}

func newTestRepository() *testRepository {
	return &testRepository{files: make(map[uint]models.File), folders: make(map[uint]models.Folder), shares: make(map[uint]models.Share), blobs: make(map[string]models.Blob), references: make(map[string]int)}
}

func (r *testRepository) ListEntries(_ context.Context, _ *uint, _ database.ListEntriesOptions) (database.Page[database.Entry], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.page, nil
}

func (r *testRepository) CreateFile(_ context.Context, file *models.File, blob *models.Blob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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

func (r *testRepository) GetFile(_ context.Context, id uint) (*models.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &file, nil
}

func (r *testRepository) CreateShare(_ context.Context, share *models.Share) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if share.ID == 0 {
		share.ID = uint(len(r.shares) + 1)
	}
	if share.CreatedAt.IsZero() {
		share.CreatedAt = time.Now().UTC()
	}
	share.UpdatedAt = share.CreatedAt
	r.shares[share.ID] = *share
	return nil
}

func (r *testRepository) GetShare(_ context.Context, id uint) (*models.Share, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	share, ok := r.shares[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &share, nil
}

func (r *testRepository) ListShares(_ context.Context, page, size int) (database.Page[models.Share], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]models.Share, 0, len(r.shares))
	for _, share := range r.shares {
		items = append(items, share)
	}
	return database.Page[models.Share]{Items: items, Total: int64(len(items)), Page: page, Size: size}, nil
}

func (r *testRepository) UpdateShare(_ context.Context, id uint, patch database.UpdateShareInput) (*models.Share, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	share, ok := r.shares[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	if patch.HashedPassword.Set {
		share.HashedPassword = patch.HashedPassword.Value
	}
	if patch.ExpiresAt.Set {
		share.ExpiresAt = patch.ExpiresAt.Value
	}
	if patch.MaxDownloads.Set {
		share.MaxDownloads = patch.MaxDownloads.Value
	}
	share.UpdatedAt = time.Now().UTC()
	r.shares[id] = share
	return &share, nil
}

func (r *testRepository) DeleteShare(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.shares[id]; !ok {
		return database.ErrNotFound
	}
	delete(r.shares, id)
	return nil
}

func (r *testRepository) ResolveSharedResource(_ context.Context, token string, _ time.Time) (*database.SharedResource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, share := range r.shares {
		if share.Token != token {
			continue
		}
		if share.EntryType == models.EntryTypeFile {
			file, ok := r.files[share.EntryID]
			if !ok {
				return nil, database.ErrNotFound
			}
			return &database.SharedResource{Share: share, Entry: database.Entry{Type: share.EntryType, File: &file}}, nil
		}
		folder, ok := r.folders[share.EntryID]
		if !ok {
			return nil, database.ErrNotFound
		}
		return &database.SharedResource{Share: share, Entry: database.Entry{Type: share.EntryType, Folder: &folder}}, nil
	}
	return nil, database.ErrNotFound
}

func (r *testRepository) ListSharedEntries(_ context.Context, _ string, _ *uint, _ database.ListEntriesOptions) (database.Page[database.Entry], error) {
	return r.page, nil
}

func (r *testRepository) AuthorizeSharedBlobDownload(_ context.Context, token, digest string, _ time.Time) (*models.Blob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, share := range r.shares {
		if share.Token == token {
			if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
				return nil, database.ErrDownloadLimit
			}
			break
		}
	}
	blob, ok := r.blobs[digest]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &blob, nil
}

func (r *testRepository) ConsumeSharedBlobDownload(_ context.Context, token, digest string, _ time.Time) (*models.Blob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, share := range r.shares {
		if share.Token == token {
			if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
				return nil, database.ErrDownloadLimit
			}
			share.DownloadCount++
			r.shares[id] = share
			break
		}
	}
	blob, ok := r.blobs[digest]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &blob, nil
}

func (r *testRepository) GetBlob(_ context.Context, digest string) (*models.Blob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	blob, ok := r.blobs[digest]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &blob, nil
}

func (r *testRepository) BlobExists(_ context.Context, digest string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.blobs[digest]
	return ok, nil
}

func (r *testRepository) ListUnreferencedBlobs(_ context.Context, _ int) ([]models.Blob, error) {
	return nil, nil
}

func (r *testRepository) DeleteBlobIfUnreferenced(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func modelAt(id uint, created time.Time) gorm.Model {
	return gorm.Model{ID: id, CreatedAt: created, UpdatedAt: created}
}
