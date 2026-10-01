package handlers

import (
	"net/http"
	"strings"

	"demo_echo/internal/auth"
	"demo_echo/internal/dto"
	"demo_echo/internal/middleware"
	"demo_echo/internal/models"
	"demo_echo/internal/repository"

	"github.com/labstack/echo/v4"
)

type UserHandler struct {
	repo repository.UserRepository
}

func NewUserHandler(repo repository.UserRepository) *UserHandler {
	return &UserHandler{repo: repo}
}

// List godoc
// @Summary      Danh sách user
// @Tags         users
// @Produce      json
// @Security     TokenAuth
// @Success      200  {object}  handlers.Response{data=[]dto.UserResponse}  "OK"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Router       /users [get]
func (h *UserHandler) List(c echo.Context) error {
	users, err := h.repo.FindAll()
	if err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewUserListResponse(users))
}

// Get godoc
// @Summary      Chi tiết user
// @Tags         users
// @Produce      json
// @Security     TokenAuth
// @Param        id    path  int     true  "ID user"
// @Success      200  {object}  handlers.Response{data=dto.UserResponse}  "OK"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /users/{id} [get]
func (h *UserHandler) Get(c echo.Context) error {
	id, err := parseID(c)
	if err != nil {
		return err
	}
	user, err := h.repo.FindByID(id)
	if err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewUserResponse(user))
}

// Update godoc
// @Summary      Cập nhật tài khoản
// @Description  Chỉ sửa được tài khoản của chính mình. Field nào không gửi thì giữ nguyên.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     TokenAuth
// @Param        id    path  int     true  "ID user"
// @Param        body  body  dto.UpdateUserRequest  true  "Các field cần sửa"
// @Success      200  {object}  handlers.Response{data=dto.UserResponse}  "OK"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      403  {object}  handlers.ErrorResponse  "Không phải chủ sở hữu"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Failure      409  {object}  handlers.ErrorResponse  "Đã tồn tại"
// @Failure      422  {object}  handlers.ErrorResponse  "Dữ liệu không hợp lệ"
// @Router       /users/{id} [put]
func (h *UserHandler) Update(c echo.Context) error {
	user, err := middleware.OwnedResource[*models.User](c)
	if err != nil {
		return err
	}
	var req dto.UpdateUserRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid request body")
	}
	if req.Username != nil {
		user.Username = strings.TrimSpace(*req.Username)
	}
	if req.Email != nil {
		user.Email = strings.ToLower(strings.TrimSpace(*req.Email))
	}
	if req.Bio != nil {
		user.Bio = *req.Bio
	}
	if req.Image != nil {
		user.Image = *req.Image
	}
	if req.Password != nil {
		if len(*req.Password) < auth.MinPasswordLength {
			return failure(c, http.StatusUnprocessableEntity, "password must be at least 6 characters")
		}
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			return failure(c, http.StatusInternalServerError, "could not hash password")
		}
		user.Password = hash
	}
	if user.Username == "" || user.Email == "" {
		return failure(c, http.StatusUnprocessableEntity, "username and email cannot be empty")
	}

	if err := h.repo.Update(user); err != nil {
		return dbError(c, err)
	}
	return success(c, http.StatusOK, dto.NewUserResponse(user))
}

// Delete godoc
// @Summary      Xoá tài khoản
// @Description  Chỉ xoá được tài khoản của chính mình.
// @Tags         users
// @Security     TokenAuth
// @Param        id    path  int     true  "ID user"
// @Success      204  "No Content"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Failure      403  {object}  handlers.ErrorResponse  "Không phải chủ sở hữu"
// @Failure      404  {object}  handlers.ErrorResponse  "Không tìm thấy"
// @Router       /users/{id} [delete]
func (h *UserHandler) Delete(c echo.Context) error {
	user, err := middleware.OwnedResource[*models.User](c)
	if err != nil {
		return err
	}
	if err := h.repo.Delete(user.ID); err != nil {
		return dbError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// LoadOwner là OwnerLoader cho middleware.RequireOwner: user "sở hữu" chính tài khoản của mình.
func (h *UserHandler) LoadOwner(c echo.Context) (any, uint, error) {
	id, err := parseID(c)
	if err != nil {
		return nil, 0, err
	}
	user, err := h.repo.FindByID(id)
	if err != nil {
		return nil, 0, dbError(c, err)
	}
	return user, user.ID, nil
}
