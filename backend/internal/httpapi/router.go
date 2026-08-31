// Package httpapi exposes the authenticated file API.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/liuhq/fpan/internal/auth"
	"github.com/liuhq/fpan/internal/database"
	"github.com/liuhq/fpan/internal/files"
	"github.com/liuhq/fpan/internal/models"
	"github.com/liuhq/fpan/internal/storage"
	"gorm.io/gorm"
)

type OIDC interface {
	LoginURL() (string, error)
	Authenticate(context.Context, string, string) error
}

type Repository interface {
	ListEntries(context.Context, *uint, database.ListEntriesOptions) (database.Page[database.Entry], error)
	GetFile(context.Context, uint) (*models.File, error)
}

type RouterConfig struct {
	Repository Repository
	Files      *files.Service
	OIDC       OIDC
	Sessions   *auth.Sessions
}

func NewRouter(config RouterConfig) (*gin.Engine, error) {
	if config.Repository == nil || config.Files == nil || config.OIDC == nil || config.Sessions == nil {
		return nil, errors.New("create http router: incomplete dependencies")
	}
	router := gin.Default()
	router.GET("/ping", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"message": "pong"}) })

	api := router.Group("/api/v1")
	api.GET("/auth/login", loginHandler(config.OIDC))
	api.GET("/auth/callback", callbackHandler(config.OIDC, config.Sessions))
	api.POST("/auth/logout", func(ctx *gin.Context) {
		auth.Logout(ctx, config.Sessions)
		ctx.Status(http.StatusNoContent)
	})

	api.Use(auth.Authentication(config.Sessions), auth.RequireAuth())
	api.GET("/entries", listEntriesHandler(config.Repository, nil))
	api.GET("/folders/:id/entries", func(ctx *gin.Context) {
		id, ok := parseID(ctx, "id")
		if !ok {
			return
		}
		listEntriesHandler(config.Repository, &id)(ctx)
	})
	api.POST("/files", multipartUploadHandler(config.Files, nil))
	api.POST("/folders/:id/files", func(ctx *gin.Context) {
		id, ok := parseID(ctx, "id")
		if !ok {
			return
		}
		multipartUploadHandler(config.Files, &id)(ctx)
	})
	api.POST("/files/stream", uploadHandler(config.Files, nil))
	api.POST("/folders/:id/files/stream", func(ctx *gin.Context) {
		id, ok := parseID(ctx, "id")
		if !ok {
			return
		}
		uploadHandler(config.Files, &id)(ctx)
	})
	api.GET("/files/:id", func(ctx *gin.Context) {
		id, ok := parseID(ctx, "id")
		if !ok {
			return
		}
		file, err := config.Repository.GetFile(ctx, id)
		if err != nil {
			writeError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, fileResponse(file))
	})
	api.GET("/blobs/:sha256", downloadHandler(config.Files))
	return router, nil
}

func loginHandler(oidc OIDC) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		url, err := oidc.LoginURL()
		if err != nil {
			writeError(ctx, err)
			return
		}
		ctx.Redirect(http.StatusFound, url)
	}
}

func callbackHandler(oidc OIDC, sessions *auth.Sessions) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		code, state := ctx.Query("code"), ctx.Query("state")
		if code == "" || state == "" {
			writeClientError(ctx, http.StatusBadRequest, "code and state are required")
			return
		}
		if err := oidc.Authenticate(ctx, code, state); err != nil {
			writeClientError(ctx, http.StatusBadRequest, "OIDC authentication failed")
			return
		}
		sessionID, err := sessions.Create()
		if err != nil {
			writeError(ctx, err)
			return
		}
		auth.SetSessionCookie(ctx, sessionID)
		ctx.Redirect(http.StatusFound, "/")
	}
}

func listEntriesHandler(repository Repository, parentID *uint) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		opts, err := listOptions(ctx)
		if err != nil {
			writeClientError(ctx, http.StatusBadRequest, err.Error())
			return
		}
		page, err := repository.ListEntries(ctx, parentID, opts)
		if err != nil {
			writeError(ctx, err)
			return
		}
		items := make([]any, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, entryResponse(item))
		}
		ctx.JSON(http.StatusOK, gin.H{"items": items, "total": page.Total, "page": page.Page, "size": page.Size})
	}
}

func uploadHandler(service *files.Service, parentID *uint) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		display := strings.TrimSpace(ctx.GetHeader("X-File-Name"))
		if display == "" {
			writeClientError(ctx, http.StatusBadRequest, "display is required")
			return
		}
		result, err := service.Upload(ctx, files.UploadInput{
			Display: display, MimeType: ctx.GetHeader("X-File-Type"), ParentID: parentID, Content: ctx.Request.Body,
		})
		if err != nil {
			writeError(ctx, err)
			return
		}
		ctx.JSON(http.StatusCreated, fileResponse(result))
	}
}

func multipartUploadHandler(service *files.Service, parentID *uint) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		header, err := ctx.FormFile("file")
		if err != nil {
			writeClientError(ctx, http.StatusBadRequest, "file is required")
			return
		}
		reader, err := header.Open()
		if err != nil {
			writeClientError(ctx, http.StatusBadRequest, "file cannot be opened")
			return
		}
		defer func() { _ = reader.Close() }()
		result, err := service.Upload(ctx, files.UploadInput{
			Display: strings.TrimSpace(header.Filename), MimeType: header.Header.Get("Content-Type"), ParentID: parentID, Content: reader,
		})
		if err != nil {
			writeError(ctx, err)
			return
		}
		ctx.JSON(http.StatusCreated, fileResponse(result))
	}
}

