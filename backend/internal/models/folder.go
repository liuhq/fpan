package models

import "gorm.io/gorm"

type Folder struct {
	gorm.Model

	Display string `gorm:"not null;check:chk_folders_display_not_blank,length(trim(display)) > 0;index:uidx_folders_parent_display_active,unique,where:deleted_at IS NULL AND parent_id IS NOT NULL,priority:2;index:uidx_folders_root_display_active,unique,where:deleted_at IS NULL AND parent_id IS NULL"`

	ParentID *uint   `gorm:"index:idx_folders_parent;index:uidx_folders_parent_display_active,unique,where:deleted_at IS NULL AND parent_id IS NOT NULL,priority:1"`
	Parent   *Folder `gorm:"foreignKey:ParentID;references:ID;constraint:OnDelete:CASCADE"`
}
