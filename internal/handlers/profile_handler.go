package handlers

import (
	"net/http"

	"demo_echo/internal/dto"
	"demo_echo/internal/middleware"
	"demo_echo/internal/repository"

	"github.com/labstack/echo/v4"
)

type ProfileHandler struct {
	userRepo   repository.UserRepository
	followRepo repository.FollowRepository
}

func NewProfileHandler(userRepo repository.UserRepository, followRepo repository.FollowRepository) *ProfileHandler {
	return &ProfileHandler{userRepo: userRepo, followRepo: followRepo}
}

// Get godoc
// @Summary      Profile công khai
// @Description  Token là tuỳ chọn: có token thì "following" cho biết bạn có đang follow user này không.
// @Tags         profiles
// @Produce      json
// @Security     TokenAuth
// @Param        username  path  string  true  "Username"  example(alice)
// @Success      200  {object}  handlers.Response{data=dto.ProfileResponse}  "OK"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /profiles/{username} [get]
func (h *ProfileHandler) Get(c echo.Context) error {
	user, err := h.userRepo.FindByUsername(c.Param("username"))
	if err != nil {
		return dbError(c, err)
	}

	following := false
	if viewerID, ok := middleware.CurrentUserID(c); ok {
		if following, err = h.followRepo.IsFollowing(viewerID, user.ID); err != nil {
			return dbError(c, err)
		}
	}
	return success(c, http.StatusOK, dto.NewProfileWithFollowing(user, following))
}

// Follow godoc
// @Summary      Follow user
// @Description  Idempotent: follow lại người đã follow không báo lỗi.
// @Tags         profiles
// @Produce      json
// @Security     TokenAuth
// @Param        username  path  string  true  "Username"  example(alice)
// @Success      200  {object}  handlers.Response{data=dto.ProfileResponse}  "OK"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Failure      422  {object}  handlers.ErrorResponse  "Không thể tự follow chính mình"
// @Router       /profiles/{username}/follow [post]
func (h *ProfileHandler) Follow(c echo.Context) error {
	viewerID, err := requireUserID(c)
	if err != nil {
		return err
	}
	user, err := h.userRepo.FindByUsername(c.Param("username"))
	if err != nil {
		return dbError(c, err)
	}
	if user.ID == viewerID {
		return failure(c, http.StatusUnprocessableEntity, "you cannot follow yourself")
	}
	if err := h.followRepo.Follow(viewerID, user.ID); err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewProfileWithFollowing(user, true))
}

// Unfollow godoc
// @Summary      Unfollow user
// @Tags         profiles
// @Produce      json
// @Security     TokenAuth
// @Param        username  path  string  true  "Username"  example(alice)
// @Success      200  {object}  handlers.Response{data=dto.ProfileResponse}  "OK"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /profiles/{username}/follow [delete]
func (h *ProfileHandler) Unfollow(c echo.Context) error {
	viewerID, err := requireUserID(c)
	if err != nil {
		return err
	}
	user, err := h.userRepo.FindByUsername(c.Param("username"))
	if err != nil {
		return dbError(c, err)
	}
	if err := h.followRepo.Unfollow(viewerID, user.ID); err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewProfileWithFollowing(user, false))
}
