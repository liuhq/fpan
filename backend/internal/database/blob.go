package database

import (
	"context"
	"fmt"

	"github.com/liuhq/fpan/internal/models"
)

func (db *DB) GetBlob(ctx context.Context, sha256 string) (*models.Blob, error) {
	var blob models.Blob
	if err := db.WithContext(ctx).First(&blob, "sha256 = ?", sha256).Error; err != nil {
		return nil, fmt.Errorf("get blob %q: %w", sha256, translateError(err))
	}
	return &blob, nil
}

func (db *DB) BlobExists(ctx context.Context, sha256 string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&models.Blob{}).Where("sha256 = ?", sha256).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check blob %q: %w", sha256, err)
	}
	return count > 0, nil
}

func (db *DB) ListUnreferencedBlobs(ctx context.Context, limit int) ([]models.Blob, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list unreferenced blobs: %w: invalid limit", ErrConflict)
	}
	var blobs []models.Blob
	err := db.WithContext(ctx).Raw(`
		SELECT b.* FROM blobs b
		WHERE NOT EXISTS (SELECT 1 FROM files f WHERE f.sha256 = b.sha256)
		ORDER BY b.created_at ASC LIMIT ?
	`, limit).Scan(&blobs).Error
	if err != nil {
		return nil, fmt.Errorf("list unreferenced blobs: %w", err)
	}
	return blobs, nil
}

func (db *DB) DeleteBlobIfUnreferenced(ctx context.Context, sha256 string) (bool, error) {
	result := db.WithContext(ctx).Exec(`
		DELETE FROM blobs b WHERE b.sha256 = ?
		AND NOT EXISTS (SELECT 1 FROM files f WHERE f.sha256 = b.sha256)
	`, sha256)
	if result.Error != nil {
		return false, fmt.Errorf("delete unreferenced blob %q: %w", sha256, translateError(result.Error))
	}
	return result.RowsAffected > 0, nil
}
