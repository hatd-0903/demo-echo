package database

import (
	"log"
	"time"

	"demo_echo/internal/config"
	"demo_echo/internal/models"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect mở kết nối MySQL qua GORM và cấu hình connection pool.
func Connect(cfg *config.Config) (*gorm.DB, error) {
	logLevel := logger.Info // in mọi câu SQL khi dev
	if cfg.IsProduction() {
		logLevel = logger.Warn // chỉ in query chậm / lỗi
	}

	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger:         logger.Default.LogMode(logLevel),
		TranslateError: true, // chuyển lỗi MySQL (vd. duplicate key) thành gorm.ErrDuplicatedKey
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("connected to MySQL")
	return db, nil
}

// Migrate tạo/cập nhật bảng theo các model.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Tag{},
		&models.Article{},
		&models.Comment{},
		&models.Follow{},
	)
}
