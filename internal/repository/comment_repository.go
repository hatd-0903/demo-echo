package repository

import (
	"demo_echo/internal/models"

	"gorm.io/gorm"
)

type CommentRepository interface {
	FindByArticleID(articleID uint) ([]models.Comment, error)
	Create(comment *models.Comment) error
}

type commentRepository struct {
	db *gorm.DB
}

func NewCommentRepository(db *gorm.DB) CommentRepository {
	return &commentRepository{db: db}
}

// FindByArticleID eager-load tác giả của từng comment trong 1 query phụ (tránh N+1).
func (r *commentRepository) FindByArticleID(articleID uint) ([]models.Comment, error) {
	var comments []models.Comment
	err := r.db.Preload("User", selectProfileColumns).
		Where("article_id = ?", articleID).
		Order("created_at DESC").
		Find(&comments).Error
	return comments, err
}

func (r *commentRepository) Create(comment *models.Comment) error {
	return r.db.Omit("User", "Article").Create(comment).Error
}
