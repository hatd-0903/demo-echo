package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

const ownedResourceKey = "owned_resource"

// OwnerLoader tải resource từ request (vd. theo :slug) và trả về ID chủ sở hữu của nó.
// Lỗi trả về nên là *echo.HTTPError (vd. 404) để ErrorHandler hiển thị đúng.
type OwnerLoader func(c echo.Context) (resource any, ownerID uint, err error)

// RequireOwner chỉ cho request đi tiếp khi user đang đăng nhập là chủ sở hữu resource.
// Phải đặt SAU middleware JWT. Resource đã tải được gắn vào context để handler
// lấy lại bằng OwnedResource, không phải query DB lần nữa.
func RequireOwner(load OwnerLoader, forbiddenMsg string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, ok := CurrentUserID(c)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing or invalid token")
			}

			resource, ownerID, err := load(c)
			if err != nil {
				return err
			}
			if ownerID != userID {
				return echo.NewHTTPError(http.StatusForbidden, forbiddenMsg)
			}

			c.Set(ownedResourceKey, resource)
			return next(c)
		}
	}
}

// OwnedResource lấy resource mà RequireOwner đã tải và kiểm tra.
func OwnedResource[T any](c echo.Context) (T, error) {
	res, ok := c.Get(ownedResourceKey).(T)
	if !ok {
		return res, echo.NewHTTPError(http.StatusInternalServerError, "route is missing RequireOwner middleware")
	}
	return res, nil
}
