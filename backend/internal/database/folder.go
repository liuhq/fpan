package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/liuhq/fpan/internal/models"
	"gorm.io/gorm"
)

func (db *DB) CreateFolder(
	ctx context.Context,
	folder *models.Folder,
) error {
	if err := gorm.G[models.Folder](db.DB).Create(ctx, folder); err != nil {
		return fmt.Errorf("create folder: %w", err)
	}

	return nil
}

func (db *DB) GetFolder(
	ctx context.Context,
	id uint,
) (*models.Folder, error) {
	folder, err := gorm.G[models.Folder](db.DB).Where("id = ?", id).First(ctx)

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("get folder %d: %w", id, err)
	}

	return &folder, nil
}

func (db *DB) ListFolders(
	ctx context.Context,
	parentID uint,
) (folders []models.Folder, err error) {
	folders, err = gorm.G[models.Folder](db.DB).Where("parent_id = ?", parentID).Order("display ASC").Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}

	return
}
