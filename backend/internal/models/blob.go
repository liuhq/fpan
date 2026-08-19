package models

import "time"

type Blob struct {
	SHA256    string `gorm:"primaryKey;type:char(64)"`
	Size      int64  `gorm:"not null"`
	CreatedAt time.Time
}
