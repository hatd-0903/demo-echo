package config

import (
	"os"
	"testing"
	"time"
)

// isolate trỏ ENV_FILE tới file không tồn tại để test không đọc .env thật,
// và unset các biến liên quan (godotenv không ghi đè biến đã tồn tại, kể cả rỗng).
// t.Setenv đăng ký khôi phục giá trị cũ khi test kết thúc.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("ENV_FILE", t.TempDir()+"/missing.env")
	for _, k := range []string{"APP_ENV", "APP_PORT", "JWT_SECRET", "JWT_EXPIRES_IN", "CACHE_TTL", "REDIS_URL", "SWAGGER_ENABLED"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	isolate(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppEnv != EnvDevelopment || cfg.AppPort != "8080" {
		t.Errorf("unexpected defaults: env=%q port=%q", cfg.AppEnv, cfg.AppPort)
	}
	if cfg.JWTSecret != devJWTSecret {
		t.Errorf("expected dev JWT secret fallback in development")
	}
	if cfg.JWTExpiry != 72*time.Hour || cfg.CacheTTL != 5*time.Minute {
		t.Errorf("unexpected durations: jwt=%v cache=%v", cfg.JWTExpiry, cfg.CacheTTL)
	}
	if cfg.RedisURL != "" {
		t.Errorf("cache should be disabled by default")
	}
	if !cfg.SwaggerEnabled {
		t.Errorf("swagger should be enabled by default in development")
	}
}

func TestLoadFromEnvFile(t *testing.T) {
	isolate(t)
	file := t.TempDir() + "/test.env"
	writeFile(t, file, "APP_PORT=9000\nCACHE_TTL=30s\nREDIS_URL=redis://localhost:6379/1\n")
	t.Setenv("ENV_FILE", file)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppPort != "9000" || cfg.CacheTTL != 30*time.Second || cfg.RedisURL != "redis://localhost:6379/1" {
		t.Errorf("values from env file not applied: %+v", cfg)
	}
}

func TestLoadValidation(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"production without secret", map[string]string{"APP_ENV": "production"}},
		{"production with short secret", map[string]string{"APP_ENV": "production", "JWT_SECRET": "short"}},
		{"unknown APP_ENV", map[string]string{"APP_ENV": "staging"}},
		{"invalid port", map[string]string{"APP_PORT": "abc"}},
		{"invalid duration", map[string]string{"CACHE_TTL": "five minutes"}},
		{"negative duration", map[string]string{"JWT_EXPIRES_IN": "-1h"}},
		{"invalid bool", map[string]string{"SWAGGER_ENABLED": "maybe"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestLoadProductionWithValidSecret(t *testing.T) {
	isolate(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.IsProduction() {
		t.Error("expected production config")
	}
	if cfg.SwaggerEnabled {
		t.Error("swagger should be disabled by default in production")
	}
}
