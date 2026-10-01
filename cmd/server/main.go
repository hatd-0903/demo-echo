package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"demo_echo/internal/auth"
	"demo_echo/internal/cache"
	"demo_echo/internal/config"
	"demo_echo/internal/database"
	"demo_echo/internal/routes"
	"demo_echo/internal/server"
)

// @title                      demo_echo API
// @version                    1.0
// @description                RealWorld API (Conduit) viết bằng Go + Echo + GORM + MySQL + Redis.
// @description                Mọi response có dạng {"success": bool, "message"?: string, "data"?: ...}.
// @BasePath                   /api
// @schemes                    http https
//
// @securityDefinitions.apikey TokenAuth
// @in                         header
// @name                       Authorization
// @description                Nhập "Token <jwt>" (hoặc "Bearer <jwt>"). Lấy jwt từ /users/login.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	if cfg.DBAutoMigrate {
		if err := database.Migrate(db); err != nil {
			log.Fatalf("failed to migrate database: %v", err)
		}
	}

	auth.CheckPasswordAgainstDummy("") // tạo sẵn hash giả để lần login đầu không chậm hơn

	store, closeCache := setupCache(cfg)
	defer closeCache()

	deps := routes.GormRepositories(db)
	deps.Tokens = auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpiry)
	deps.Cache = store
	deps.CacheTTL = cfg.CacheTTL

	e := server.New(deps, server.Options{LogRequests: true, Swagger: cfg.SwaggerEnabled})
	if cfg.SwaggerEnabled {
		log.Printf("Swagger UI: http://localhost:%s/swagger/index.html", cfg.AppPort)
	}

	go func() {
		if err := e.Start(":" + cfg.AppPort); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	log.Println("server stopped")
}

// setupCache dùng Redis nếu có REDIS_URL. Redis không kết nối được lúc khởi động
// chỉ cảnh báo: middleware cache tự bỏ qua khi Redis lỗi nên API vẫn chạy.
func setupCache(cfg *config.Config) (cache.Cache, func()) {
	if cfg.RedisURL == "" {
		log.Println("REDIS_URL not set, caching disabled")
		return cache.Noop{}, func() {}
	}
	r, err := cache.NewRedis(cfg.RedisURL)
	if err != nil {
		log.Fatalf("invalid REDIS_URL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.Ping(ctx); err != nil {
		log.Printf("WARNING: cannot reach Redis (%v), requests will bypass cache until it is back", err)
	} else {
		log.Println("connected to Redis")
	}
	return r, func() { _ = r.Close() }
}
