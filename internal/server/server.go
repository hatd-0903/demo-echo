package server

import (
	"net/http"

	"demo_echo/docs"
	"demo_echo/internal/handlers"
	"demo_echo/internal/routes"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
)

type Options struct {
	LogRequests bool
	// Swagger bật Swagger UI tại /swagger/index.html và spec tại /swagger/doc.json.
	Swagger bool
}

// New dựng Echo hoàn chỉnh (error handler, middleware chung, routes).
// main và test dùng chung hàm này nên test chạy đúng stack như production.
func New(d routes.Deps, opts Options) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = !opts.LogRequests
	e.HTTPErrorHandler = handlers.ErrorHandler

	if opts.LogRequests {
		e.Use(middleware.RequestLogger())
	}
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	routes.Register(e, d)

	if opts.Swagger {
		// Host để trống: Swagger UI gọi API trên chính host đang mở trang.
		docs.SwaggerInfo.Host = ""
		e.GET("/swagger", func(c echo.Context) error {
			return c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
		})
		e.GET("/swagger/*", echoSwagger.EchoWrapHandler(echoSwagger.PersistAuthorization(true)))
	}
	return e
}
