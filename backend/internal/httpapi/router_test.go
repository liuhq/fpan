package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), `"code":4010`) {
		t.Fatalf("response = %d %s, want JSON 401", recorder.Code, recorder.Body.String())
	}
}

func TestHealthAndReadiness(t *testing.T) {
	router, _, _, _ := newTestRouter(t)

	responses := map[string]string{
		"/healthz": `{"status":"ok"}`,
		"/readyz":  `{"status":"ready"}`,
	}
	for path, wantBody := range responses {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != wantBody {
			t.Fatalf("%s response = %d %s, want 200 %s", path, recorder.Code, recorder.Body.String(), wantBody)
		}
	}

	router, _, _, _ = newTestRouterWithReady(t, errors.New("database unavailable"))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable || strings.TrimSpace(recorder.Body.String()) != `{"code":5030,"message":"service not ready"}` {
		t.Fatalf("unready response = %d %s", recorder.Code, recorder.Body.String())
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
	cookies := recorder.Result().Cookies()
	if oidc.code != "abc" || oidc.state != "state" || len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("callback did not authenticate and set a cookie: %#v", oidc)
	}
}

func TestMockOIDCLoginCreatesDevelopmentSession(t *testing.T) {
	router, _, _ := newTestRouterWithOIDC(t, nil, auth.NewMockOIDC(), false)

	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	if login.Code != http.StatusFound {
		t.Fatalf("mock login status = %d: %s", login.Code, login.Body.String())
	}
	callbackLocation := login.Header().Get("Location")
	if !strings.HasPrefix(callbackLocation, "/api/v1/auth/callback?") {
		t.Fatalf("mock login location = %q", callbackLocation)
	}

	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, callbackLocation, nil))
	cookies := callback.Result().Cookies()
	if callback.Code != http.StatusFound || callback.Header().Get("Location") != "/" || len(cookies) != 1 {
		t.Fatalf("mock callback response = %d, location %q, cookies %#v", callback.Code, callback.Header().Get("Location"), cookies)
	}
	if cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("mock session cookie = %#v", cookies[0])
	}

	protected := httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil)
	protected.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	router.ServeHTTP(response, protected)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated mock request status = %d: %s", response.Code, response.Body.String())
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

