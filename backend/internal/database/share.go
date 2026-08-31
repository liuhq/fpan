package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (db *DB) CreateShare(ctx context.Context, share *models.Share) error {
	if share == nil || !share.EntryType.IsValid() || strings.TrimSpace(share.Token) == "" {
		return fmt.Errorf("create share: %w: invalid share", ErrConflict)
	}
	if share.MaxDownloads != nil && *share.MaxDownloads == 0 {
		return fmt.Errorf("create share: %w: max downloads must be positive", ErrConflict)
	}
	if share.MaxDownloads != nil && share.DownloadCount > *share.MaxDownloads {
		return fmt.Errorf("create share: %w: download count exceeds limit", ErrConflict)
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := activeEntry(tx, share.EntryType, share.EntryID); err != nil {
			return err
		}
		return tx.Create(share).Error
	})
	if err = translateError(err); err != nil {
		return fmt.Errorf("create share: %w", err)
	}
	return nil
}

func (db *DB) GetShare(ctx context.Context, id uint) (*models.Share, error) {
	var share models.Share
	if err := db.WithContext(ctx).First(&share, id).Error; err != nil {
		return nil, fmt.Errorf("get share %d: %w", id, translateError(err))
	}
	return &share, nil
}

func (db *DB) GetShareByToken(ctx context.Context, token string) (*models.Share, error) {
	var share models.Share
	if err := db.WithContext(ctx).Where("token = ?", token).First(&share).Error; err != nil {
		return nil, fmt.Errorf("get share token: %w", translateError(err))
	}
	return &share, nil
}

func (db *DB) ListShares(ctx context.Context, page, size int) (Page[models.Share], error) {
	if page < 1 || size < 1 || size > 100 {
		return Page[models.Share]{}, fmt.Errorf("list shares: %w: invalid pagination", ErrConflict)
	}
	var total int64
	if err := db.WithContext(ctx).Model(&models.Share{}).Count(&total).Error; err != nil {
		return Page[models.Share]{}, fmt.Errorf("count shares: %w", err)
	}
	var shares []models.Share
	if err := db.WithContext(ctx).Order("created_at DESC, id DESC").Limit(size).Offset((page - 1) * size).Find(&shares).Error; err != nil {
		return Page[models.Share]{}, fmt.Errorf("list shares: %w", err)
	}
	return Page[models.Share]{Items: shares, Total: total, Page: page, Size: size}, nil
}

func (db *DB) UpdateShare(ctx context.Context, id uint, patch UpdateShareInput) (*models.Share, error) {
	if patch.MaxDownloads.Set && patch.MaxDownloads.Value != nil && *patch.MaxDownloads.Value == 0 {
		return nil, fmt.Errorf("update share %d: %w: max downloads must be positive", id, ErrConflict)
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var share models.Share
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&share, id).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if patch.HashedPassword.Set {
			updates["hashed_password"] = patch.HashedPassword.Value
		}
		if patch.ExpiresAt.Set {
			updates["expires_at"] = patch.ExpiresAt.Value
		}
		if patch.MaxDownloads.Set {
			if patch.MaxDownloads.Value != nil && *patch.MaxDownloads.Value < share.DownloadCount {
				return ErrConflict
			}
			updates["max_downloads"] = patch.MaxDownloads.Value
		}
		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&share).Updates(updates).Error
	})
	if err = translateError(err); err != nil {
		return nil, fmt.Errorf("update share %d: %w", id, err)
	}
	return db.GetShare(ctx, id)
}

func (db *DB) DeleteShare(ctx context.Context, id uint) error {
	result := db.WithContext(ctx).Delete(&models.Share{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete share %d: %w", id, translateError(result.Error))
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("delete share %d: %w", id, ErrNotFound)
	}
	return nil
}

func (db *DB) ResolveSharedResource(ctx context.Context, token string, now time.Time) (*SharedResource, error) {
	var result SharedResource
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var share models.Share
		if err := tx.Where("token = ?", token).First(&share).Error; err != nil {
			return err
		}
		if share.ExpiresAt != nil && !share.ExpiresAt.After(now) {
			return ErrShareExpired
		}
		if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
			return ErrDownloadLimit
		}
		entry, err := activeEntry(tx, share.EntryType, share.EntryID)
		if err != nil {
			return err
		}
		result = SharedResource{Share: share, Entry: entry}
		return nil
	})
	if err = translateError(err); err != nil {
		return nil, fmt.Errorf("resolve shared resource: %w", err)
	}
	return &result, nil
}

