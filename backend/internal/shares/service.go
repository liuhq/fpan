// Package shares contains share-link business logic.
package shares

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/liuhq/fpan/internal/database"
	fileService "github.com/liuhq/fpan/internal/files"
	"github.com/liuhq/fpan/internal/models"
	"github.com/liuhq/fpan/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

const tokenBytes = 32

type Repository interface {
	CreateShare(context.Context, *models.Share) error
	GetShare(context.Context, uint) (*models.Share, error)
	ListShares(context.Context, int, int) (database.Page[models.Share], error)
	UpdateShare(context.Context, uint, database.UpdateShareInput) (*models.Share, error)
	DeleteShare(context.Context, uint) error
	ResolveSharedResource(context.Context, string, time.Time) (*database.SharedResource, error)
	ListSharedEntries(context.Context, string, *uint, database.ListEntriesOptions) (database.Page[database.Entry], error)
	AuthorizeSharedBlobDownload(context.Context, string, string, time.Time) (*models.Blob, error)
	ConsumeSharedBlobDownload(context.Context, string, string, time.Time) (*models.Blob, error)
}

type BlobOpener interface {
	OpenBlob(context.Context, string) (*models.Blob, io.ReadCloser, error)
}

type Service struct {
	repository Repository
	blobs      BlobOpener
}

type CreateInput struct {
	EntryID      uint
	EntryType    models.EntryType
	Password     *string
	ExpiresAt    *time.Time
	MaxDownloads *uint
}

type UpdateInput struct {
	Password     database.Optional[*string]
	ExpiresAt    database.Optional[*time.Time]
	MaxDownloads database.Optional[*uint]
}

func New(repository Repository, blobs BlobOpener) (*Service, error) {
	if repository == nil || blobs == nil {
		return nil, fmt.Errorf("create share service: %w", database.ErrInvalidInput)
	}
	return &Service{repository: repository, blobs: blobs}, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*models.Share, error) {
	if input.EntryID == 0 || !input.EntryType.IsValid() {
		return nil, fmt.Errorf("create share: %w", database.ErrInvalidInput)
	}
	if err := validateOptions(input.Password, input.ExpiresAt, input.MaxDownloads); err != nil {
		return nil, fmt.Errorf("create share: %w", err)
	}

	hashedPassword, err := hashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("create share: %w", err)
	}
	share := &models.Share{
		EntryID: input.EntryID, EntryType: input.EntryType, HashedPassword: hashedPassword,
		ExpiresAt: input.ExpiresAt, MaxDownloads: input.MaxDownloads,
	}
	for range 3 {
		share.Token, err = newToken()
		if err != nil {
			return nil, fmt.Errorf("create share: %w", err)
		}
		if err = s.repository.CreateShare(ctx, share); err == nil {
			return s.repository.GetShare(ctx, share.ID)
		}
	}
	return nil, fmt.Errorf("create share: %w", err)
}

func (s *Service) Get(ctx context.Context, id uint) (*models.Share, error) {
	return s.repository.GetShare(ctx, id)
}

func (s *Service) List(ctx context.Context, page, size int) (database.Page[models.Share], error) {
	return s.repository.ListShares(ctx, page, size)
}

func (s *Service) Update(ctx context.Context, id uint, input UpdateInput) (*models.Share, error) {
	if input.ExpiresAt.Set && input.ExpiresAt.Value != nil && !input.ExpiresAt.Value.After(time.Now().UTC()) {
		return nil, fmt.Errorf("update share: %w: expiration must be in the future", database.ErrInvalidInput)
	}
	if input.MaxDownloads.Set && input.MaxDownloads.Value != nil && *input.MaxDownloads.Value == 0 {
		return nil, fmt.Errorf("update share: %w: max downloads must be positive", database.ErrInvalidInput)
	}
	patch := database.UpdateShareInput{
		ExpiresAt: input.ExpiresAt, MaxDownloads: input.MaxDownloads,
	}
	if input.Password.Set {
		hashed, err := hashPassword(input.Password.Value)
		if err != nil {
			return nil, fmt.Errorf("update share: %w", err)
		}
		patch.HashedPassword = database.Optional[*string]{Set: true, Value: hashed}
	}
	return s.repository.UpdateShare(ctx, id, patch)
}

func (s *Service) Delete(ctx context.Context, id uint) error {
	return s.repository.DeleteShare(ctx, id)
}

func (s *Service) Access(ctx context.Context, token, password string) (*database.SharedResource, error) {
	if err := validateToken(token); err != nil {
		return nil, err
	}
	resource, err := s.repository.ResolveSharedResource(ctx, token, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if resource.Share.HashedPassword != nil && bcrypt.CompareHashAndPassword([]byte(*resource.Share.HashedPassword), []byte(password)) != nil {
		return nil, database.ErrAccessDenied
	}
	return resource, nil
}

func (s *Service) ListEntries(ctx context.Context, token, password string, parentID *uint, opts database.ListEntriesOptions) (database.Page[database.Entry], error) {
	if _, err := s.Access(ctx, token, password); err != nil {
		return database.Page[database.Entry]{}, err
	}
	return s.repository.ListSharedEntries(ctx, token, parentID, opts)
}

func (s *Service) OpenBlob(ctx context.Context, token, password, digest string) (*models.Blob, io.ReadCloser, error) {
	if err := storage.ValidateSHA256(digest); err != nil {
		return nil, nil, err
	}
	if _, err := s.Access(ctx, token, password); err != nil {
		return nil, nil, err
	}
	if _, err := s.repository.AuthorizeSharedBlobDownload(ctx, token, digest, time.Now().UTC()); err != nil {
		return nil, nil, err
	}
	blob, reader, err := s.blobs.OpenBlob(ctx, digest)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil, fmt.Errorf("open shared blob: %w", fileService.ErrInconsistentBlob)
	}
	if err != nil {
		return nil, nil, err
	}
	consumed, err := s.repository.ConsumeSharedBlobDownload(ctx, token, digest, time.Now().UTC())
	if err != nil {
		_ = reader.Close()
		return nil, nil, err
	}
	if consumed != nil {
		blob = consumed
	}
	return blob, reader, nil
}

func hashPassword(password *string) (*string, error) {
	if password == nil {
		return nil, nil
	}
	if *password == "" {
		return nil, database.ErrInvalidInput
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	value := string(hashed)
	return &value, nil
}

func validateOptions(password *string, expiresAt *time.Time, maxDownloads *uint) error {
	if password != nil && *password == "" {
		return database.ErrInvalidInput
	}
	if expiresAt != nil && !expiresAt.After(time.Now().UTC()) {
		return fmt.Errorf("%w: expiration must be in the future", database.ErrInvalidInput)
	}
	if maxDownloads != nil && *maxDownloads == 0 {
		return fmt.Errorf("%w: max downloads must be positive", database.ErrInvalidInput)
	}
	return nil
}

func validateToken(token string) error {
	if len(token) < 8 || strings.TrimSpace(token) != token {
		return fmt.Errorf("%w: invalid share token", database.ErrInvalidInput)
	}
	return nil
}

func newToken() (string, error) {
	data := make([]byte, tokenBytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
