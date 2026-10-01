package middleware

import (
	"net/http"

	"demo_echo/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
)

const userIDKey = "user_id"

// JWT trả về middleware bắt buộc đăng nhập.
// Nhận header "Authorization: Token <jwt>" (chuẩn RealWorld) hoặc "Authorization: Bearer <jwt>".
func JWT(tm *auth.TokenManager) echo.MiddlewareFunc {
	return echojwt.WithConfig(jwtConfig(tm))
}

// OptionalJWT cho phép request không có token đi qua (khách vãng lai);
// nếu có gửi token thì token phải hợp lệ.
func OptionalJWT(tm *auth.TokenManager) echo.MiddlewareFunc {
	cfg := jwtConfig(tm)
	cfg.Skipper = func(c echo.Context) bool {
		return c.Request().Header.Get(echo.HeaderAuthorization) == ""
	}
	return echojwt.WithConfig(cfg)
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

// CurrentUserID lấy user ID do middleware JWT gắn vào context.
// Trả về false nếu route không đi qua middleware JWT.
func CurrentUserID(c echo.Context) (uint, bool) {
	id, ok := c.Get(userIDKey).(uint)
	return id, ok && id != 0
}