func (db *DB) ListSharedEntries(ctx context.Context, token string, parentID *uint, opts ListEntriesOptions) (Page[Entry], error) {
	resource, err := db.ResolveSharedResource(ctx, token, time.Now().UTC())
	if err != nil {
		return Page[Entry]{}, err
	}
	if resource.Share.EntryType != models.EntryTypeFolder {
		return Page[Entry]{}, fmt.Errorf("list shared entries: %w", ErrAccessDenied)
	}
	listingParentID := resource.Share.EntryID
	if parentID != nil {
		var allowed bool
		err := db.WithContext(ctx).Raw(`
			WITH RECURSIVE subtree AS (
				SELECT id FROM folders WHERE id = ? AND deleted_at IS NULL
				UNION ALL
				SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
				WHERE f.deleted_at IS NULL
			)
			SELECT EXISTS(SELECT 1 FROM subtree WHERE id = ?)
		`, resource.Share.EntryID, *parentID).Scan(&allowed).Error
		if err != nil {
			return Page[Entry]{}, fmt.Errorf("list shared entries: %w", err)
		}
		if !allowed {
			return Page[Entry]{}, fmt.Errorf("list shared entries: %w", ErrAccessDenied)
		}
		listingParentID = *parentID
	}
	return db.ListEntries(ctx, &listingParentID, opts)
}

// AuthorizeSharedBlobDownload verifies that a blob belongs to an active share
// without consuming the download allowance. The caller can open the physical
// blob first and then call ConsumeSharedBlobDownload.
func (db *DB) AuthorizeSharedBlobDownload(ctx context.Context, token, sha256 string, now time.Time) (*models.Blob, error) {
	var blob models.Blob
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var share models.Share
		if err := tx.Where("token = ?", token).First(&share).Error; err != nil {
			return err
		}
		if share.ExpiresAt != nil && !share.ExpiresAt.After(now) {
			return ErrShareExpired
		}
		if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
			return ErrDownloadLimit
		}
		allowed, err := sharedBlobAllowed(tx, &share, sha256)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrAccessDenied
		}
		return tx.First(&blob, "sha256 = ?", sha256).Error
	})
	if err = translateError(err); err != nil {
		return nil, fmt.Errorf("authorize shared download: %w", err)
	}
	return &blob, nil
}

func (db *DB) ConsumeSharedBlobDownload(ctx context.Context, token, sha256 string, now time.Time) (*models.Blob, error) {
	var blob models.Blob
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var share models.Share
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token = ?", token).First(&share).Error; err != nil {
			return err
		}
		if share.ExpiresAt != nil && !share.ExpiresAt.After(now) {
			return ErrShareExpired
		}
		if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
			return ErrDownloadLimit
		}
		allowed, err := sharedBlobAllowed(tx, &share, sha256)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrAccessDenied
		}
		result := tx.Model(&models.Share{}).
			Where("id = ? AND (expires_at IS NULL OR expires_at > ?) AND (max_downloads IS NULL OR download_count < max_downloads)", share.ID, now).
			UpdateColumn("download_count", gorm.Expr("download_count + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrDownloadLimit
		}
		return tx.First(&blob, "sha256 = ?", sha256).Error
	})
	if err = translateError(err); err != nil {
		return nil, fmt.Errorf("consume shared download: %w", err)
	}
	return &blob, nil
}

func activeEntry(tx *gorm.DB, entryType models.EntryType, id uint) (Entry, error) {
	switch entryType {
	case models.EntryTypeFile:
		var file models.File
		if err := tx.Preload("Blob").First(&file, id).Error; err != nil {
			return Entry{}, err
		}
		return Entry{Type: entryType, File: &file}, nil
	case models.EntryTypeFolder:
		var folder models.Folder
		if err := tx.First(&folder, id).Error; err != nil {
			return Entry{}, err
		}
		return Entry{Type: entryType, Folder: &folder}, nil
	default:
		return Entry{}, ErrConflict
	}
}

func sharedBlobAllowed(tx *gorm.DB, share *models.Share, sha256 string) (bool, error) {
	var allowed bool
	switch share.EntryType {
	case models.EntryTypeFile:
		err := tx.Raw(`SELECT EXISTS(
			SELECT 1 FROM files WHERE id = ? AND sha256 = ? AND deleted_at IS NULL
		)`, share.EntryID, sha256).Scan(&allowed).Error
		return allowed, err
	case models.EntryTypeFolder:
		err := tx.Raw(`
			WITH RECURSIVE subtree AS (
				SELECT id FROM folders WHERE id = ? AND deleted_at IS NULL
				UNION ALL
				SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
				WHERE f.deleted_at IS NULL
			)
			SELECT EXISTS(
				SELECT 1 FROM files WHERE parent_id IN (SELECT id FROM subtree)
				AND sha256 = ? AND deleted_at IS NULL
			)
		`, share.EntryID, sha256).Scan(&allowed).Error
		return allowed, err
	default:
		return false, errors.New("invalid share entry type")
	}
}
