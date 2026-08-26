package database

import (
	"errors"

	"gorm.io/gorm"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrInvalidMove   = errors.New("invalid move")
	ErrAccessDenied  = errors.New("access denied")
	ErrShareExpired  = errors.New("share expired")
	ErrDownloadLimit = errors.New("download limit reached")
)

func translateError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey), errors.Is(err, gorm.ErrForeignKeyViolated):
		return ErrConflict
	default:
		return err
	}
}
