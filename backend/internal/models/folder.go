package models

import "gorm.io/gorm"

type Folder struct {
	gorm.Model
	Display  string `gorm:"not null;check:chk_display_not_blank,length(trim(display)) > 0"`
	ParentID *uint  `gorm:"index"`
}
