package database

import (
	"fmt"
	"strings"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm"
)

const sha256Length = 64

func activeFolderExists(tx *gorm.DB, id uint) (bool, error) {
	var count int64
	if err := tx.Table("folders").Where("id = ? AND deleted_at IS NULL", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func validateDisplay(display string) error {
	if strings.TrimSpace(display) == "" {
		return fmt.Errorf("%w: display must not be blank", ErrInvalidInput)
	}
	return nil
}

func validateBlobMetadata(blob *models.Blob) error {
	if blob == nil || len(blob.SHA256) != sha256Length || blob.Size < 0 {
		return fmt.Errorf("%w: invalid blob metadata", ErrConflict)
	}
	for _, char := range blob.SHA256 {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return fmt.Errorf("%w: invalid blob metadata", ErrConflict)
		}
	}
	return nil
}

func normalizeListOptions(opts ListEntriesOptions) (ListEntriesOptions, error) {
	if opts.Page == 0 {
		opts.Page = 1
	}
	if opts.Size == 0 {
		opts.Size = 100
	}
	if opts.Sort == "" {
		opts.Sort = SortAscending
	}
	if opts.SortBy == "" {
		opts.SortBy = EntrySortName
	}
	if opts.Type == "" {
		opts.Type = EntryTypeAll
	}
	if opts.Page < 1 || opts.Size < 1 || opts.Size > 100 {
		return opts, fmt.Errorf("%w: invalid pagination", ErrInvalidInput)
	}
	if opts.Sort != SortAscending && opts.Sort != SortDescending {
		return opts, fmt.Errorf("%w: invalid sort direction", ErrInvalidInput)
	}
	if opts.SortBy != EntrySortName && opts.SortBy != EntrySortCreatedAt && opts.SortBy != EntrySortUpdatedAt {
		return opts, fmt.Errorf("%w: invalid sort field", ErrInvalidInput)
	}
	if opts.Type != EntryTypeAll && opts.Type != EntryTypeFile && opts.Type != EntryTypeFolder {
		return opts, fmt.Errorf("%w: invalid entry type", ErrInvalidInput)
	}
	return opts, nil
}
