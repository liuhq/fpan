package models

import (
	"gorm.io/gorm"
)

type File struct {
	gorm.Model

	Display string `gorm:"not null;check:chk_files_display_not_blank,length(trim(display)) > 0;index:uidx_files_parent_display_active,unique,where:deleted_at IS NULL AND parent_id IS NOT NULL,priority:2;index:uidx_files_root_display_active,unique,where:deleted_at IS NULL AND parent_id IS NULL"`

	MimeType string `gorm:"not null"`

	ParentID *uint   `gorm:"index:idx_files_parent;index:uidx_files_parent_display_active,unique,where:deleted_at IS NULL AND parent_id IS NOT NULL,priority:1"`
	Parent   *Folder `gorm:"belongsTo:Parent;foreignKey:ParentID;references:ID;constraint:OnDelete:CASCADE"`

	SHA256 string `gorm:"type:char(64);not null;index"`
	Blob   Blob   `gorm:"belongsTo:Blob;foreignKey:SHA256;references:SHA256;constraint:OnDelete:RESTRICT"`
}
