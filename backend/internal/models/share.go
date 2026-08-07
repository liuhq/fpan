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

type SharePerm uint8

const (
	SharePermRead  = 0
	SharePermWrite = 1
)

type Share struct {
	ID             uint      `gorm:"primaryKey;autoIncrement"`
	EntryID        uint      `gorm:"not null;index"`
	EntryType      EntryType `gorm:"type:varchar(20);not null;check:chk_entry_type,entry_type IN ('file','folder')"`
	Token          string    `gorm:"not null;uniqueIndex"`
	HashedPassword *string
	ExpiresAt      *time.Time
	Permission     SharePerm `gorm:"not null;default:0"`
	MaxDownloads   *uint
	DownloadCount  uint `gorm:"not null;default:0"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
