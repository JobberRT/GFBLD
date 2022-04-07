package database

import (
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	sqlLogger "gorm.io/gorm/logger"
)

func NewDB(path string) *gorm.DB {
	var db *gorm.DB
	var err error

	if db, err = gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: sqlLogger.Default.LogMode(sqlLogger.Silent),
	}); err != nil {
		logrus.WithError(err).Panic("failed to create sqlite db")
	}
	if err = db.AutoMigrate(&Video{}); err != nil {
		logrus.WithError(err).Panic("failed to migrate Video table")
	}
	if err = db.AutoMigrate(&Audio{}); err != nil {
		logrus.WithError(err).Panic("failed to migrate Audio table")
	}
	if err = db.AutoMigrate(&LiveRecord{}); err != nil {
		logrus.WithError(err).Panic("failed to migrate LiveRecord table")
	}
	return db
}
