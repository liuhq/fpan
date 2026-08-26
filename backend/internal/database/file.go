package database

import (
	"context"
	"fmt"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (db *DB) CreateFile(ctx context.Context, file *models.File, blob *models.Blob) error {
	if file == nil || blob == nil {
		return fmt.Errorf("create file: %w: nil file or blob", ErrConflict)
	}
	if err := validateDisplay(file.Display); err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	if file.SHA256 == "" {
		file.SHA256 = blob.SHA256
	}
	if err := validateBlobMetadata(blob); err != nil || file.SHA256 != blob.SHA256 {
		return fmt.Errorf("create file: %w: invalid blob metadata", ErrConflict)
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if file.ParentID != nil {
			exists, err := activeFolderExists(tx, *file.ParentID)
			if err != nil {
				return err
			}
			if !exists {
				return ErrNotFound
			}
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(blob).Error; err != nil {
			return err
		}
		var stored models.Blob
		if err := tx.First(&stored, "sha256 = ?", blob.SHA256).Error; err != nil {
			return err
		}
		if stored.Size != blob.Size {
			return ErrConflict
		}
		return tx.Omit("Blob").Create(file).Error
	})
	if err = translateError(err); err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	return nil
}

func (db *DB) GetFile(ctx context.Context, id uint) (*models.File, error) {
	var file models.File
	if err := db.WithContext(ctx).Preload("Blob").First(&file, id).Error; err != nil {
		return nil, fmt.Errorf("get file %d: %w", id, translateError(err))
	}
	return &file, nil
}

func (db *DB) UpdateFile(ctx context.Context, id uint, patch UpdateFileInput) (*models.File, error) {
	if patch.Display.Set {
		if err := validateDisplay(patch.Display.Value); err != nil {
			return nil, fmt.Errorf("update file %d: %w", id, err)
		}
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var file models.File
		if err := tx.First(&file, id).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if patch.Display.Set {
			updates["display"] = patch.Display.Value
		}
		if patch.ParentID.Set {
			if patch.ParentID.Value != nil {
				exists, err := activeFolderExists(tx, *patch.ParentID.Value)
				if err != nil {
					return err
				}
				if !exists {
					return ErrNotFound
				}
			}
			updates["parent_id"] = patch.ParentID.Value
		}
		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&file).Updates(updates).Error
	})
	if err = translateError(err); err != nil {
		return nil, fmt.Errorf("update file %d: %w", id, err)
	}
	return db.GetFile(ctx, id)
}

func (db *DB) DeleteFile(ctx context.Context, id uint) error {
	result := db.WithContext(ctx).Delete(&models.File{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete file %d: %w", id, translateError(result.Error))
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("delete file %d: %w", id, ErrNotFound)
	}
	return nil
}
