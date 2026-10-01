# demo_echo

REST API demo (RealWorld) với **Go + Echo v4 + GORM + MySQL + Redis**.

## Cấu trúc

```
demo_echo/
├── cmd/server/main.go          # Entry point: load config, kết nối DB, migrate, start Echo
├── internal/
│   ├── config/                 # Đọc + validate cấu hình từ .env / biến môi trường
│   ├── database/               # Kết nối MySQL (GORM) + AutoMigrate
│   ├── models/                 # User, Article, Comment, Tag (GORM, chỉ dùng để lưu trữ)
│   ├── dto/                    # Request/Response struct + mapper (ẩn password, email...)
│   ├── repository/             # Truy cập dữ liệu (interface + GORM impl)
│   ├── auth/                   # bcrypt + tạo/verify JWT
│   ├── cache/                  # Interface Cache: Redis / Memory (test) / Noop
│   ├── middleware/             # JWT, RequireOwner, CacheResponse/InvalidateCache
│   ├── handlers/               # HTTP handlers (Echo) + test httptest
│   ├── routes/                 # Đăng ký route, wiring repository -> handler
│   ├── server/                 # server.New: dựng Echo (dùng chung cho main và test)
│   └── e2e/                    # Test end-to-end + đếm query (build tag e2e)
├── docs/                       # OpenAPI sinh bởi swag (make swagger)
├── docker-compose.yml          # MySQL 8.4 (port 3307) + Redis 7 (port 6379)
├── .env.example
└── Makefile
```

## Quan hệ model

- `User` 1-n `Article` (qua `author_id`), `User` 1-n `Comment`
- `Article` 1-n `Comment`
- `Article` n-n `Tag` (bảng trung gian `article_tags`)
- `Article` / `Comment` BelongsTo `User`, `Comment` BelongsTo `Article`

API không bao giờ trả model trực tiếp: handler map model -> DTO trong `internal/dto`.
`ProfileResponse` (username, bio, image) được dùng làm `author` của article/comment
nên email, password không bao giờ lộ ra ngoài.

## Chạy

```bash
cp .env.example .env     # rồi điền JWT_SECRET: openssl rand -hex 32
make up                  # khởi động MySQL + Redis bằng Docker
make run                 # chạy server tại :8080
make test                # chạy test (không cần MySQL/Redis)
make e2e                 # test end-to-end trên MySQL + Redis thật
make cover               # test + coverage
```

Swagger UI: <http://localhost:8080/swagger/index.html> (spec: `/swagger/doc.json`, file: `docs/swagger.yaml`).

## Tài liệu API (Swagger / OpenAPI)

