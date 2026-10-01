package models

import (
	"time"

	"gorm.io/gorm"
)

type Article struct {
	ID    uint   `gorm:"primaryKey"`
	Title string `gorm:"size:255;not null"`
	Slug  string `gorm:"size:255;uniqueIndex;not null"`
	Body  string `gorm:"type:longtext"`

	// BelongsTo: article thuộc về một user (tác giả).
	AuthorID uint  `gorm:"not null;index"`
	Author   *User `gorm:"foreignKey:AuthorID;constraint:OnDelete:CASCADE"`

	// HasMany: một article có nhiều comment.
	Comments []Comment `gorm:"foreignKey:ArticleID"`

	// Many2Many: article <-> tag qua bảng trung gian article_tags.
	Tags []Tag `gorm:"many2many:article_tags"`

	// Index cho ORDER BY created_at (sắp xếp mặc định của danh sách + feed).
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
