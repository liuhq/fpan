package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm"
)

type entryRow struct {
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
}

func (db *DB) ListEntries(ctx context.Context, parentID *uint, opts ListEntriesOptions) (Page[Entry], error) {
	opts, err := normalizeListOptions(opts)
	if err != nil {
		return Page[Entry]{}, fmt.Errorf("list entries: %w", err)
	}
	tx := db.WithContext(ctx)
	if parentID != nil {
		exists, err := activeFolderExists(tx, *parentID)
		if err != nil {
			return Page[Entry]{}, fmt.Errorf("list entries: %w", err)
		}
		if !exists {
			return Page[Entry]{}, fmt.Errorf("list entries: %w", ErrNotFound)
		}
	}

	parentClause := "parent_id IS NULL"
	args := make([]any, 0, 4)
	if parentID != nil {
		parentClause = "parent_id = ?"
		args = append(args, *parentID)
	}
	filterClause := ""
	if opts.Filter != "" {
		filterClause = " AND display ILIKE ?"
		args = append(args, "%"+escapeLike(opts.Filter)+"%")
	}

	parts := make([]string, 0, 2)
	if opts.Type == EntryTypeAll || opts.Type == EntryTypeFile {
		parts = append(parts, `SELECT 'file'::text AS entry_type, f.id, f.display, f.parent_id,
			f.mime_type, f.sha256, b.size AS blob_size, b.created_at AS blob_at,
			f.created_at, f.updated_at
			FROM files f JOIN blobs b ON b.sha256 = f.sha256
			WHERE f.deleted_at IS NULL AND f.`+parentClause+strings.Replace(filterClause, "display", "f.display", 1))
	}
	if opts.Type == EntryTypeAll || opts.Type == EntryTypeFolder {
		parts = append(parts, `SELECT 'folder'::text AS entry_type, d.id, d.display, d.parent_id,
			NULL::text AS mime_type, NULL::char(64) AS sha256, NULL::bigint AS blob_size,
			NULL::timestamptz AS blob_at, d.created_at, d.updated_at
			FROM folders d
			WHERE d.deleted_at IS NULL AND d.`+parentClause+strings.Replace(filterClause, "display", "d.display", 1))
	}
	union := strings.Join(parts, " UNION ALL ")
	queryArgs := duplicateArgs(args, len(parts))

	var total int64
	if err := tx.Raw("SELECT COUNT(*) FROM ("+union+") entries", queryArgs...).Scan(&total).Error; err != nil {
		return Page[Entry]{}, fmt.Errorf("count entries: %w", err)
	}
	orderColumn := map[EntrySortField]string{
		EntrySortName: "display", EntrySortCreatedAt: "created_at", EntrySortUpdatedAt: "updated_at",
	}[opts.SortBy]
	orderDirection := map[SortDirection]string{SortAscending: "ASC", SortDescending: "DESC"}[opts.Sort]
	query := "SELECT * FROM (" + union + ") entries ORDER BY " + orderColumn + " " + orderDirection + ", entry_type ASC, id ASC LIMIT ? OFFSET ?"
	queryArgs = append(queryArgs, opts.Size, (opts.Page-1)*opts.Size)
	var rows []entryRow
	if err := tx.Raw(query, queryArgs...).Scan(&rows).Error; err != nil {
		return Page[Entry]{}, fmt.Errorf("list entries: %w", err)
	}

	items := make([]Entry, 0, len(rows))
	for _, row := range rows {
		if row.EntryType == models.EntryTypeFolder {
			items = append(items, Entry{Type: row.EntryType, Folder: &models.Folder{
				Model:   gorm.Model{ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt},
				Display: row.Display, ParentID: row.ParentID,
			}})
			continue
		}
		file := &models.File{
			Model:   gorm.Model{ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt},
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
		items = append(items, Entry{Type: row.EntryType, File: file})
	}
	return Page[Entry]{Items: items, Total: total, Page: opts.Page, Size: opts.Size}, nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func duplicateArgs(args []any, copies int) []any {
	result := make([]any, 0, len(args)*copies)
	for range copies {
		result = append(result, args...)
	}
	return result
}