Tài liệu sinh từ annotation `// @Summary`, `// @Router`... ngay trên mỗi handler bằng
[swag](https://github.com/swaggo/swag). Sau khi thêm/sửa endpoint:

```bash
make swagger     # cần swag CLI: go install github.com/swaggo/swag/cmd/swag@latest
```

- Bấm **Authorize**, nhập `Token <jwt>` (jwt lấy từ `POST /users/login`) để gọi các route 🔒.
- Bật mặc định ở development, tắt ở production (đổi bằng `SWAGGER_ENABLED`).
- `TestSwaggerDocsMatchRoutes` fail nếu có route chưa được tài liệu hoá hoặc tài liệu chứa route không còn tồn tại.

## Biến môi trường

Cấu hình đọc từ file `.env` (mẫu: `.env.example`), sau đó tới biến môi trường thật.
Biến môi trường thật **luôn thắng** file `.env`, nên Docker/CI chỉ cần set env, không cần file.
Dùng file khác bằng `ENV_FILE`, ví dụ `ENV_FILE=.env.staging make run`.

| Biến             | Mặc định      | Ghi chú                                                         |
|------------------|---------------|-----------------------------------------------------------------|
| `APP_ENV`        | `development` | `development` hoặc `production`                                 |
| `APP_PORT`       | `8080`        |                                                                 |
| `DB_HOST` ... `DB_NAME` |        | Kết nối MySQL                                                   |
| `JWT_SECRET`     |               | **Bắt buộc** (≥ 32 ký tự) khi `production`; dev dùng secret tạm |
| `JWT_EXPIRES_IN` | `72h`         | Thời hạn token                                                  |
| `REDIS_URL`      | (trống)       | Trống = tắt cache. Vd `redis://127.0.0.1:6379/0`                |
| `CACHE_TTL`      | `5m`          | Thời gian sống của response đã cache                            |
| `SWAGGER_ENABLED`| theo `APP_ENV`| Bật Swagger UI; mặc định bật ở development, tắt ở production    |

Config được validate lúc khởi động: sai giá trị (port không phải số, duration sai, thiếu secret
ở production...) thì server dừng ngay với thông báo rõ ràng thay vì chạy với cấu hình hỏng.

## Cache (Redis)

`GET /api/tags` và `GET /api/articles` (global feed) được cache bằng middleware `CacheResponse`.
Header `X-Cache` cho biết `HIT` / `MISS` / `BYPASS` (Redis lỗi).

- **Key**: `cache:<namespace>:v<version>:<path>?<query đã sắp xếp>` (`?a=1&b=2` và `?b=2&a=1` dùng chung key).
- **Invalidate**: thao tác ghi thành công (2xx) tăng `cache:<namespace>:version` → mọi key cũ
  không còn được đọc và tự hết hạn theo TTL. Không cần `SCAN`/`DEL` hàng loạt.

  | Thao tác ghi                       | Namespace bị làm mới |
  |------------------------------------|----------------------|
  | Tạo/sửa/xoá article                | `articles`           |
  | Tạo/sửa/xoá tag                    | `tags`, `articles` (article nhúng tag) |
  | Sửa/xoá user                       | `articles` (article nhúng author)      |

- `/api/articles/feed` **không** cache vì phụ thuộc từng user.
- Redis down hoặc treo: request vẫn trả bình thường (`X-Cache: BYPASS`), timeout ~0.5s.

## Tối ưu truy vấn GORM (N+1)

Danh sách article/comment dùng `Preload` nên số query **cố định** dù trang có bao nhiêu dòng
(`TestNoNPlusOne` đo với 2 và 12 bản ghi, số query phải bằng nhau). Các tối ưu thêm, đo bằng
`TestQueryBudget` (đếm câu SQL thật qua GORM callback):

| Thao tác                        | Trước | Sau | Thay đổi |
|---------------------------------|------:|----:|----------|
| GET /articles (1 trang)         | 5 | 4 | Bỏ `COUNT(*)` khi trang chưa đầy (tổng = offset + số dòng) |
| GET /articles/:slug/comments    | 6 | 3 | Chỉ lấy dòng article (không preload author/tags) để biết `article_id` |
| POST /articles (2 tags)         | 10 | 5 | `Omit("Tags.*")`: không upsert lại bảng `tags`; dựng response từ dữ liệu đã có |
| POST /articles (title trùng lần 4) | 9 | 3 | Lấy mọi slug `base`, `base-%` trong **1** query thay vì hỏi từng slug (N query) |
| PUT /articles/:slug             | 9 | 6 | `RequireOwner` không preload; `UPDATE` chỉ các cột sửa được |
| PUT /articles/:slug (đổi tags)  | 14 | 9 | Đồng bộ bảng nối trực tiếp thay cho `Association.Replace` |
| PUT/DELETE bởi người khác (403) | 4 | 1 | |
| POST comment                    | 8 | 3 | Không preload article; không đọc lại comment sau khi tạo |
| DELETE /articles/:slug          | 5 | 2 | |

Ngoài ra: preload author chỉ `SELECT id, username, bio, image` (không kéo password hash/email về),
thêm index `articles.created_at` (sắp xếp mặc định) và index ghép `comments(article_id, created_at)`.

Xem câu SQL thật của từng thao tác: `E2E_SQL=1 make e2e`.

## Test

**Unit/handler test** (`make test`): test chạy qua toàn bộ stack HTTP thật (`server.New`: router, JWT, RequireOwner, cache,
error handler) bằng `httptest`, với repository giả lập trong bộ nhớ (`internal/handlers/fakes_test.go`)
nên không cần MySQL/Redis.

```bash
make test
go test ./internal/handlers/ -run TestArticlesCache -v   # chạy 1 test
```

**End-to-end** (`make e2e`, cần `make up`): chạy app thật trên MySQL (database riêng
`demo_echo_test`, tạo lại mỗi lần chạy) + Redis (DB 15) qua HTTP thật:

- `TestUserJourney`: đăng ký → đăng nhập → đăng bài có tag → tìm theo tag/tác giả (cache HIT)
  → comment → follow + feed → người khác sửa bị 403, tác giả sửa (cache được làm mới)
  → unfollow → xoá bài.
- `TestNoNPlusOne`, `TestQueryBudget`: số câu SQL của từng endpoint (xem phần tối ưu ở trên).

Tự skip nếu MySQL/Redis không chạy.

## Xác thực (JWT)

- Mật khẩu được băm bằng **bcrypt** trước khi lưu (`internal/auth/password.go`).
- Đăng ký / đăng nhập trả về JWT (HS256), payload `{"sub": "<user_id>", "username", "exp", "iat"}`.
- Gửi token qua header `Authorization: Token <jwt>` (chuẩn RealWorld) hoặc `Authorization: Bearer <jwt>`.
- Cấu hình trong `.env`: `JWT_SECRET` (tạo bằng `openssl rand -hex 32`), `JWT_EXPIRES_IN` (vd. `72h`).

## API

🔒 = cần token.

| Method | Path                          | Mô tả                                   |
|--------|-------------------------------|-----------------------------------------|
| GET    | /health                       | Health check                            |
| POST   | /api/users                    | Đăng ký, trả về user + token            |
| POST   | /api/users/login              | Đăng nhập, trả về user + token          |
| GET    | /api/user                     | 🔒 User hiện tại (theo token)           |
| GET    | /api/users                    | 🔒 Danh sách user                       |
| GET    | /api/users/:id                | 🔒 Chi tiết user                        |
| PUT    | /api/users/:id                | 🔒 Cập nhật (chỉ chính mình — `RequireOwner`) |
| DELETE | /api/users/:id                | 🔒 Xoá (chỉ chính mình — `RequireOwner`) |
| GET    | /api/profiles/:username       | Profile công khai (token tuỳ chọn → `following`) |
| POST   | /api/profiles/:username/follow| 🔒 Follow user                          |
| DELETE | /api/profiles/:username/follow| 🔒 Unfollow user                        |
| GET    | /api/articles                 | Danh sách article (lọc/sắp xếp/phân trang) |
| GET    | /api/articles/feed            | 🔒 Bài của những người đang follow      |
| GET    | /api/articles/:slug           | Chi tiết article                        |
| POST   | /api/articles                 | 🔒 Tạo article (tác giả = user từ token)|
| PUT    | /api/articles/:slug           | 🔒 Cập nhật (chỉ tác giả — `RequireOwner`) |
| DELETE | /api/articles/:slug           | 🔒 Xoá (chỉ tác giả — `RequireOwner`)   |
| GET    | /api/articles/:slug/comments  | Danh sách comment                       |
| POST   | /api/articles/:slug/comments  | 🔒 Thêm comment                         |
| GET    | /api/tags                     | Danh sách tag                           |
| GET    | /api/tags/:id                 | Chi tiết tag                            |
| POST   | /api/tags                     | 🔒 Tạo tag                              |
| PUT    | /api/tags/:id                 | 🔒 Đổi tên tag                          |
| DELETE | /api/tags/:id                 | 🔒 Xoá tag                              |

### Query params cho `GET /api/articles` và `/api/articles/feed`

| Param    | Mặc định      | Mô tả                                                        |
|----------|---------------|--------------------------------------------------------------|
| `tag`    |               | Lọc theo tên tag (chỉ `/api/articles`)                       |
| `author` |               | Lọc theo username tác giả (chỉ `/api/articles`)              |
| `sort`   | `-created_at` | `created_at`, `updated_at`, `title`; thêm `-` để giảm dần    |
| `limit`  | `20`          | Số bài mỗi trang, tối đa `100`                               |
| `offset` | `0`           | Bỏ qua bao nhiêu bài                                         |

Response: `{"articles": [...], "articles_count": <tổng số bài khớp filter>, "limit", "offset"}`.

```bash
curl 'localhost:8080/api/articles?tag=golang&author=lan&sort=-created_at&limit=10&offset=0'
curl localhost:8080/api/articles/feed -H "Authorization: Token $TOKEN"
```

### Middleware kiểm tra quyền sở hữu

`middleware.RequireOwner(loader, msg)` (`internal/middleware/ownership.go`) dùng chung cho mọi resource:
loader tải resource và trả về ID chủ sở hữu; nếu khác user trong JWT → 403. Resource đã tải được
gắn vào context, handler lấy lại bằng `middleware.OwnedResource[*models.Article](c)`.

```go
ownsArticle := middleware.RequireOwner(articleHandler.LoadOwner, "you are not the author of this article")
api.PUT("/articles/:slug", articleHandler.Update, requireAuth, ownsArticle)
```

Ví dụ:

```bash
# Đăng ký
curl -X POST localhost:8080/api/users -H 'Content-Type: application/json' \
  -d '{"username":"ha","email":"ha@example.com","password":"123456"}'

# Đăng nhập, lấy token
TOKEN=$(curl -s -X POST localhost:8080/api/users/login -H 'Content-Type: application/json' \
  -d '{"email":"ha@example.com","password":"123456"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

# User hiện tại
curl localhost:8080/api/user -H "Authorization: Token $TOKEN"

# Tạo tag, article, comment
curl -X POST localhost:8080/api/tags -H "Authorization: Token $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"golang"}'

curl -X POST localhost:8080/api/articles -H "Authorization: Token $TOKEN" \
  -H 'Content-Type: application/json' -d '{"title":"Hello Echo","body":"...","tag_ids":[1]}'

curl -X POST localhost:8080/api/articles/hello-echo/comments -H "Authorization: Token $TOKEN" \
  -H 'Content-Type: application/json' -d '{"body":"Bài viết hay!"}'

curl localhost:8080/api/articles/hello-echo/comments
curl localhost:8080/api/profiles/ha
```
