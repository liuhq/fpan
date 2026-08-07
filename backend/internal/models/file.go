package models

import (
	"gorm.io/gorm"
)

type File struct {
	gorm.Model
	Display  string `gorm:"not null;check:chk_display_not_blank,length(trim(display)) > 0"`
	ParentID *uint  `gorm:"index"`
	MimeType string `gorm:"not null"`
	SHA256   string `gorm:"type:varchar(64);not null;index"`
	Blob     Blob   `gorm:"foreignKey:SHA256;references:SHA256;constraint:OnDelete:RESTRICT"`
}
