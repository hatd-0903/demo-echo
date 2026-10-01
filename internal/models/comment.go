package models

import (
	"time"

	"gorm.io/gorm"
)

type Comment struct {
	ID   uint   `gorm:"primaryKey"`
	Body string `gorm:"type:text;not null"`

	// BelongsTo: comment thuộc về một article.
	ArticleID uint     `gorm:"not null;index:idx_comments_article_created,priority:1"`
	Article   *Article `gorm:"foreignKey:ArticleID;constraint:OnDelete:CASCADE"`

	// BelongsTo: comment thuộc về một user (người bình luận).
	UserID uint  `gorm:"not null;index"`
	User   *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`

	// Index ghép (article_id, created_at): lọc comment theo bài + ORDER BY created_at
	// trong cùng một index, không phải sort thêm (filesort).
	CreatedAt time.Time `gorm:"index:idx_comments_article_created,priority:2"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
