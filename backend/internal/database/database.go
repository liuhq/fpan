package database

import (
	"context"
	"fmt"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type DB struct {
	*gorm.DB
}

func Open(dsn string) (*DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	return &DB{DB: db}, err
}

func (db *DB) Migrate() error {
	if err := db.AutoMigrate(
		&models.Blob{},
		&models.Folder{},
		&models.File{},
		&models.Share{},
	); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	return nil
}

func (db *DB) Ping(ctx context.Context) error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}
	return sqlDB.Close()
}
