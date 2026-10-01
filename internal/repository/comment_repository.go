package repository

import (
	"demo_echo/internal/models"

	"gorm.io/gorm"
)

type CommentRepository interface {
	// FindByArticleID trả về một trang comment (mới nhất trước) và tổng số comment của bài.
	FindByArticleID(articleID uint, limit, offset int) ([]models.Comment, int64, error)
	Create(comment *models.Comment) error
}

type commentRepository struct {
	db *gorm.DB
}

func NewCommentRepository(db *gorm.DB) CommentRepository {
	return &commentRepository{db: db}
}

// FindByArticleID eager-load tác giả của từng comment trong 1 query phụ (tránh N+1).
// ORDER BY created_at, id dùng được index ghép (article_id, created_at).
func (r *commentRepository) FindByArticleID(articleID uint, limit, offset int) ([]models.Comment, int64, error) {
	comments := []models.Comment{}
	err := r.db.Preload("User", selectProfileColumns).
		Where("article_id = ?", articleID).
		Order("created_at DESC").
		Order("id DESC").
		Limit(limit).
		Offset(offset).
		Find(&comments).Error
	if err != nil {
		return nil, 0, err
	}
	// Như article: trang chưa đầy và không rỗng thì đã biết tổng, bỏ được COUNT(*).
	if n := len(comments); n > 0 && n < limit {
		return comments, int64(offset + n), nil
	}
	var total int64
	err = r.db.Model(&models.Comment{}).Where("article_id = ?", articleID).Count(&total).Error
	return comments, total, err
}

func (r *commentRepository) Create(comment *models.Comment) error {
	return r.db.Omit("User", "Article").Create(comment).Error
}
