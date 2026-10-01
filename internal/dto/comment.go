package dto

import (
	"time"

	"demo_echo/internal/models"
)

type CreateCommentRequest struct {
	Body string `json:"body" example:"Bài viết hay quá!"`
}

type CommentResponse struct {
	ID        uint             `json:"id" example:"1"`
	Body      string           `json:"body" example:"Bài viết hay quá!"`
	Author    *ProfileResponse `json:"author"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// NewCommentResponse cần comment đã Preload("User").
func NewCommentResponse(c *models.Comment) CommentResponse {
	return CommentResponse{
		ID:        c.ID,
		Body:      c.Body,
		Author:    NewProfileResponse(c.User),
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// CommentListResponse là một trang comment; CommentsCount là tổng số comment của bài.
type CommentListResponse struct {
	Comments      []CommentResponse `json:"comments"`
	CommentsCount int64             `json:"comments_count" example:"3"`
	Limit         int               `json:"limit" example:"20"`
	Offset        int               `json:"offset" example:"0"`
}

func NewCommentListResponse(comments []models.Comment, total int64, limit, offset int) CommentListResponse {
	return CommentListResponse{
		Comments:      mapSlice(comments, NewCommentResponse),
		CommentsCount: total,
		Limit:         limit,
		Offset:        offset,
	}
}
