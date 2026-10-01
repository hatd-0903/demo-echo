package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"demo_echo/internal/dto"
	"demo_echo/internal/middleware"
	"demo_echo/internal/models"
	"demo_echo/internal/repository"

	"github.com/labstack/echo/v4"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

type ArticleHandler struct {
	repo     repository.ArticleRepository
	userRepo repository.UserRepository
	tagRepo  repository.TagRepository
}

func NewArticleHandler(repo repository.ArticleRepository, userRepo repository.UserRepository, tagRepo repository.TagRepository) *ArticleHandler {
	return &ArticleHandler{repo: repo, userRepo: userRepo, tagRepo: tagRepo}
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// slugify bỏ dấu (kể cả tiếng Việt) và chuyển title thành dạng "hello-world".
func slugify(title string) string {
	title = strings.NewReplacer("đ", "d", "Đ", "D").Replace(title)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if plain, _, err := transform.String(t, title); err == nil {
		title = plain
	}
	s := strings.Trim(nonSlugChars.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if s == "" {
		s = "article"
	}
	return s
}

// reservedSlugs trùng với route tĩnh dưới /api/articles nên không được dùng làm slug.
var reservedSlugs = map[string]bool{"feed": true}

// uniqueSlug thêm hậu tố -2, -3, ... nếu slug đã tồn tại hoặc bị reserve.
// Lấy mọi slug có thể trùng trong 1 query rồi chọn trong bộ nhớ.
func (h *ArticleHandler) uniqueSlug(title string) (string, error) {
	base := slugify(title)
	existing, err := h.repo.SlugsLike(base)
	if err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(existing))
	for _, s := range existing {
		taken[s] = true
	}

	slug := base
	for i := 2; taken[slug] || reservedSlugs[slug]; i++ {
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return slug, nil
}

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

// List godoc
// @Summary      Danh sách article (global feed)
// @Description  Lọc theo tag/tác giả, sắp xếp, phân trang. Response được cache (header X-Cache: HIT/MISS/BYPASS).
// @Tags         articles
// @Produce      json
// @Param        tag     query  string  false  "Lọc theo tên tag"         example(golang)
// @Param        author  query  string  false  "Lọc theo username tác giả"  example(alice)
// @Param        limit   query  int     false  "Số bài mỗi trang (1-100)"  default(20)
// @Param        offset  query  int     false  "Bỏ qua bao nhiêu bài"      default(0)
// @Param        sort    query  string  false  "Cột sắp xếp, tiền tố - = giảm dần"  Enums(created_at, -created_at, updated_at, -updated_at, title, -title)  default(-created_at)
// @Success      200  {object}  handlers.Response{data=dto.ArticleListResponse}  "OK"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Header       200  {string}  X-Cache  "HIT | MISS | BYPASS"
// @Router       /articles [get]
func (h *ArticleHandler) List(c echo.Context) error {
	filter, err := parseArticleFilter(c)
	if err != nil {
		return err
	}
	filter.Tag = strings.TrimSpace(c.QueryParam("tag"))
	filter.Author = strings.TrimSpace(c.QueryParam("author"))
	return h.listArticles(c, filter)
}

// Feed godoc
// @Summary      Feed cá nhân
// @Description  Bài viết của những người bạn đang follow. Không cache.
// @Tags         articles
// @Produce      json
// @Security     TokenAuth
// @Param        limit   query  int     false  "Số bài mỗi trang (1-100)"  default(20)
// @Param        offset  query  int     false  "Bỏ qua bao nhiêu bài"      default(0)
// @Param        sort    query  string  false  "Cột sắp xếp, tiền tố - = giảm dần"  Enums(created_at, -created_at, updated_at, -updated_at, title, -title)  default(-created_at)
// @Success      200  {object}  handlers.Response{data=dto.ArticleListResponse}  "OK"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Router       /articles/feed [get]
func (h *ArticleHandler) Feed(c echo.Context) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	filter, err := parseArticleFilter(c)
	if err != nil {
		return err
	}
	filter.FollowedBy = userID
	return h.listArticles(c, filter)
}

func (h *ArticleHandler) listArticles(c echo.Context, filter repository.ArticleFilter) error {
	articles, total, err := h.repo.List(filter)
	if err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewArticleListResponse(articles, total, filter.Limit, filter.Offset))
}

// parseArticleFilter đọc và validate các query param chung: limit, offset, sort.
// sort nhận "created_at", "-created_at" (dấu "-" = giảm dần), "updated_at", "title"...
func parseArticleFilter(c echo.Context) (repository.ArticleFilter, error) {
	f := repository.ArticleFilter{Limit: defaultPageLimit, SortBy: "created_at", Desc: true}
	sort := "-created_at"

	if err := echo.QueryParamsBinder(c).
		Int("limit", &f.Limit).
		Int("offset", &f.Offset).
		String("sort", &sort).
		BindError(); err != nil {
		return f, echo.NewHTTPError(http.StatusBadRequest, "limit and offset must be integers")
	}

	if f.Limit < 1 {
		return f, echo.NewHTTPError(http.StatusBadRequest, "limit must be greater than 0")
	}
	if f.Limit > maxPageLimit {
		f.Limit = maxPageLimit
	}
	if f.Offset < 0 {
		return f, echo.NewHTTPError(http.StatusBadRequest, "offset must not be negative")
	}

	f.Desc = strings.HasPrefix(sort, "-")
	f.SortBy = strings.TrimPrefix(sort, "-")
	if _, ok := repository.ArticleSortColumns[f.SortBy]; !ok {
		return f, echo.NewHTTPError(http.StatusBadRequest, "sort must be one of: created_at, updated_at, title (prefix with - for descending)")
	}
	return f, nil
}

