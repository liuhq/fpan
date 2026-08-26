package models

import "time"

type Blob struct {
	SHA256    string `gorm:"primaryKey;type:char(64);check:chk_blobs_sha256_format,sha256 ~ '^[0-9a-f]{64}$'"`
	Size      int64  `gorm:"not null;check:chk_blobs_size_nonnegative,size >= 0"`
	CreatedAt time.Time
}
