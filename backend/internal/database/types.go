package database

import (
	"time"

	"github.com/liuhq/fpan/internal/models"
)

type Optional[T any] struct {
	Set   bool
	Value T
}

type Page[T any] struct {
	Items []T
	Total int64
	Page  int
	Size  int
}

type Entry struct {
	Type   models.EntryType
	File   *models.File
	Folder *models.Folder
}

type SortDirection string

const (
	SortAscending  SortDirection = "asc"
	SortDescending SortDirection = "desc"
)

type EntrySortField string

const (
	EntrySortName      EntrySortField = "name"
	EntrySortCreatedAt EntrySortField = "created_at"
	EntrySortUpdatedAt EntrySortField = "updated_at"
)

type EntryTypeFilter string

const (
	EntryTypeAll    EntryTypeFilter = "all"
	EntryTypeFile   EntryTypeFilter = "file"
	EntryTypeFolder EntryTypeFilter = "folder"
)

type ListEntriesOptions struct {
	Page   int
	Size   int
	Sort   SortDirection
	SortBy EntrySortField
	Filter string
	Type   EntryTypeFilter
}

type UpdateFileInput struct {
	Display  Optional[string]
	ParentID Optional[*uint]
}

type UpdateFolderInput struct {
	Display  Optional[string]
	ParentID Optional[*uint]
}

type UpdateShareInput struct {
	HashedPassword Optional[*string]
	ExpiresAt      Optional[*time.Time]
	Permission     Optional[models.SharePerm]
	MaxDownloads   Optional[*uint]
}

type SharedResource struct {
	Share models.Share
	Entry Entry
}
