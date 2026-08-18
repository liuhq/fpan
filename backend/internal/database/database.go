package database

import (
	"fmt"

	M "github.com/liuhq/fpan/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type FpanDb struct {
	dsn string
	*gorm.DB
}

func ConnectFpanDb(dsn string) (*FpanDb, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})

	return &FpanDb{dsn: dsn, DB: db}, err
}

func (f *FpanDb) Migrate() error {
	errs := []error{}

	err := f.DB.AutoMigrate(&M.Blob{})
	if err != nil {
		errs = append(errs, err)
	}

	err = f.DB.AutoMigrate(&M.File{})
	if err != nil {
		errs = append(errs, err)
	}

	err = f.DB.AutoMigrate(&M.Folder{})
	if err != nil {
		errs = append(errs, err)
	}

	err = f.DB.AutoMigrate(&M.Share{})
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return fmt.Errorf("errors from automigrating database:\n\t%v", errs)
	}

	return nil
}
