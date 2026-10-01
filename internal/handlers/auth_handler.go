package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"demo_echo/internal/auth"
	"demo_echo/internal/dto"
	"demo_echo/internal/models"
	"demo_echo/internal/repository"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type AuthHandler struct {
	userRepo repository.UserRepository
	tokens   *auth.TokenManager
}

func NewAuthHandler(userRepo repository.UserRepository, tokens *auth.TokenManager) *AuthHandler {
	return &AuthHandler{userRepo: userRepo, tokens: tokens}
}

// Register godoc
// @Summary      Đăng ký
// @Description  Tạo tài khoản mới, trả về user kèm JWT. Mật khẩu được băm bằng bcrypt.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  dto.RegisterRequest  true  "Thông tin đăng ký"
// @Success      201  {object}  handlers.Response{data=dto.AuthUserResponse}  "Created"
// @Failure      400  {object}  handlers.ErrorResponse  "Body/param không hợp lệ"
// @Failure      409  {object}  handlers.ErrorResponse  "Đã tồn tại"
// @Failure      422  {object}  handlers.ErrorResponse  "Dữ liệu không hợp lệ"
// @Router       /users [post]
func (h *AuthHandler) Register(c echo.Context) error {
	var req dto.RegisterRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid request body")
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Username == "" {
		return failure(c, http.StatusUnprocessableEntity, "username is required")
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return failure(c, http.StatusUnprocessableEntity, "email is invalid")
	}
	if len(req.Password) < auth.MinPasswordLength {
		return failure(c, http.StatusUnprocessableEntity, "password must be at least 6 characters")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "could not hash password")
	}

	user := &models.User{Username: req.Username, Email: req.Email, Password: hash, Role: models.RoleUser}
	if err := h.userRepo.Create(user); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return failure(c, http.StatusConflict, "username or email already taken")
		}
		return dbError(c, err)
	}
	return h.respondWithToken(c, http.StatusCreated, user)
}

// Login godoc
// @Summary      Đăng nhập
// @Description  Sai email hay sai mật khẩu đều trả cùng lỗi 401 để không lộ email nào đã đăng ký.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body  dto.LoginRequest  true  "Email + mật khẩu"
// @Success      200  {object}  handlers.Response{data=dto.AuthUserResponse}  "OK"
// @Failure      401  {object}  handlers.ErrorResponse  "Sai email hoặc mật khẩu"
// @Failure      422  {object}  handlers.ErrorResponse  "Dữ liệu không hợp lệ"
// @Router       /users/login [post]
func (h *AuthHandler) Login(c echo.Context) error {
	var req dto.LoginRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid request body")
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		return failure(c, http.StatusUnprocessableEntity, "email and password are required")
	}

	// "Sai email" và "sai mật khẩu" trả cùng thông báo và tốn cùng thời gian (đều chạy bcrypt)
	// để không thể dò ra email nào đã đăng ký.
	user, err := h.userRepo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			auth.CheckPasswordAgainstDummy(req.Password)
			return failure(c, http.StatusUnauthorized, "invalid email or password")
		}
		return dbError(c, err)
	}
	if !auth.CheckPassword(user.Password, req.Password) {
		return failure(c, http.StatusUnauthorized, "invalid email or password")
	}
	return h.respondWithToken(c, http.StatusOK, user)
}

// Current godoc
// @Summary      User hiện tại
// @Description  Lấy user theo JWT (kèm token mới).
// @Tags         auth
// @Produce      json
// @Security     TokenAuth
// @Success      200  {object}  handlers.Response{data=dto.AuthUserResponse}  "OK"
// @Failure      401  {object}  handlers.ErrorResponse  "Thiếu hoặc sai token"
// @Router       /user [get]
func (h *AuthHandler) Current(c echo.Context) error {
	user, err := requireUser(c) // middleware.JWT đã nạp từ DB (user bị xoá -> 401 ở đó)
	if err != nil {
		return err
	}
	return h.respondWithToken(c, http.StatusOK, user)
}

func (h *AuthHandler) respondWithToken(c echo.Context, status int, user *models.User) error {
	token, err := h.tokens.Generate(user.ID, user.Username)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "could not generate token")
	}
	return success(c, status, dto.NewAuthUserResponse(user, token))
}
