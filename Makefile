export PATH := $(HOME)/.local/go/bin:$(HOME)/go/bin:$(PATH)

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

# Test end-to-end trên MySQL + Redis thật (DB riêng demo_echo_test). Cần: make up
e2e:
	go test -tags e2e -count=1 -v ./internal/e2e/

# Chạy migration (dùng khi DB_AUTO_MIGRATE=false, vd. production)
migrate:
	go run ./cmd/manage migrate

# Cấp quyền admin: make admin EMAIL=alice@example.com
admin:
	go run ./cmd/manage set-role -email $(EMAIL) admin

# Sinh lại tài liệu OpenAPI từ annotation trong handlers -> docs/
swagger:
	swag init -g cmd/server/main.go -o docs --parseInternal --outputTypes go,json,yaml

cover:
	go test -coverpkg=./internal/... -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out | tail -1

# Khởi động MySQL + Redis
up:
	docker compose up -d mysql redis

down:
	docker compose down

db-up:
	docker compose up -d mysql

redis-cli:
	docker exec -it demo_echo_redis redis-cli

tidy:
	go mod tidy

.PHONY: run build test e2e migrate admin swagger cover up down db-up redis-cli tidy
