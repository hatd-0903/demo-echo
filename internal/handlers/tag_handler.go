package handlers

import (
	"net/http"
	"strings"

	"demo_echo/internal/dto"
	"demo_echo/internal/models"
	"demo_echo/internal/repository"

	"github.com/labstack/echo/v4"
)

type TagHandler struct {
	repo repository.TagRepository
}

func NewTagHandler(repo repository.TagRepository) *TagHandler {
	return &TagHandler{repo: repo}
}

// List godoc
// @Summary      Danh sách tag
// @Description  Sắp xếp theo tên. Response được cache (header X-Cache).
// @Tags         tags
// @Produce      json
// @Success      200  {object}  handlers.Response{data=[]dto.TagResponse}  "OK"
// @Header       200  {string}  X-Cache  "HIT | MISS | BYPASS"
// @Router       /tags [get]
func (h *TagHandler) List(c echo.Context) error {
	tags, err := h.repo.FindAll()
	if err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewTagListResponse(tags))
}

// Get godoc
// @Summary      Chi tiết tag
// @Tags         tags
// @Produce      json
// @Param        id    path  int     true  "ID tag"
// @Success      200  {object}  handlers.Response{data=dto.TagResponse}  "OK"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /tags/{id} [get]
func (h *TagHandler) Get(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	tag, err := h.repo.FindByID(id)
	if err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewTagResponse(tag))
}

// Create godoc
// @Summary      Tạo tag
// @Tags         tags
// @Accept       json
// @Produce      json
// @Security     TokenAuth
// @Param        body  body  dto.TagRequest  true  "Tên tag"
// @Success      201  {object}  handlers.Response{data=dto.TagResponse}  "Created"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      409  {object}  handlers.ErrorResponse  "Đã tồn tại"
// @Failure      422  {object}  handlers.ErrorResponse  "Dữ liệu không hợp lệ"
// @Router       /tags [post]
func (h *TagHandler) Create(c echo.Context) error {
	var req dto.TagRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid request body")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return failure(c, http.StatusUnprocessableEntity, "name is required")
	}

	tag := &models.Tag{Name: name}
	if err := h.repo.Create(tag); err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusCreated, dto.NewTagResponse(tag))
}

// Update godoc
// @Summary      Đổi tên tag (admin)
// @Description  Chỉ admin: tag dùng chung cho mọi article.
// @Tags         tags
// @Accept       json
// @Produce      json
// @Security     TokenAuth
// @Param        id    path  int     true  "ID tag"
// @Param        body  body  dto.TagRequest  true  "Tên mới"
// @Success      200  {object}  handlers.Response{data=dto.TagResponse}  "OK"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      403  {object}  handlers.ErrorResponse  "Không phải admin"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Failure      409  {object}  handlers.ErrorResponse  "Đã tồn tại"
// @Failure      422  {object}  handlers.ErrorResponse  "Dữ liệu không hợp lệ"
// @Router       /tags/{id} [put]
func (h *TagHandler) Update(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	var req dto.TagRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid request body")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return failure(c, http.StatusUnprocessableEntity, "name is required")
	}

	tag, err := h.repo.FindByID(id)
	if err != nil {
		return dbError(c, err)
	}
	tag.Name = name
	if err := h.repo.Update(tag); err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewTagResponse(tag))
}

// Delete godoc
// @Summary      Xoá tag (admin)
// @Description  Chỉ admin. Tag cũng bị gỡ khỏi mọi article.
// @Tags         tags
// @Security     TokenAuth
// @Param        id    path  int     true  "ID tag"
// @Success      204  "No Content"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      403  {object}  handlers.ErrorResponse  "Không phải admin"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /tags/{id} [delete]
func (h *TagHandler) Delete(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	if _, err := h.repo.FindByID(id); err != nil {
		return dbError(c, err)
	}
	if err := h.repo.Delete(id); err != nil {
		return dbError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
