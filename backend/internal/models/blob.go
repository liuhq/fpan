package models

import "time"

type Blob struct {
	SHA256    string `gorm:"primaryKey;type:varchar(64)"`
	Size      uint   `gorm:"not null"`
	CreatedAt time.Time
}
