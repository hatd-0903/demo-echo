package models

import (
	"time"

	"gorm.io/gorm"
)

// User là model lưu trữ; dữ liệu trả ra API phải đi qua DTO (internal/dto).
type User struct {
	ID       uint   `gorm:"primaryKey"`
	Username string `gorm:"size:100;uniqueIndex;not null"`
	Email    string `gorm:"size:255;uniqueIndex;not null"`
	Password string `gorm:"size:255;not null" json:"-"`
	Bio      string `gorm:"type:text"`
	Image    string `gorm:"size:500"`

	// HasMany: một user viết nhiều article (articles.author_id -> users.id).
	Articles []Article `gorm:"foreignKey:AuthorID"`
	// HasMany: một user viết nhiều comment (comments.user_id -> users.id).
	Comments []Comment `gorm:"foreignKey:UserID"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