func TestFolderCRUD(t *testing.T) {
	router, _, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/folders", strings.NewReader(`{"display":"docs"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"display":"docs"`) {
		t.Fatalf("create folder response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/v1/folders/1", strings.NewReader(`{"display":"documents","parent_id":null}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"display":"documents"`) {
		t.Fatalf("update folder response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/folders/1", nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get folder status = %d: %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/folders/1", nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete folder status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestFileMetadataCRUD(t *testing.T) {
	router, _, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/files/stream", strings.NewReader("content"))
	request.Header.Set("X-File-Name", "old.txt")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var uploaded fileResponseBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPut, "/api/v1/files/"+strconv.FormatUint(uint64(uploaded.ID), 10), strings.NewReader(`{"display":"new.txt","parent_id":null}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"display":"new.txt"`) {
		t.Fatalf("update file response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+strconv.FormatUint(uint64(uploaded.ID), 10), nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete file status = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUpdateRejectsMalformedParentID(t *testing.T) {
	router, _, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/files/1", strings.NewReader(`{"parent_id":0}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUpdateRejectsEmptyBody(t *testing.T) {
	router, _, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)
	for _, path := range []string{"/api/v1/files/1", "/api/v1/folders/1", "/api/v1/shares/1"} {
		request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(session)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s response = %d %s, want 400", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestInvalidEntriesQueryReturnsBadRequest(t *testing.T) {
	router, _, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)
	queries := []string{
		"page=0",
		"page=-1",
		"page=invalid",
		"size=101",
		"sort=sideways",
		"sort_by=size",
		"type=blob",
	}
	for _, query := range queries {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/entries?"+query, nil)
		request.AddCookie(session)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":4000`) {
			t.Fatalf("query %q response = %d %s, want JSON 400", query, recorder.Code, recorder.Body.String())
		}
	}
}

func TestTrashRoutes(t *testing.T) {
	router, repository, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)
	deletedAt := time.Unix(1700000300, 0).UTC()
	repository.trash = []database.Entry{
		{Type: models.EntryTypeFile, File: &models.File{Model: gorm.Model{ID: 7, DeletedAt: gorm.DeletedAt{Time: deletedAt, Valid: true}}, Display: "deleted.txt"}},
		{Type: models.EntryTypeFolder, Folder: &models.Folder{Model: gorm.Model{ID: 8, DeletedAt: gorm.DeletedAt{Time: deletedAt, Valid: true}}, Display: "deleted-folder"}},
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/trash", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"display":"deleted.txt"`) {
		t.Fatalf("list trash response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/trash/file/7/restore", nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("restore trash response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/trash/folder/8", nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("purge trash response = %d %s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/trash", nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || len(repository.trash) != 0 {
		t.Fatalf("empty trash response = %d %s, remaining = %d", recorder.Code, recorder.Body.String(), len(repository.trash))
	}
}

func TestTrashRoutesRejectInvalidTargetAndReportConflict(t *testing.T) {
	router, repository, _, sessions := newTestRouter(t)
	session := authenticatedSession(t, sessions)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/trash/invalid/1/restore", nil)
	request.AddCookie(session)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid type response = %d %s", recorder.Code, recorder.Body.String())
	}

	repository.trashErr = database.ErrConflict
	request = httptest.NewRequest(http.MethodPost, "/api/v1/trash/file/1/restore", nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("restore conflict response = %d %s", recorder.Code, recorder.Body.String())
	}

	repository.trashErr = database.ErrNotFound
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/trash/file/1", nil)
	request.AddCookie(session)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("purge missing response = %d %s", recorder.Code, recorder.Body.String())
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
	return newTestRouterWithReady(t, nil)
}

func newTestRouterWithReady(t *testing.T, readyErr error) (http.Handler, *testRepository, *fakeOIDC, *auth.Sessions) {
	t.Helper()
	oidc := &fakeOIDC{}
	router, repository, sessions := newTestRouterWithOIDC(t, readyErr, oidc, true)
	return router, repository, oidc, sessions
}

func newTestRouterWithOIDC(t *testing.T, readyErr error, oidc OIDC, secureCookies bool) (http.Handler, *testRepository, *auth.Sessions) {
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
	sessions := auth.NewSessions()
	router, err := NewRouter(RouterConfig{
		Repository: repository, Files: fileService, Shares: shareService, OIDC: oidc,
		Sessions: sessions, SecureCookies: secureCookies, Ready: func(context.Context) error { return readyErr },
	})
	if err != nil {
		t.Fatal(err)
	}
	return router, repository, sessions
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
	trash      []database.Entry
	trashErr   error
}

func newTestRepository() *testRepository {
	return &testRepository{files: make(map[uint]models.File), folders: make(map[uint]models.Folder), shares: make(map[uint]models.Share), blobs: make(map[string]models.Blob), references: make(map[string]int)}
}

func (r *testRepository) ListEntries(_ context.Context, _ *uint, _ database.ListEntriesOptions) (database.Page[database.Entry], error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.page, nil
}

func (r *testRepository) ListTrash(_ context.Context) ([]database.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trashErr != nil {
		return nil, r.trashErr
	}
	return append([]database.Entry(nil), r.trash...), nil
}

func (r *testRepository) Restore(_ context.Context, entryType models.EntryType, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trashErr != nil {
		return r.trashErr
	}
	r.removeTrash(entryType, id)
	return nil
}

func (r *testRepository) Purge(_ context.Context, entryType models.EntryType, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trashErr != nil {
		return r.trashErr
	}
	r.removeTrash(entryType, id)
	return nil
}

func (r *testRepository) EmptyTrash(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trashErr != nil {
		return r.trashErr
	}
	r.trash = nil
	return nil
}

func (r *testRepository) removeTrash(entryType models.EntryType, id uint) {
	for index, entry := range r.trash {
		entryID := uint(0)
		if entry.Type == models.EntryTypeFile && entry.File != nil {
			entryID = entry.File.ID
		}
		if entry.Type == models.EntryTypeFolder && entry.Folder != nil {
			entryID = entry.Folder.ID
		}
		if entry.Type == entryType && entryID == id {
			r.trash = append(r.trash[:index], r.trash[index+1:]...)
			return
		}
	}
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

func (r *testRepository) CreateFolder(_ context.Context, folder *models.Folder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	folder.ID = r.nextID
	folder.CreatedAt = time.Now().UTC()
	folder.UpdatedAt = folder.CreatedAt
	r.folders[folder.ID] = *folder
	return nil
}

func (r *testRepository) GetFolder(_ context.Context, id uint) (*models.Folder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	folder, ok := r.folders[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	return &folder, nil
}

func (r *testRepository) UpdateFolder(_ context.Context, id uint, patch database.UpdateFolderInput) (*models.Folder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	folder, ok := r.folders[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	if patch.Display.Set {
		folder.Display = patch.Display.Value
	}
	if patch.ParentID.Set {
		folder.ParentID = patch.ParentID.Value
	}
	folder.UpdatedAt = time.Now().UTC()
	r.folders[id] = folder
	return &folder, nil
}

func (r *testRepository) DeleteFolder(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.folders[id]; !ok {
		return database.ErrNotFound
	}
	delete(r.folders, id)
	return nil
}

func (r *testRepository) UpdateFile(_ context.Context, id uint, patch database.UpdateFileInput) (*models.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	file, ok := r.files[id]
	if !ok {
		return nil, database.ErrNotFound
	}
	if patch.Display.Set {
		file.Display = patch.Display.Value
	}
	if patch.ParentID.Set {
		file.ParentID = patch.ParentID.Value
	}
	file.UpdatedAt = time.Now().UTC()
	r.files[id] = file
	return &file, nil
}

func (r *testRepository) DeleteFile(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.files[id]; !ok {
		return database.ErrNotFound
	}
	delete(r.files, id)
	return nil
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
