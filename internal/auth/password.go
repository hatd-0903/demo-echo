package auth

import (
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const MinPasswordLength = 6

// HashPassword băm mật khẩu bằng bcrypt (salt được sinh tự động và lưu kèm trong hash).
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// dummyHash là hash bcrypt (cùng cost với hash thật) của một chuỗi không ai biết,
// tạo một lần khi cần.
var dummyHash = sync.OnceValue(func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return hash
})

// CheckPasswordAgainstDummy tốn thời gian như CheckPassword nhưng luôn trả false.
// Dùng ở nhánh "không tìm thấy user" khi đăng nhập để thời gian phản hồi không
// tiết lộ email nào đã được đăng ký.
func CheckPasswordAgainstDummy(password string) bool {
	_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
	return false
}

// CheckPassword so sánh mật khẩu người dùng nhập với hash đã lưu.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
