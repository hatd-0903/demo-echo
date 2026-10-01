package dto

import "demo_echo/internal/models"

type TagRequest struct {
	Name string `json:"name" example:"golang"`
}

type TagResponse struct {
	ID   uint   `json:"id" example:"1"`
	Name string `json:"name" example:"golang"`
}

func NewTagResponse(t *models.Tag) TagResponse {
	return TagResponse{ID: t.ID, Name: t.Name}
}

func NewTagListResponse(tags []models.Tag) []TagResponse {
	return mapSlice(tags, NewTagResponse)
}
