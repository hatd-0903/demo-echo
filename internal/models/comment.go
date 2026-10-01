package models

import (
	"time"

	"gorm.io/gorm"
)

type Comment struct {
	ID   uint   `gorm:"primaryKey"`
	Body string `gorm:"type:text;not null"`

	// BelongsTo: comment thuộc về một article. FK + CASCADE khai báo ở Article.Comments.
	ArticleID uint     `gorm:"not null;index:idx_comments_article_created,priority:1"`
	Article   *Article `gorm:"foreignKey:ArticleID"`

	// BelongsTo: comment thuộc về một user. FK + CASCADE khai báo ở User.Comments.
	UserID uint  `gorm:"not null;index"`
	User   *User `gorm:"foreignKey:UserID"`

	// Index ghép (article_id, created_at): lọc comment theo bài + ORDER BY created_at
	// trong cùng một index, không phải sort thêm (filesort).
	CreatedAt time.Time `gorm:"index:idx_comments_article_created,priority:2"`
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}
