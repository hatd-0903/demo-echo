package dto

import (
	"time"

	"demo_echo/internal/models"
)

type RegisterRequest struct {
	Username string `json:"username" example:"alice"`
	Email    string `json:"email" example:"alice@example.com"`
	Password string `json:"password" example:"secret123" minLength:"6"`
}

type LoginRequest struct {
	Email    string `json:"email" example:"alice@example.com"`
	Password string `json:"password" example:"secret123"`
}

type UpdateUserRequest struct {
	Username *string `json:"username" example:"alice"`
	Email    *string `json:"email" example:"alice@example.com"`
	Password *string `json:"password" example:"new-secret123" minLength:"6"`
	Bio      *string `json:"bio" example:"Gopher ở Hà Nội"`
	Image    *string `json:"image" example:"https://i.pravatar.cc/150"`
}

// UserResponse là thông tin đầy đủ (có email, role; không có password) và CHỈ được trả
// cho chính chủ tài khoản: GET /user, đăng ký/đăng nhập, PUT /users/:id của mình.
// Mọi chỗ xem user khác phải dùng ProfileResponse.
type UserResponse struct {
	ID        uint      `json:"id" example:"1"`
	Username  string    `json:"username" example:"alice"`
	Email     string    `json:"email" example:"alice@example.com"`
	Bio       string    `json:"bio" example:"Gopher ở Hà Nội"`
	Image     string    `json:"image" example:"https://i.pravatar.cc/150"`
	Role      string    `json:"role" example:"user" enums:"user,admin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuthUserResponse là user hiện tại kèm JWT, trả về khi đăng ký/đăng nhập và GET /api/user.
type AuthUserResponse struct {
	UserResponse
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// ProfileResponse là thông tin công khai của user, không lộ email/password.
// Dùng cho /api/profiles/:username và làm "author" trong article, comment.
type ProfileResponse struct {
	Username string `json:"username" example:"alice"`
	Bio      string `json:"bio" example:"Gopher ở Hà Nội"`
	Image    string `json:"image" example:"https://i.pravatar.cc/150"`
	// Following chỉ có ở các endpoint /api/profiles (nil = không trả về).
	Following *bool `json:"following,omitempty" example:"false"`
}

func NewUserResponse(u *models.User) UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		Bio:       u.Bio,
		Image:     u.Image,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

func NewAuthUserResponse(u *models.User, token string) AuthUserResponse {
	return AuthUserResponse{UserResponse: NewUserResponse(u), Token: token}
}

func NewProfileListResponse(users []models.User) []*ProfileResponse {
	return mapSlice(users, NewProfileResponse)
}

// NewProfileWithFollowing dùng cho /api/profiles: kèm trạng thái người xem có đang follow hay không.
func NewProfileWithFollowing(u *models.User, following bool) *ProfileResponse {
	p := NewProfileResponse(u)
	p.Following = &following
	return p
}

// NewProfileResponse trả về nil nếu quan hệ chưa được Preload.
func NewProfileResponse(u *models.User) *ProfileResponse {
	if u == nil {
		return nil
	}
	return &ProfileResponse{
		Username: u.Username,
		Bio:      u.Bio,
		Image:    u.Image,
	}
}
