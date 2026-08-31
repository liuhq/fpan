package database

import (
	"context"
	"fmt"
	"time"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm"
)

type trashRow struct {
	EntryType models.EntryType
	ID        uint
	Display   string
	ParentID  *uint
	MimeType  *string
	SHA256    *string
	BlobSize  *int64
	BlobAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// ListTrash returns deleted entries whose immediate parent is still active.
// Descendants of a deleted folder are represented by that folder only.
func (db *DB) ListTrash(ctx context.Context) ([]Entry, error) {
	entries, err := listTrashTx(db.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list trash: %w", err)
	}
	return entries, nil
}

// Restore clears the deletion marker on an entry and, for folders, its whole
// deleted subtree. The operation is atomic and never overwrites an active
// same-type entry.
func (db *DB) Restore(ctx context.Context, entryType models.EntryType, id uint) error {
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		switch entryType {
		case models.EntryTypeFile:
			return restoreFileTx(tx, id)
		case models.EntryTypeFolder:
			return restoreFolderTx(tx, id)
		default:
			return ErrInvalidInput
		}
	})
	if err != nil {
		return fmt.Errorf("restore %s %d: %w", entryType, id, translateError(err))
	}
	return nil
}

// Purge permanently removes an entry and its descendants from the database.
// Physical blobs are intentionally left for the garbage collector.
func (db *DB) Purge(ctx context.Context, entryType models.EntryType, id uint) error {
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return purgeTx(tx, entryType, id)
	})
	if err != nil {
		return fmt.Errorf("purge %s %d: %w", entryType, id, translateError(err))
	}
	return nil
}

// EmptyTrash permanently removes every top-level deleted entry in one
// database transaction. Physical blobs remain available to garbage
// collection after their final metadata reference is gone.
func (db *DB) EmptyTrash(ctx context.Context) error {
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		entries, err := listTrashTx(tx)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := purgeTx(tx, entry.Type, entryID(entry)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("empty trash: %w", translateError(err))
	}
	return nil
}

func listTrashTx(tx *gorm.DB) ([]Entry, error) {
	query := `
		SELECT 'file'::text AS entry_type, f.id, f.display, f.parent_id,
			f.mime_type, f.sha256, b.size AS blob_size, b.created_at AS blob_at,
			f.created_at, f.updated_at, f.deleted_at
		FROM files f LEFT JOIN blobs b ON b.sha256 = f.sha256
		WHERE f.deleted_at IS NOT NULL
			AND (f.parent_id IS NULL OR EXISTS (
				SELECT 1 FROM folders p WHERE p.id = f.parent_id AND p.deleted_at IS NULL
			))
		UNION ALL
		SELECT 'folder'::text AS entry_type, f.id, f.display, f.parent_id,
			NULL::text AS mime_type, NULL::char(64) AS sha256,
			NULL::bigint AS blob_size, NULL::timestamptz AS blob_at,
			f.created_at, f.updated_at, f.deleted_at
		FROM folders f
		WHERE f.deleted_at IS NOT NULL
			AND (f.parent_id IS NULL OR EXISTS (
				SELECT 1 FROM folders p WHERE p.id = f.parent_id AND p.deleted_at IS NULL
			))
		ORDER BY deleted_at DESC, entry_type ASC, id ASC`

	var rows []trashRow
	if err := tx.Raw(query).Scan(&rows).Error; err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		deletedAt := gorm.DeletedAt{}
		if row.DeletedAt != nil {
			deletedAt = gorm.DeletedAt{Time: *row.DeletedAt, Valid: true}
		}
		if row.EntryType == models.EntryTypeFolder {
			entries = append(entries, Entry{Type: row.EntryType, Folder: &models.Folder{
				Model:   gorm.Model{ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: deletedAt},
				Display: row.Display, ParentID: row.ParentID,
			}})
			continue
		}
		file := &models.File{
			Model:   gorm.Model{ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: deletedAt},
			Display: row.Display, ParentID: row.ParentID,
		}
		if row.MimeType != nil {
			file.MimeType = *row.MimeType
		}
		if row.SHA256 != nil {
			file.SHA256 = *row.SHA256
			file.Blob.SHA256 = *row.SHA256
		}
		if row.BlobSize != nil {
			file.Blob.Size = *row.BlobSize
		}
		if row.BlobAt != nil {
			file.Blob.CreatedAt = *row.BlobAt
		}
		entries = append(entries, Entry{Type: row.EntryType, File: file})
	}
	return entries, nil
}

