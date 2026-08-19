package models

import (
	"gorm.io/gorm"
)

type File struct {
	gorm.Model

	Display  string `gorm:"not null;check:chk_files_display_not_blank,length(trim(display)) > 0"`
	MimeType string `gorm:"not null"`

	ParentID *uint
	Parent   *Folder `gorm:"foreignKey:ParentID;references:ID;constraint:OnDelete:CASCADE"`

	SHA256 string `gorm:"type:char(64);not null;index"`
	Blob   Blob   `gorm:"foreignKey:SHA256;references:SHA256;constraint:OnDelete:RESTRICT"`
}
