package models

import "gorm.io/gorm"

type Folder struct {
	gorm.Model

	Display string `gorm:"not null;check:chk_folders_display_not_blank,length(trim(display)) > 0"`

	ParentID *uint
	Parent   *Folder `gorm:"foreignKey:ParentID;references:ID;constraint:OnDelete:CASCADE"`
}