func downloadHandler(service *files.Service) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		digest := ctx.Param("sha256")
		blob, reader, err := service.OpenBlob(ctx, digest)
		if err != nil {
			writeError(ctx, err)
			return
		}
		defer func() { _ = reader.Close() }()
		ctx.DataFromReader(http.StatusOK, blob.Size, "application/octet-stream", reader, nil)
	}
}

func listOptions(ctx *gin.Context) (database.ListEntriesOptions, error) {
	page, err := queryInt(ctx, "page", 0)
	if err != nil {
		return database.ListEntriesOptions{}, err
	}
	size, err := queryInt(ctx, "size", 0)
	if err != nil {
		return database.ListEntriesOptions{}, err
	}
	return database.ListEntriesOptions{
		Page: page, Size: size, Sort: database.SortDirection(ctx.Query("sort")),
		SortBy: database.EntrySortField(ctx.Query("sort_by")), Filter: ctx.Query("filter"),
		Type: database.EntryTypeFilter(ctx.Query("type")),
	}, nil
}

func queryInt(ctx *gin.Context, name string, fallback int) (int, error) {
	value := ctx.Query(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
}

func parseID(ctx *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(ctx.Param(name), 10, 64)
	if err != nil || value == 0 || value > uint64(^uint(0)) {
		writeClientError(ctx, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return uint(value), true
}

func writeError(ctx *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, 5000, "internal server error"
	switch {
	case errors.Is(err, files.ErrInconsistentBlob):
		status, code, message = http.StatusInternalServerError, 5001, "blob storage is inconsistent"
	case errors.Is(err, database.ErrNotFound), errors.Is(err, storage.ErrNotFound):
		status, code, message = http.StatusNotFound, 4040, "resource not found"
	case errors.Is(err, database.ErrConflict), errors.Is(err, database.ErrInvalidMove):
		status, code, message = http.StatusConflict, 4090, "resource conflict"
	case errors.Is(err, files.ErrInvalidInput), errors.Is(err, storage.ErrInvalidSHA256):
		status, code, message = http.StatusBadRequest, 4000, "invalid request"
	case errors.Is(err, database.ErrShareExpired):
		status, code, message = http.StatusGone, 4100, "share expired"
	case errors.Is(err, database.ErrDownloadLimit):
		status, code, message = http.StatusTooManyRequests, 4290, "download limit reached"
	}
	ctx.AbortWithStatusJSON(status, gin.H{"code": code, "message": message})
}

func writeClientError(ctx *gin.Context, status int, message string) {
	ctx.AbortWithStatusJSON(status, gin.H{"code": status * 10, "message": message})
}

type blobResponse struct {
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
}

type fileResponseBody struct {
	Type      string       `json:"type"`
	ID        uint         `json:"id"`
	Display   string       `json:"display"`
	ParentID  *uint        `json:"parent_id"`
	MimeType  string       `json:"mime_type"`
	Blob      blobResponse `json:"blob"`
	CreatedAt int64        `json:"created_at"`
	UpdatedAt int64        `json:"updated_at"`
	DeletedAt *int64       `json:"deleted_at"`
}

type folderResponseBody struct {
	Type      string `json:"type"`
	ID        uint   `json:"id"`
	Display   string `json:"display"`
	ParentID  *uint  `json:"parent_id"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	DeletedAt *int64 `json:"deleted_at"`
}

func entryResponse(entry database.Entry) any {
	if entry.Type == models.EntryTypeFolder && entry.Folder != nil {
		return folderResponse(entry.Folder)
	}
	return fileResponse(entry.File)
}

func fileResponse(file *models.File) fileResponseBody {
	return fileResponseValue(file)
}

func fileResponseValue(file *models.File) fileResponseBody {
	return fileResponseBody{Type: string(models.EntryTypeFile), ID: file.ID, Display: file.Display, ParentID: file.ParentID,
		MimeType: file.MimeType, Blob: blobResponse{SHA256: file.Blob.SHA256, Size: file.Blob.Size, CreatedAt: unix(file.Blob.CreatedAt)},
		CreatedAt: unix(file.CreatedAt), UpdatedAt: unix(file.UpdatedAt), DeletedAt: optionalUnix(file.DeletedAt)}
}

func folderResponse(folder *models.Folder) folderResponseBody {
	return folderResponseValue(folder)
}

func folderResponseValue(folder *models.Folder) folderResponseBody {
	return folderResponseBody{Type: string(models.EntryTypeFolder), ID: folder.ID, Display: folder.Display, ParentID: folder.ParentID,
		CreatedAt: unix(folder.CreatedAt), UpdatedAt: unix(folder.UpdatedAt), DeletedAt: optionalUnix(folder.DeletedAt)}
}

func unix(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func optionalUnix(value gorm.DeletedAt) *int64 {
	if !value.Valid {
		return nil
	}
	result := unix(value.Time)
	return &result
}
