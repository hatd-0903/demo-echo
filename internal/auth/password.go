package auth

import "golang.org/x/crypto/bcrypt"

const MinPasswordLength = 6

// HashPassword băm mật khẩu bằng bcrypt (salt được sinh tự động và lưu kèm trong hash).
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword so sánh mật khẩu người dùng nhập với hash đã lưu.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
