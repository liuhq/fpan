package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm"
)

func (db *DB) CreateFolder(ctx context.Context, folder *models.Folder) error {
	if folder == nil {
		return fmt.Errorf("create folder: %w: nil folder", ErrConflict)
	}
	if err := validateDisplay(folder.Display); err != nil {
		return fmt.Errorf("create folder: %w", err)
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if folder.ParentID != nil {
			exists, err := activeFolderExists(tx, *folder.ParentID)
			if err != nil {
				return err
			}
			if !exists {
				return ErrNotFound
			}
		}
		return tx.Create(folder).Error
	})
	if err = translateError(err); err != nil {
		return fmt.Errorf("create folder: %w", err)
	}
	return nil
}

func (db *DB) GetFolder(ctx context.Context, id uint) (*models.Folder, error) {
	var folder models.Folder
	if err := db.WithContext(ctx).First(&folder, id).Error; err != nil {
		return nil, fmt.Errorf("get folder %d: %w", id, translateError(err))
	}
	return &folder, nil
}

func (db *DB) FolderExists(ctx context.Context, id uint) (bool, error) {
	exists, err := activeFolderExists(db.WithContext(ctx), id)
	if err != nil {
		return false, fmt.Errorf("check folder %d: %w", id, err)
	}
	return exists, nil
}

func (db *DB) IsFolderDescendant(ctx context.Context, folderID, ancestorID uint) (bool, error) {
	var found bool
	err := db.WithContext(ctx).Raw(`
		WITH RECURSIVE descendants AS (
			SELECT id FROM folders WHERE parent_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT f.id FROM folders f JOIN descendants d ON f.parent_id = d.id
			WHERE f.deleted_at IS NULL
		)
		SELECT EXISTS(SELECT 1 FROM descendants WHERE id = ?)
	`, ancestorID, folderID).Scan(&found).Error
	if err != nil {
		return false, fmt.Errorf("check folder ancestry: %w", err)
	}
	return found, nil
}

func (db *DB) UpdateFolder(ctx context.Context, id uint, patch UpdateFolderInput) (*models.Folder, error) {
	if patch.Display.Set {
		if err := validateDisplay(patch.Display.Value); err != nil {
			return nil, fmt.Errorf("update folder %d: %w", id, err)
		}
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var folder models.Folder
		if err := tx.First(&folder, id).Error; err != nil {
			return err
		}
		updates := map[string]any{}
		if patch.Display.Set {
			updates["display"] = patch.Display.Value
		}
		if patch.ParentID.Set {
			if patch.ParentID.Value != nil {
				parentID := *patch.ParentID.Value
				if parentID == id {
					return ErrInvalidMove
				}
				exists, err := activeFolderExists(tx, parentID)
				if err != nil {
					return err
				}
				if !exists {
					return ErrNotFound
				}
				var descendant bool
				if err := tx.Raw(`
					WITH RECURSIVE descendants AS (
						SELECT id FROM folders WHERE parent_id = ? AND deleted_at IS NULL
						UNION ALL
						SELECT f.id FROM folders f JOIN descendants d ON f.parent_id = d.id
						WHERE f.deleted_at IS NULL
					)
					SELECT EXISTS(SELECT 1 FROM descendants WHERE id = ?)
				`, id, parentID).Scan(&descendant).Error; err != nil {
					return err
				}
				if descendant {
					return ErrInvalidMove
				}
			}
			updates["parent_id"] = patch.ParentID.Value
		}
		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&folder).Updates(updates).Error
	})
	if err = translateError(err); err != nil {
		return nil, fmt.Errorf("update folder %d: %w", id, err)
	}
	return db.GetFolder(ctx, id)
}

func (db *DB) DeleteFolder(ctx context.Context, id uint) error {
	now := time.Now().UTC()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.Folder{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
		if err := tx.Exec(`
			WITH RECURSIVE subtree AS (
				SELECT id FROM folders WHERE id = ? AND deleted_at IS NULL
				UNION ALL
				SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
				WHERE f.deleted_at IS NULL
			)
			UPDATE files SET deleted_at = ?, updated_at = ?
			WHERE parent_id IN (SELECT id FROM subtree) AND deleted_at IS NULL
		`, id, now, now).Error; err != nil {
			return err
		}
		result := tx.Exec(`
			WITH RECURSIVE subtree AS (
				SELECT id FROM folders WHERE id = ? AND deleted_at IS NULL
				UNION ALL
				SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
				WHERE f.deleted_at IS NULL
			)
			UPDATE folders SET deleted_at = ?, updated_at = ?
			WHERE id IN (SELECT id FROM subtree) AND deleted_at IS NULL
		`, id, now, now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err = translateError(err); err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("delete folder %d: %w", id, ErrNotFound)
		}
		return fmt.Errorf("delete folder %d: %w", id, err)
	}
	return nil
}
