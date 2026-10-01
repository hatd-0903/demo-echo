package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"

	devJWTSecret = "dev-insecure-secret-change-me"
)

type Config struct {
	AppEnv  string
	AppPort string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	JWTSecret string
	JWTExpiry time.Duration

	// RedisURL rỗng = tắt cache.
	RedisURL string
	CacheTTL time.Duration

	// SwaggerEnabled mặc định bật ở development, tắt ở production.
	SwaggerEnabled bool
}

// Load đọc file env (mặc định .env, đổi bằng biến ENV_FILE) rồi tới biến môi trường.
// Biến môi trường đã có sẵn (vd. từ Docker/CI) luôn được ưu tiên hơn giá trị trong file.
func Load() (*Config, error) {
	envFile := getEnv("ENV_FILE", ".env")
	if err := godotenv.Load(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load %s: %w", envFile, err)
	}

	cfg := &Config{
		AppEnv:     getEnv("APP_ENV", EnvDevelopment),
		AppPort:    getEnv("APP_PORT", "8080"),
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "demo_echo"),
		JWTSecret:  getEnv("JWT_SECRET", ""),
		RedisURL:   getEnv("REDIS_URL", ""),
	}

	var err error
	if cfg.JWTExpiry, err = getDuration("JWT_EXPIRES_IN", 72*time.Hour); err != nil {
		return nil, err
	}
	if cfg.CacheTTL, err = getDuration("CACHE_TTL", 5*time.Minute); err != nil {
		return nil, err
	}
	if cfg.SwaggerEnabled, err = getBool("SWAGGER_ENABLED", cfg.AppEnv != EnvProduction); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) IsProduction() bool {
	return c.AppEnv == EnvProduction
}

func (c *Config) validate() error {
	if c.AppEnv != EnvDevelopment && c.AppEnv != EnvProduction {
		return fmt.Errorf("APP_ENV must be %q or %q, got %q", EnvDevelopment, EnvProduction, c.AppEnv)
	}
	if port, err := strconv.Atoi(c.AppPort); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("APP_PORT must be a valid port number, got %q", c.AppPort)
	}

	switch {
	case c.JWTSecret == "" && c.IsProduction():
		return errors.New("JWT_SECRET is required when APP_ENV=production")
	case c.JWTSecret == "":
		log.Println("WARNING: JWT_SECRET is not set, using an insecure default (development only)")
		c.JWTSecret = devJWTSecret
	case len(c.JWTSecret) < 32 && c.IsProduction():
		return errors.New("JWT_SECRET must be at least 32 characters in production")
	}
	return nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getBool(key string, fallback bool) (bool, error) {
	v := getEnv(key, "")
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false, got %q", key, v)
	}
	return b, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := getEnv(key, "")
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration like 30s, 5m, 72h; got %q", key, v)
	}
	return d, nil
}
