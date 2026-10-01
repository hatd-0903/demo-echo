package middleware

import (
	"errors"
	"net/http"

	"demo_echo/internal/auth"
	"demo_echo/internal/models"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	userIDKey      = "user_id"
	currentUserKey = "current_user"
)

// UserLoader tải user theo ID trong token (thường là UserRepository.FindByID);
// trả về gorm.ErrRecordNotFound nếu user không còn tồn tại.
type UserLoader func(id uint) (*models.User, error)

// JWT trả về middleware bắt buộc đăng nhập.
// Nhận header "Authorization: Token <jwt>" (chuẩn RealWorld) hoặc "Authorization: Bearer <jwt>".
// Sau khi token hợp lệ, user được nạp từ DB: tài khoản đã bị xoá thì token của nó
// bị từ chối ngay (không đợi hết hạn), và role luôn là giá trị mới nhất.
func JWT(tm *auth.TokenManager, load UserLoader) echo.MiddlewareFunc {
	return chain(echojwt.WithConfig(jwtConfig(tm)), withCurrentUser(load))
}

// OptionalJWT cho phép request không có token đi qua (khách vãng lai);
// nếu có gửi token thì token phải hợp lệ và user phải còn tồn tại.
func OptionalJWT(tm *auth.TokenManager, load UserLoader) echo.MiddlewareFunc {
	cfg := jwtConfig(tm)
	cfg.Skipper = func(c echo.Context) bool {
		return c.Request().Header.Get(echo.HeaderAuthorization) == ""
	}
	return chain(echojwt.WithConfig(cfg), withCurrentUser(load))
}

func jwtConfig(tm *auth.TokenManager) echojwt.Config {
	return echojwt.Config{
		SigningKey:    tm.Secret(),
		SigningMethod: jwt.SigningMethodHS256.Alg(),
		TokenLookup:   "header:Authorization:Token ,header:Authorization:Bearer ",
		NewClaimsFunc: func(echo.Context) jwt.Claims { return new(auth.Claims) },
		SuccessHandler: func(c echo.Context) {
			token := c.Get("user").(*jwt.Token)
			if id, err := token.Claims.(*auth.Claims).UserID(); err == nil {
				c.Set(userIDKey, id)
			}
		},
		ErrorHandler: func(c echo.Context, err error) error {
			return echo.NewHTTPError(http.StatusUnauthorized, "missing or invalid token")
		},
	}
}

// withCurrentUser nạp user của token vào context. Không có token (OptionalJWT bỏ qua) thì đi tiếp.
func withCurrentUser(load UserLoader) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			id, ok := c.Get(userIDKey).(uint)
			if !ok {
				if c.Get("user") != nil { // có token nhưng sub không phải số
					return echo.NewHTTPError(http.StatusUnauthorized, "missing or invalid token")
				}
				return next(c)
			}
			user, err := load(id)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusUnauthorized, "user no longer exists")
			}
			if err != nil {
				c.Logger().Error(err)
				return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
			}
			c.Set(currentUserKey, user)
			return next(c)
		}
	}
}

// RequireAdmin chỉ cho user có role admin đi tiếp. Phải đặt sau JWT.
func RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, ok := CurrentUser(c)
		if !ok {
			return echo.NewHTTPError(http.StatusUnauthorized, "missing or invalid token")
		}
		if !user.IsAdmin() {
			return echo.NewHTTPError(http.StatusForbidden, "admin role required")
		}
		return next(c)
	}
}

// CurrentUser trả về user đang đăng nhập (đã nạp từ DB bởi JWT/OptionalJWT).
func CurrentUser(c echo.Context) (*models.User, bool) {
	user, ok := c.Get(currentUserKey).(*models.User)
	return user, ok && user != nil
}

// CurrentUserID trả về ID user đang đăng nhập; false nếu request chưa đăng nhập.
func CurrentUserID(c echo.Context) (uint, bool) {
	user, ok := CurrentUser(c)
	if !ok {
		return 0, false
	}
	return user.ID, true
}

func chain(outer, inner echo.MiddlewareFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return outer(inner(next))
	}
}