func restoreFileTx(tx *gorm.DB, id uint) error {
	var file models.File
	if err := tx.Unscoped().First(&file, id).Error; err != nil {
		return err
	}
	if !file.DeletedAt.Valid {
		return ErrConflict
	}
	if err := assertActiveParent(tx, file.ParentID); err != nil {
		return err
	}
	return tx.Unscoped().Model(&file).Updates(map[string]any{
		"deleted_at": nil,
		"updated_at": time.Now().UTC(),
	}).Error
}

func restoreFolderTx(tx *gorm.DB, id uint) error {
	var folder models.Folder
	if err := tx.Unscoped().First(&folder, id).Error; err != nil {
		return err
	}
	if !folder.DeletedAt.Valid {
		return ErrConflict
	}
	if err := assertActiveParent(tx, folder.ParentID); err != nil {
		return err
	}

	now := time.Now().UTC()
	if err := tx.Exec(`
		WITH RECURSIVE subtree AS (
			SELECT id FROM folders WHERE id = ? AND deleted_at IS NOT NULL
			UNION ALL
			SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
			WHERE f.deleted_at IS NOT NULL
		)
		UPDATE files SET deleted_at = NULL, updated_at = ?
		WHERE deleted_at IS NOT NULL AND parent_id IN (SELECT id FROM subtree)
	`, id, now).Error; err != nil {
		return err
	}
	return tx.Exec(`
		WITH RECURSIVE subtree AS (
			SELECT id FROM folders WHERE id = ? AND deleted_at IS NOT NULL
			UNION ALL
			SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
			WHERE f.deleted_at IS NOT NULL
		)
		UPDATE folders SET deleted_at = NULL, updated_at = ?
		WHERE id IN (SELECT id FROM subtree)
	`, id, now).Error
}

func assertActiveParent(tx *gorm.DB, parentID *uint) error {
	if parentID == nil {
		return nil
	}
	exists, err := activeFolderExists(tx, *parentID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrConflict
	}
	return nil
}

func purgeTx(tx *gorm.DB, entryType models.EntryType, id uint) error {
	switch entryType {
	case models.EntryTypeFile:
		var file models.File
		if err := tx.Unscoped().First(&file, id).Error; err != nil {
			return err
		}
		if !file.DeletedAt.Valid {
			return ErrConflict
		}
		return tx.Unscoped().Delete(&file).Error
	case models.EntryTypeFolder:
		var folder models.Folder
		if err := tx.Unscoped().First(&folder, id).Error; err != nil {
			return err
		}
		if !folder.DeletedAt.Valid {
			return ErrConflict
		}
		var ids []uint
		if err := tx.Raw(`
			WITH RECURSIVE subtree AS (
				SELECT id FROM folders WHERE id = ?
				UNION ALL
				SELECT f.id FROM folders f JOIN subtree s ON f.parent_id = s.id
			)
			SELECT id FROM subtree
		`, id).Scan(&ids).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("deleted_at IS NOT NULL AND parent_id IN ?", ids).Delete(&models.File{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("deleted_at IS NOT NULL AND id IN ?", ids).Delete(&models.Folder{}).Error
	default:
		return ErrInvalidInput
	}
}

func entryID(entry Entry) uint {
	if entry.Type == models.EntryTypeFolder && entry.Folder != nil {
		return entry.Folder.ID
	}
	if entry.File != nil {
		return entry.File.ID
	}
	return 0
}