// LoadOwner là OwnerLoader cho middleware.RequireOwner: tải article theo :slug.
func (h *ArticleHandler) LoadOwner(c echo.Context) (any, uint, error) {
	article, err := h.repo.FindBySlug(c.Param("slug"))
	if err != nil {
		return nil, 0, dbError(c, err)
	}
	return article, article.AuthorID, nil
}

// Get godoc
// @Summary      Chi tiết article
// @Tags         articles
// @Produce      json
// @Param        slug  path  string  true  "Slug của article"  example(hoc-golang-co-ban)
// @Success      200  {object}  handlers.Response{data=dto.ArticleResponse}  "OK"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /articles/{slug} [get]
func (h *ArticleHandler) Get(c echo.Context) error {
	article, err := h.repo.FindDetailBySlug(c.Param("slug"))
	if err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewArticleResponse(article))
}

// Create godoc
// @Summary      Đăng bài
// @Description  Tác giả lấy từ JWT. Slug sinh từ title (bỏ dấu tiếng Việt), trùng thì thêm -2, -3...
// @Tags         articles
// @Accept       json
// @Produce      json
// @Security     TokenAuth
// @Param        body  body  dto.CreateArticleRequest  true  "Nội dung bài viết"
// @Success      201  {object}  handlers.Response{data=dto.ArticleResponse}  "Created"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      422  {object}  handlers.ErrorResponse  "Dữ liệu không hợp lệ"
// @Router       /articles [post]
func (h *ArticleHandler) Create(c echo.Context) error {
	authorID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req dto.CreateArticleRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid request body")
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		return failure(c, http.StatusUnprocessableEntity, "title is required")
	}
	author, err := h.userRepo.FindByID(authorID)
	if err != nil {
		return failure(c, http.StatusUnauthorized, "user no longer exists")
	}
	tags, err := h.tagRepo.FindByIDs(req.TagIDs)
	if err != nil {
		return dbError(c, err)
	}

	slug, err := h.uniqueSlug(req.Title)
	if err != nil {
		return dbError(c, err)
	}

	article := &models.Article{
		Title:    req.Title,
		Slug:     slug,
		Body:     req.Body,
		AuthorID: authorID,
		Tags:     tags,
	}
	if err := h.repo.Create(article); err != nil {
		return dbError(c, err)
	}

	// Author và tags đã có sẵn trong bộ nhớ: không cần query lại để dựng response.
	article.Author = author
	return success(c, http.StatusCreated, dto.NewArticleResponse(article))
}

// Update godoc
// @Summary      Sửa bài
// @Description  Chỉ tác giả được sửa. Đổi title thì slug đổi theo. Gửi tag_ids để thay toàn bộ tag.
// @Tags         articles
// @Accept       json
// @Produce      json
// @Security     TokenAuth
// @Param        slug  path  string  true  "Slug của article"  example(hoc-golang-co-ban)
// @Param        body  body  dto.UpdateArticleRequest  true  "Các field cần sửa"
// @Success      200  {object}  handlers.Response{data=dto.ArticleResponse}  "OK"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      403  {object}  handlers.ErrorResponse  "Không phải chủ sở hữu"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Failure      422  {object}  handlers.ErrorResponse  "Dữ liệu không hợp lệ"
// @Router       /articles/{slug} [put]
func (h *ArticleHandler) Update(c echo.Context) error {
	article, err := middleware.OwnedResource[*models.Article](c)
	if err != nil {
		return err
	}
	var req dto.UpdateArticleRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid request body")
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return failure(c, http.StatusUnprocessableEntity, "title cannot be empty")
		}
		if slugify(title) != slugify(article.Title) {
			slug, err := h.uniqueSlug(title)
			if err != nil {
				return dbError(c, err)
			}
			article.Slug = slug
		}
		article.Title = title
	}
	if req.Body != nil {
		article.Body = *req.Body
	}

	var tags []models.Tag
	if req.TagIDs != nil {
		if tags, err = h.tagRepo.FindByIDs(req.TagIDs); err != nil {
			return dbError(c, err)
		}
	}

	if err := h.repo.Update(article, tags); err != nil {
		return dbError(c, err)
	}

	updated, err := h.repo.FindByID(article.ID)
	if err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewArticleResponse(updated))
}

// Delete godoc
// @Summary      Xoá bài
// @Description  Chỉ tác giả được xoá.
// @Tags         articles
// @Security     TokenAuth
// @Param        slug  path  string  true  "Slug của article"  example(hoc-golang-co-ban)
// @Success      204  "No Content"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      403  {object}  handlers.ErrorResponse  "Không phải chủ sở hữu"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /articles/{slug} [delete]
func (h *ArticleHandler) Delete(c echo.Context) error {
	article, err := middleware.OwnedResource[*models.Article](c)
	if err != nil {
		return err
	}
	if err := h.repo.Delete(article.ID); err != nil {
		return dbError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
