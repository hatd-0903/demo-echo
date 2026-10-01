package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"demo_echo/internal/middleware"
	"demo_echo/internal/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Response là format chung của mọi response thành công: {"success": true, "data": ...}.
type Response struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// ErrorResponse mô tả format lỗi (dùng cho tài liệu Swagger; runtime dùng Response).
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"resource not found"`
}

func success(c echo.Context, status int, data any) error {
	return c.JSON(status, Response{Success: true, Data: data})
}

func failure(c echo.Context, status int, msg string) error {
	return c.JSON(status, Response{Success: false, Message: msg})
}

// dbError chuyển lỗi GORM thành *echo.HTTPError; ErrorHandler sẽ ghi response.
// Luôn trả về error khác nil nên dùng được cả trong hàm helper lẫn "return dbError(c, err)".
func dbError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "resource not found")
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return echo.NewHTTPError(http.StatusConflict, "resource already exists")
	default:
		c.Logger().Error(err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}

func parseID(c echo.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	return uint(id), nil
}

// requireUser lấy user đang đăng nhập (middleware.JWT đã nạp từ DB).
func requireUser(c echo.Context) (*models.User, error) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "missing or invalid token")
	}
	return user, nil
}

// requireUserID như requireUser nhưng chỉ cần ID.
func requireUserID(c echo.Context) (uint, error) {
	user, err := requireUser(c)
	if err != nil {
		return 0, err
	}
	return user.ID, nil
}

// ErrorHandler định dạng mọi lỗi của Echo (404 route, 405, HTTPError...) theo cùng format Response.
func ErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}
	status := http.StatusInternalServerError
	msg := "internal server error"
	var he *echo.HTTPError
	if errors.As(err, &he) {
		status = he.Code
		if m, ok := he.Message.(string); ok {
			msg = m
		} else {
			msg = http.StatusText(status)
		}
	} else {
		c.Logger().Error(err)
	}

	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(status)
		return
	}
	_ = failure(c, status, msg)
}

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

// parsePage đọc ?limit=&offset= dùng chung cho các endpoint danh sách.
// limit mặc định 20, lớn hơn 100 thì hạ xuống 100.
func parsePage(c echo.Context) (limit, offset int, err error) {
	limit = defaultPageLimit
	if err := echo.QueryParamsBinder(c).
		Int("limit", &limit).
		Int("offset", &offset).
		BindError(); err != nil {
		return 0, 0, echo.NewHTTPError(http.StatusBadRequest, "limit and offset must be integers")
	}
	if limit < 1 {
		return 0, 0, echo.NewHTTPError(http.StatusBadRequest, "limit must be greater than 0")
	}
	if offset < 0 {
		return 0, 0, echo.NewHTTPError(http.StatusBadRequest, "offset must not be negative")
	}
	return min(limit, maxPageLimit), offset, nil
}
