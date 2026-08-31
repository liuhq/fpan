package models

import (
	"time"
)

type EntryType string

const (
	EntryTypeFile   EntryType = "file"
	EntryTypeFolder EntryType = "folder"
)

func (t EntryType) IsValid() bool {
	return t == EntryTypeFile || t == EntryTypeFolder
}

type Share struct {
	ID             uint      `gorm:"primaryKey;autoIncrement"`
	EntryID        uint      `gorm:"not null;index"`
	EntryType      EntryType `gorm:"type:varchar(20);not null;check:chk_shares_entry_type,entry_type IN ('file','folder')"`
	Token          string    `gorm:"not null;uniqueIndex"`
	HashedPassword *string
	ExpiresAt      *time.Time `gorm:"index"`
	MaxDownloads   *uint      `gorm:"check:chk_shares_download_limit,max_downloads IS NULL OR (max_downloads > 0 AND download_count <= max_downloads)"`
	DownloadCount  uint       `gorm:"not null;default:0"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
