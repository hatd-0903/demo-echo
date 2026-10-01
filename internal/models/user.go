package models

import "time"

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// User là model lưu trữ; dữ liệu trả ra API phải đi qua DTO (internal/dto).
//
// Không có DeletedAt (soft delete): xoá user là DELETE thật, để unique index
// username/email được giải phóng (đăng ký lại được) và FK CASCADE dọn sạch
// article, comment, follow của user đó.
type User struct {
	ID       uint   `gorm:"primaryKey"`
	Username string `gorm:"size:100;uniqueIndex;not null"`
	Email    string `gorm:"size:255;uniqueIndex;not null"`
	Password string `gorm:"size:255;not null" json:"-"`
	Bio      string `gorm:"type:text"`
	Image    string `gorm:"size:500"`
	Role     string `gorm:"size:20;not null;default:user"`

	// HasMany: một user viết nhiều article (articles.author_id -> users.id).
	// FK được GORM tạo từ phía HasMany nên constraint phải khai báo ở đây.
	Articles []Article `gorm:"foreignKey:AuthorID;constraint:OnDelete:CASCADE"`
	// HasMany: một user viết nhiều comment (comments.user_id -> users.id).
	Comments []Comment `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}
