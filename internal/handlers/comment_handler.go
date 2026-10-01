package handlers

import (
	"net/http"
	"strings"

	"demo_echo/internal/dto"
	"demo_echo/internal/models"
	"demo_echo/internal/repository"

	"github.com/labstack/echo/v4"
)

type CommentHandler struct {
	repo        repository.CommentRepository
	articleRepo repository.ArticleRepository
}

func NewCommentHandler(repo repository.CommentRepository, articleRepo repository.ArticleRepository) *CommentHandler {
	return &CommentHandler{repo: repo, articleRepo: articleRepo}
}

// List godoc
// @Summary      Danh sách comment
// @Description  Mới nhất trước, có phân trang.
// @Tags         comments
// @Produce      json
// @Param        slug    path   string  true   "Slug của article"  example(hoc-golang-co-ban)
// @Param        limit   query  int     false  "Số comment mỗi trang (1-100)"  default(20)
// @Param        offset  query  int     false  "Bỏ qua bao nhiêu comment"      default(0)
// @Success      200  {object}  handlers.Response{data=dto.CommentListResponse}  "OK"
// @Failure      400  {object}  handlers.ErrorResponse  "Param không hợp lệ"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /articles/{slug}/comments [get]
func (h *CommentHandler) List(c echo.Context) error {
	limit, offset, err := parsePage(c)
	if err != nil {
		return err
	}
	article, err := h.articleRepo.FindBySlug(c.Param("slug"))
	if err != nil {
		return dbError(c, err)
	}
	comments, total, err := h.repo.FindByArticleID(article.ID, limit, offset)
	if err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewCommentListResponse(comments, total, limit, offset))
}

// Create godoc
// @Summary      Thêm comment
// @Tags         comments
// @Accept       json
// @Produce      json
// @Security     TokenAuth
// @Param        slug  path  string  true  "Slug của article"  example(hoc-golang-co-ban)
// @Param        body  body  dto.CreateCommentRequest  true  "Nội dung comment"
// @Success      201  {object}  handlers.Response{data=dto.CommentResponse}  "Created"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Failure      422  {object}  handlers.ErrorResponse  "Dữ liệu không hợp lệ"
// @Router       /articles/{slug}/comments [post]
func (h *CommentHandler) Create(c echo.Context) error {
	user, err := requireUser(c)
	if err != nil {
		return err
	}
	var req dto.CreateCommentRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid request body")
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" {
		return failure(c, http.StatusUnprocessableEntity, "body is required")
	}

	article, err := h.articleRepo.FindBySlug(c.Param("slug"))
	if err != nil {
		return dbError(c, err)
	}

	comment := &models.Comment{
		Body:      req.Body,
		ArticleID: article.ID,
		UserID:    user.ID,
	}
	if err := h.repo.Create(comment); err != nil {
		return dbError(c, err)
	}

	comment.User = user // đã có sẵn, không cần query lại
	return success(c, http.StatusCreated, dto.NewCommentResponse(comment))
}
