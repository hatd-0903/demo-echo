package routes

import (
	"net/http"
	"time"

	"demo_echo/internal/auth"
	"demo_echo/internal/cache"
	"demo_echo/internal/handlers"
	"demo_echo/internal/middleware"
	"demo_echo/internal/repository"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Deps là mọi thứ router cần. Repository là interface nên test có thể truyền fake.
type Deps struct {
	Users    repository.UserRepository
	Articles repository.ArticleRepository
	Tags     repository.TagRepository
	Comments repository.CommentRepository
	Follows  repository.FollowRepository

	Tokens   *auth.TokenManager
	Cache    cache.Cache
	CacheTTL time.Duration
}

// GormRepositories tạo các repository dùng MySQL qua GORM.
func GormRepositories(db *gorm.DB) Deps {
	return Deps{
		Users:    repository.NewUserRepository(db),
		Articles: repository.NewArticleRepository(db),
		Tags:     repository.NewTagRepository(db),
		Comments: repository.NewCommentRepository(db),
		Follows:  repository.NewFollowRepository(db),
	}
}

func Register(e *echo.Echo, d Deps) {
	if d.Cache == nil {
		d.Cache = cache.Noop{}
	}

	authHandler := handlers.NewAuthHandler(d.Users, d.Tokens)
	userHandler := handlers.NewUserHandler(d.Users)
	articleHandler := handlers.NewArticleHandler(d.Articles, d.Users, d.Tags)
	tagHandler := handlers.NewTagHandler(d.Tags)
	commentHandler := handlers.NewCommentHandler(d.Comments, d.Articles, d.Users)
	profileHandler := handlers.NewProfileHandler(d.Users, d.Follows)

	// requireAuth gắn theo từng route (không dùng Group middleware) để các route
	// public và private cùng prefix như /api/users không ảnh hưởng lẫn nhau.
	requireAuth := middleware.JWT(d.Tokens)
	optionalAuth := middleware.OptionalJWT(d.Tokens)

	// Kiểm tra quyền sở hữu: phải đặt sau requireAuth.
	ownsArticle := middleware.RequireOwner(articleHandler.LoadOwner, "you are not the author of this article")
	ownsAccount := middleware.RequireOwner(userHandler.LoadOwner, "you can only modify your own account")

	// Cache cho endpoint public truy cập nhiều. Mỗi thao tác ghi làm mới namespace liên quan:
	// - article đổi -> danh sách article
	// - tag đổi tên/xoá -> danh sách tag và danh sách article (article nhúng tag)
	// - user đổi username/bio/image/xoá -> danh sách article (article nhúng author)
	cacheTags := middleware.CacheResponse(d.Cache, middleware.CacheNSTags, d.CacheTTL)
	cacheArticles := middleware.CacheResponse(d.Cache, middleware.CacheNSArticles, d.CacheTTL)
	invalidateArticles := middleware.InvalidateCache(d.Cache, middleware.CacheNSArticles)
	invalidateTags := middleware.InvalidateCache(d.Cache, middleware.CacheNSTags, middleware.CacheNSArticles)

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	api := e.Group("/api")

	// Auth
	api.POST("/users", authHandler.Register)
	api.POST("/users/login", authHandler.Login)
	api.GET("/user", authHandler.Current, requireAuth)

	// Users
	api.GET("/users", userHandler.List, requireAuth)
	api.GET("/users/:id", userHandler.Get, requireAuth)
	api.PUT("/users/:id", userHandler.Update, requireAuth, ownsAccount, invalidateArticles)
	api.DELETE("/users/:id", userHandler.Delete, requireAuth, ownsAccount, invalidateArticles)

	// Profiles
	api.GET("/profiles/:username", profileHandler.Get, optionalAuth)
	api.POST("/profiles/:username/follow", profileHandler.Follow, requireAuth)
	api.DELETE("/profiles/:username/follow", profileHandler.Unfollow, requireAuth)

	// Articles (route tĩnh /feed được Echo ưu tiên hơn /:slug).
	// Feed phụ thuộc từng user nên không cache; global list thì cache.
	api.GET("/articles", articleHandler.List, cacheArticles)
	api.GET("/articles/feed", articleHandler.Feed, requireAuth)
	api.GET("/articles/:slug", articleHandler.Get)
	api.POST("/articles", articleHandler.Create, requireAuth, invalidateArticles)
	api.PUT("/articles/:slug", articleHandler.Update, requireAuth, ownsArticle, invalidateArticles)
	api.DELETE("/articles/:slug", articleHandler.Delete, requireAuth, ownsArticle, invalidateArticles)

	// Comments
	api.GET("/articles/:slug/comments", commentHandler.List)
	api.POST("/articles/:slug/comments", commentHandler.Create, requireAuth)

	// Tags
	api.GET("/tags", tagHandler.List, cacheTags)
	api.GET("/tags/:id", tagHandler.Get)
	api.POST("/tags", tagHandler.Create, requireAuth, invalidateTags)
	api.PUT("/tags/:id", tagHandler.Update, requireAuth, invalidateTags)
	api.DELETE("/tags/:id", tagHandler.Delete, requireAuth, invalidateTags)
}
