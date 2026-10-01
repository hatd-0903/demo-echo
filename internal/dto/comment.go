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

func NewCommentListResponse(comments []models.Comment) []CommentResponse {
	return mapSlice(comments, NewCommentResponse)
}
