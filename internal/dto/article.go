package dto

import (
	"time"

	"demo_echo/internal/models"
)

type CreateArticleRequest struct {
	Title  string `json:"title" example:"Học Golang cơ bản"`
	Body   string `json:"body" example:"Nội dung bài viết..."`
	TagIDs []uint `json:"tag_ids" example:"1,2"`
}

type UpdateArticleRequest struct {
	Title  *string `json:"title" example:"Học Golang nâng cao"`
	Body   *string `json:"body" example:"Nội dung mới..."`
	TagIDs []uint  `json:"tag_ids" example:"1"` // gửi để thay toàn bộ tag; bỏ trống = giữ nguyên
}

type ArticleResponse struct {
	ID        uint             `json:"id" example:"1"`
	Slug      string           `json:"slug" example:"hoc-golang-co-ban"`
	Title     string           `json:"title" example:"Học Golang cơ bản"`
	Body      string           `json:"body" example:"Nội dung bài viết..."`
	Tags      []TagResponse    `json:"tags"`
	Author    *ProfileResponse `json:"author"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// NewArticleResponse cần article đã Preload("Author") và Preload("Tags").
func NewArticleResponse(a *models.Article) ArticleResponse {
	return ArticleResponse{
		ID:        a.ID,
		Slug:      a.Slug,
		Title:     a.Title,
		Body:      a.Body,
		Tags:      NewTagListResponse(a.Tags),
		Author:    NewProfileResponse(a.Author),
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

// ArticleListResponse là một trang kết quả; ArticlesCount là tổng số bài khớp filter
// (không phải số bài trong trang) để client tính số trang.
type ArticleListResponse struct {
	Articles      []ArticleResponse `json:"articles"`
	ArticlesCount int64             `json:"articles_count" example:"42"`
	Limit         int               `json:"limit" example:"20"`
	Offset        int               `json:"offset" example:"0"`
}

func NewArticleListResponse(articles []models.Article, total int64, limit, offset int) ArticleListResponse {
	return ArticleListResponse{
		Articles:      mapSlice(articles, NewArticleResponse),
		ArticlesCount: total,
		Limit:         limit,
		Offset:        offset,
	}
}
